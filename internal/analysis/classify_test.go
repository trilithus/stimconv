package analysis

import (
	"io"
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/trilithus/stimconv/internal/decode"
)

// funcSource renders fn(t) -> (l, r) for dur seconds at 44.1 kHz.
type funcSource struct {
	fn  func(t float64) (float64, float64)
	n   int
	pos int
}

func newFuncSource(dur float64, fn func(t float64) (float64, float64)) *funcSource {
	return &funcSource{fn: fn, n: int(dur * 44100)}
}

func (s *funcSource) SampleRate() int { return 44100 }
func (s *funcSource) Close() error    { return nil }
func (s *funcSource) Read(dst []float32) (int, error) {
	if s.pos >= s.n {
		return 0, io.EOF
	}
	k := 0
	for ; k < len(dst)/2 && s.pos < s.n; k++ {
		l, r := s.fn(float64(s.pos) / 44100)
		dst[2*k], dst[2*k+1] = float32(l), float32(r)
		s.pos++
	}
	return k, nil
}

func sq(x float64) float64 { return math.Copysign(1, math.Sin(x)) }

// stim signals modelled on the sample segments
var stimSignals = map[string]func(t float64) (float64, float64){
	"650 Hz square, 20 Hz bursts": func(t float64) (float64, float64) {
		g := 0.0
		if math.Mod(t*20, 1) < 0.5 {
			g = 0.8
		}
		return g * sq(2*math.Pi*650*t), 0.5 * sq(2*math.Pi*650*t)
	},
	"800/900 Hz crossfade": func(t float64) (float64, float64) {
		a := 0.5 + 0.5*math.Sin(2*math.Pi*0.1*t)
		v := 0.7 * (a*math.Sin(2*math.Pi*800*t) + (1-a)*math.Sin(2*math.Pi*900*t))
		return v, v
	},
	"2.4 kHz AM": func(t float64) (float64, float64) {
		m := 0.5 + 0.5*math.Sin(2*math.Pi*3*t)
		return 0.8 * m * math.Sin(2*math.Pi*2400*t), 0.6 * (1 - m) * math.Sin(2*math.Pi*2400*t)
	},
	"300 Hz sine, slow ramp": func(t float64) (float64, float64) {
		g := 0.2 + 0.8*math.Mod(t/10, 1)
		return g * math.Sin(2*math.Pi*300*t), 0
	},
}

var rng = rand.New(rand.NewSource(1))

// note returns a decaying harmonic tone.
func note(f, t, age float64) float64 {
	v := 0.0
	for h := 1.0; h <= 6; h++ {
		v += math.Sin(2*math.Pi*f*h*t) / h
	}
	return v * math.Exp(-2*age)
}

var audioSignals = map[string]func(t float64) (float64, float64){
	"music: bass, chords, drums": func(t float64) (float64, float64) {
		beat := math.Floor(t * 2)
		age := t*2 - beat
		roots := []float64{110, 87.3, 130.8, 98}
		r := roots[int(beat/4)%4]
		bass := 0.5 * note(r/2, t, age)
		chord := 0.25 * (note(r*2, t, age) + note(r*2.52, t, age) + note(r*3, t, age))
		kick := 0.0
		if age < 0.1 {
			kick = 0.8 * math.Sin(2*math.Pi*(60-200*age)*t) * (1 - age*10)
		}
		hat := 0.1 * rng.NormFloat64() * math.Exp(-40*math.Mod(t*4, 1))
		return 0.3 * (bass + chord + kick + hat), 0.3 * (bass + chord*0.8 + kick + hat)
	},
	"speech-like, male 110 Hz":   speech(110),
	"speech-like, female 210 Hz": speech(210),
	"white noise": func(t float64) (float64, float64) {
		return 0.2 * rng.NormFloat64(), 0.2 * rng.NormFloat64()
	},
}

// speech is a glottal pulse train with pitch movement, syllables and pauses,
// shaped by two resonances.
func speech(f0 float64) func(t float64) (float64, float64) {
	var phase, y1, y2, z1, z2 float64
	return func(t float64) (float64, float64) {
		f := f0 * (1 + 0.15*math.Sin(2*math.Pi*0.7*t))
		phase += f / 44100
		pulse := 0.0
		if phase >= 1 {
			phase--
			pulse = 1
		}
		syl := math.Max(0, math.Sin(2*math.Pi*4*t))
		if math.Mod(t, 3) > 2.4 { // pause
			syl = 0
		}
		// two resonators (formants ~700 and ~1200 Hz)
		res := func(x float64, fc float64, a1, a2 *float64) float64 {
			r := 0.97
			c := 2 * r * math.Cos(2*math.Pi*fc/44100)
			y := x + c**a1 - r*r**a2
			*a2, *a1 = *a1, y
			return y
		}
		v := res(pulse, 700, &y1, &y2)*0.02 + res(pulse, 1200, &z1, &z2)*0.01
		v *= syl
		return v, v
	}
}

func TestClassifySynthetic(t *testing.T) {
	for name, fn := range stimSignals {
		c, err := Classify(newFuncSource(30, fn), 90)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-30s %v frac %.2f low %.3f peak %.2f", name, c.Kind, c.StimFrac, c.LowFreq, c.Peak)
		if c.Kind != KindStim {
			t.Errorf("%s: %v, want stim", name, c)
		}
	}
	for name, fn := range audioSignals {
		c, err := Classify(newFuncSource(30, fn), 90)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-30s %v frac %.2f low %.3f peak %.2f", name, c.Kind, c.StimFrac, c.LowFreq, c.Peak)
		if c.Kind != KindAudio {
			t.Errorf("%s: %v, want music/speech", name, c)
		}
	}
}

func TestClassifySilence(t *testing.T) {
	c, _ := Classify(newFuncSource(10, func(float64) (float64, float64) { return 0, 0 }), 90)
	if c.Kind != KindSilent {
		t.Errorf("silence: %v", c.Kind)
	}
}

// TestClassifySamples checks the real sample tracks (skipped without them).
func TestClassifySamples(t *testing.T) {
	for _, f := range []string{"../../reference/audio/sample_long.mp3", "../../reference/audio/sample_stayh.mp3"} {
		if _, err := os.Stat(f); err != nil {
			t.Skip("sample audio not present")
		}
		src, _, err := decode.Open(f, decode.Options{})
		if err != nil {
			t.Logf("%s: %v (skipped)", f, err)
			continue
		}
		c, err := Classify(src, 90)
		src.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %v frac %.2f low %.3f peak %.2f frames %d", f, c.Kind, c.StimFrac, c.LowFreq, c.Peak, c.Frames)
		if c.Kind != KindStim {
			t.Errorf("%s: %v", f, c)
		}
	}
}
