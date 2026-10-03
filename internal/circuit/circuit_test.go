package circuit

import (
	"math"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
)

func TestStepMatchesAnalyticGain(t *testing.T) {
	c := config.Default().Circuit
	const sr = 48000
	for _, f := range []float64{50, 200, 650, 900, 2400} {
		m := New(c, sr)
		peak := 0.0
		for i := 0; i < sr; i++ {
			y := m.Step(math.Sin(2 * math.Pi * f * float64(i) / sr))
			if i > sr/2 {
				peak = math.Max(peak, math.Abs(y))
			}
		}
		want := Gain(c, f)
		if math.Abs(peak-want)/want > 0.02 {
			t.Errorf("f=%g: simulated %.5f A, analytic %.5f A", f, peak, want)
		}
	}
}

func TestPlausibleCurrent(t *testing.T) {
	c := config.Default().Circuit
	g := Gain(c, 800)
	// ~12 V / ~5 ohm / 35 turns ~ 70 mA; allow for skin load and inductance.
	if g < 0.03 || g > 0.1 {
		t.Fatalf("full-scale current at 800 Hz = %.3f A, expected tens of mA", g)
	}
	if Gain(c, 50) >= Gain(c, 800) {
		t.Fatal("expected low-frequency roll-off from magnetising inductance")
	}
}
