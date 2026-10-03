package threephase

import (
	"math"
	"testing"
)

// Vectors generated with restim's stim_math.threephase.ThreePhaseSignalGenerator.generate:
// mean(L*L), mean(L*R), mean(R*R) over one carrier cycle.
var restimVectors = [][5]float64{
	{0, 0, 0.499999995, 0.250000004, 0.499999995},
	{1, 0, 0.375000000, 0.375000000, 0.375000000},
	{-1, 0, 0.124999995, -0.124999995, 0.124999995},
	{0, 1, 0.466506345, 0.125000002, 0.033493651},
	{0, -1, 0.033493651, 0.125000002, 0.466506345},
	{0.5, 0.3, 0.474039623, 0.323839304, 0.289978285},
	{-0.3, 0.6, 0.399911037, 0.038856443, 0.054580060},
	{0.7, -0.7, 0.185327046, 0.301771487, 0.491482342},
	{0.2, 0.1, 0.483566469, 0.289167979, 0.406646388},
}

func TestForwardMatchesRestim(t *testing.T) {
	for _, v := range restimVectors {
		ll, lr, rr := Forward(v[0], v[1])
		if math.Abs(ll-v[2]) > 1e-6 || math.Abs(lr-v[3]) > 1e-6 || math.Abs(rr-v[4]) > 1e-6 {
			t.Errorf("Forward(%g,%g) = %g %g %g, restim %g %g %g", v[0], v[1], ll, lr, rr, v[2], v[3], v[4])
		}
	}
}

func TestInverseRecoversPosition(t *testing.T) {
	for _, v := range restimVectors {
		for _, scale := range []float64{1, 0.01, 7} {
			a, b, ok := Inverse(v[2]*scale, v[3]*scale, v[4]*scale)
			if !ok || math.Abs(a-v[0]) > 1e-3 || math.Abs(b-v[1]) > 1e-3 {
				t.Errorf("Inverse(scale %g) of (%g,%g) = (%g,%g) ok=%v", scale, v[0], v[1], a, b, ok)
			}
		}
	}
}

func TestInverseSilence(t *testing.T) {
	if _, _, ok := Inverse(0, 0, 0); ok {
		t.Fatal("expected !ok for silence")
	}
}
