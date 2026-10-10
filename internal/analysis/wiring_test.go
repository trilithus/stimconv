package analysis

import (
	"math"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
)

func wiringOf(t *testing.T, fn func(t float64) (l, r float64)) Wiring {
	t.Helper()
	raw, err := Measure(synth(12, fn), config.Default().Circuit)
	if err != nil {
		t.Fatal(err)
	}
	return AnalyseWiring(raw)
}

func TestWiringVerdicts(t *testing.T) {
	sq := func(x float64) float64 { return math.Copysign(0.5, math.Sin(x)) }
	cases := []struct {
		name string
		fn   func(t float64) (float64, float64)
		want WiringVerdict
	}{
		{"mono", func(t float64) (float64, float64) {
			v := 0.5 * math.Sin(2*math.Pi*800*t)
			return v, 0.7 * v
		}, WiringMono},
		{"antiphase", func(t float64) (float64, float64) {
			v := 0.5 * math.Sin(2*math.Pi*800*t)
			return v, -v
		}, WiringMono},
		{"steady phase offset", func(t float64) (float64, float64) {
			w := 2 * math.Pi * 800 * t
			return 0.5 * math.Sin(w), 0.5 * math.Sin(w+math.Pi/3)
		}, WiringThree},
		{"rotating phase (3 Hz detune)", func(t float64) (float64, float64) {
			return 0.5 * math.Sin(2*math.Pi*800*t), 0.5 * math.Sin(2*math.Pi*803*t)
		}, WiringThree},
		{"different envelopes", func(t float64) (float64, float64) {
			w := 2 * math.Pi * 650 * t
			burst := 0.0
			if math.Mod(t*20, 1) < 0.3 {
				burst = 1
			}
			return sq(w), burst * sq(w) * (0.6 + 0.4*math.Sin(2*math.Pi*1.5*t))
		}, WiringFour},
		{"alternating pairs", func(t float64) (float64, float64) {
			v := sq(2 * math.Pi * 650 * t)
			if math.Mod(t, 2) < 1 {
				return v, 0
			}
			return 0, v
		}, WiringFour},
		{"detuned carriers", func(t float64) (float64, float64) {
			return 0.5 * math.Sin(2*math.Pi*800*t), 0.5 * math.Sin(2*math.Pi*1100*t)
		}, WiringDetuned},
	}
	for _, c := range cases {
		w := wiringOf(t, c.fn)
		if w.Verdict != c.want {
			t.Errorf("%s: verdict %d, want %d\n%s", c.name, w.Verdict, c.want, w)
		}
	}
}
