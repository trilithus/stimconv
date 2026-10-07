package analysis

import (
	"io"
	"math"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
)

// sliceSource is an in-memory decode.Source.
type sliceSource struct {
	sr   int
	data []float32 // interleaved stereo
	pos  int
}

func (s *sliceSource) SampleRate() int { return s.sr }
func (s *sliceSource) Close() error    { return nil }
func (s *sliceSource) Read(dst []float32) (int, error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	n := copy(dst, s.data[s.pos:]) &^ 1
	s.pos += n
	return n / 2, nil
}

const sr = 48000

func synth(seconds float64, fn func(t float64) (l, r float64)) *sliceSource {
	n := int(seconds * sr)
	d := make([]float32, 2*n)
	for i := 0; i < n; i++ {
		l, r := fn(float64(i) / sr)
		d[2*i], d[2*i+1] = float32(l), float32(r)
	}
	return &sliceSource{sr: sr, data: d}
}

func analyse(t *testing.T, src *sliceSource, circuitOn bool) *Features {
	t.Helper()
	cc := config.Default().Circuit
	cc.Enabled = circuitOn
	raw, err := Measure(src, cc)
	if err != nil {
		t.Fatal(err)
	}
	return Derive(raw, 3, -40)
}

// A short burst of decoder garbage (a corrupt AAC frame) must not become the
// track maximum and gate the real signal as silence.
func TestCorruptBurstIsClipped(t *testing.T) {
	src := synth(4, func(tm float64) (float64, float64) {
		v := 0.5 * math.Sin(2*math.Pi*800*tm)
		if tm > 2 && tm < 2.02 {
			return v, 1500 * math.Sin(2*math.Pi*3000*tm)
		}
		return v, v
	})
	cc := config.Default().Circuit
	raw, err := Measure(src, cc)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Overload[1] == 0 || raw.Overload[0] != 0 || math.Abs(raw.OverloadAt-2) > 0.01 {
		t.Errorf("overload = %v at %.3f s", raw.Overload, raw.OverloadAt)
	}
	f := Derive(raw, 3, -40)
	if m := median(f.Ch[1].E, 500, 1500); m == 0 {
		t.Error("signal before the burst was gated as silence")
	}
}

func median(v []float32, from, to int) float64 {
	var s []float64
	for _, x := range v[from:to] {
		s = append(s, float64(x))
	}
	// simple selection
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return s[len(s)/2]
}

// stayh-like: 650 Hz square wave, 20 Hz bursts with sharp attack and decay, mono.
func stayh(t float64) (float64, float64) {
	ph := math.Mod(t*20, 1) // 50 ms period
	env := 0.0
	if ph < 0.88 {
		env = 0.9 * (1 - 0.65*ph/0.88)
	}
	v := env * math.Copysign(1, math.Sin(2*math.Pi*650*t))
	return v, v
}

func TestStayhLike(t *testing.T) {
	f := analyse(t, synth(6, stayh), false)
	ch := f.Ch[0]
	if c := median(ch.Carrier, 100, 500); math.Abs(c-650) > 10 {
		t.Errorf("carrier %.1f, want 650", c)
	}
	if ff := median(ch.FormFactor, 100, 500); ff < 0.85 {
		t.Errorf("form factor %.2f, want ~1 for a square wave", ff)
	}
	if r := median(ch.Rate, 5, 25); math.Abs(r-20) > 1 {
		t.Errorf("rhythm %.2f Hz, want 20", r)
	}
	if a := median(ch.Attack, 5, 25); a > 0.01 {
		t.Errorf("attack %.4f s, want sharp (<10 ms)", a)
	}
}

// long file 39-53 min: 2400 Hz sine with 14 Hz sinusoidal AM.
func TestSinusoidalAM(t *testing.T) {
	f := analyse(t, synth(6, func(t float64) (float64, float64) {
		v := 0.7 * (0.55 + 0.45*math.Sin(2*math.Pi*14*t)) * math.Sin(2*math.Pi*2400*t)
		return v, v
	}), false)
	ch := f.Ch[0]
	if c := median(ch.Carrier, 100, 500); math.Abs(c-2400) > 30 {
		t.Errorf("carrier %.1f, want 2400", c)
	}
	if ff := median(ch.FormFactor, 100, 500); math.Abs(ff-2/math.Pi) > 0.06 {
		t.Errorf("form factor %.3f, want 0.637 for a sine", ff)
	}
	if r := median(ch.Rate, 5, 25); math.Abs(r-14) > 1 {
		t.Errorf("rhythm %.2f Hz, want 14", r)
	}
	if a := median(ch.Attack, 5, 25); a < 0.015 {
		t.Errorf("attack %.4f s, want smooth (sinusoidal AM)", a)
	}
}

// long file 0-12 min: L 800 Hz / R 900 Hz with 0.7 Hz anti-phase crossfade.
func TestCrossfade(t *testing.T) {
	f := analyse(t, synth(8, func(t float64) (float64, float64) {
		g := 0.5 + 0.45*math.Sin(2*math.Pi*0.7*t)
		return 0.45 * g * math.Sin(2*math.Pi*800*t), 0.45 * (1 - g) * math.Sin(2*math.Pi*900*t)
	}), true)
	if c := median(f.Ch[0].Carrier, 100, 700); math.Abs(c-800) > 10 {
		t.Errorf("L carrier %.1f", c)
	}
	if c := median(f.Ch[1].Carrier, 100, 700); math.Abs(c-900) > 10 {
		t.Errorf("R carrier %.1f", c)
	}
	if r := median(f.Ch[0].Rate, 5, 35); r != 0 {
		t.Errorf("unexpected rhythm %.2f Hz in a slow crossfade", r)
	}
	// slow envelopes must be anti-phase
	var num, da, db float64
	for i := 1000; i < 7000; i++ {
		a, b := float64(f.Ch[0].S[i]), float64(f.Ch[1].S[i])
		num += a * b
		da += a * a
		db += b * b
	}
	meanA, meanB := 0.0, 0.0
	for i := 1000; i < 7000; i++ {
		meanA += float64(f.Ch[0].S[i])
		meanB += float64(f.Ch[1].S[i])
	}
	meanA /= 6000
	meanB /= 6000
	cov := 0.0
	for i := 1000; i < 7000; i++ {
		cov += (float64(f.Ch[0].S[i]) - meanA) * (float64(f.Ch[1].S[i]) - meanB)
	}
	if cov >= 0 {
		t.Errorf("slow envelopes not anti-phase (cov %g)", cov)
	}
	// independent carriers => covariance ~ 0 on average
	var ll, lr, rr float64
	for i := 100; i < 700; i++ {
		ll += float64(f.CLL[i])
		lr += float64(f.CLR[i])
		rr += float64(f.CRR[i])
	}
	if corr := lr / math.Sqrt(ll*rr); math.Abs(corr) > 0.1 {
		t.Errorf("L/R correlation %.2f, want ~0 for different carriers", corr)
	}
}
