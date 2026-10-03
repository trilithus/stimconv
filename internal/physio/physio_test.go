package physio

import (
	"math"
	"testing"
)

const tau = 355e-6

func TestEffectiveReference(t *testing.T) {
	// a full sine at the reference carrier is exactly 1
	if e := Effective(1, SineFormFactor, 2000, 2000, tau); math.Abs(e-1) > 1e-12 {
		t.Fatalf("got %g", e)
	}
}

func TestSquareVsSine(t *testing.T) {
	// A square wave carries 1/(2/pi) = 1.57x the charge of a sine of equal peak.
	sq := Effective(1, 1, 650, 2000, tau)
	sn := Effective(1, SineFormFactor, 650, 2000, tau)
	if math.Abs(sq/sn-math.Pi/2) > 1e-9 {
		t.Fatalf("square/sine = %g", sq/sn)
	}
	// Wider phases (lower carrier) are more effective per unit current.
	if Effective(1, SineFormFactor, 650, 2000, tau) <= Effective(1, SineFormFactor, 2000, 2000, tau) {
		t.Fatal("expected 650 Hz to be more effective than 2000 Hz at equal current")
	}
}

// Consistency with restim: restim's derating makes a sine at f with volume v feel
// like a sine at fmax with volume v. Our Effective for a sine at f, divided by
// the derating restim applies, should be the same for every f.
func TestConsistentWithRestimDerating(t *testing.T) {
	base := Effective(1, SineFormFactor, 2000, 2000, tau) / RestimDerating(2000, 2000, tau)
	for _, f := range []float64{300, 650, 1000, 1500} {
		// restim outputs current = v*derating(f); its effective intensity:
		eff := Effective(RestimDerating(2000, f, tau), SineFormFactor, f, 2000, tau)
		if math.Abs(eff-base) > 1e-9 {
			t.Errorf("f=%g: restim output effective %g, want %g", f, eff, base)
		}
	}
}

func TestBeat(t *testing.T) {
	if Beat(800, 900) != 100 {
		t.Fatal("beat")
	}
}
