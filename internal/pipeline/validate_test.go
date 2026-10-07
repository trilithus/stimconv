package pipeline

import (
	"strings"
	"testing"

	"github.com/trilithus/stimconv/internal/analysis"
	"github.com/trilithus/stimconv/internal/funscript"
)

// rawActive builds a 10 s input that carries signal throughout, with an
// optional spike.
func rawActive(spike float32) *analysis.Raw {
	r := &analysis.Raw{Duration: 10}
	for c := range 2 {
		r.Bucket[c] = make([]float32, 10*analysis.EnvRate)
		for i := range r.Bucket[c] {
			r.Bucket[c][i] = 0.1
		}
	}
	r.Bucket[1][5000] = spike
	return r
}

func ramp(name string) writtenAxis {
	return writtenAxis{Name: name, Actions: []funscript.Action{{At: 0, Pos: 20}, {At: 5000, Pos: 80}, {At: 10000, Pos: 40}}}
}

func TestValidateClean(t *testing.T) {
	if issues := validate(rawActive(0.1), -40, []writtenAxis{ramp("volume"), ramp("e1")}); len(issues) != 0 {
		t.Errorf("clean output flagged: %v", issues)
	}
}

// A corrupt-frame glitch that set the level reference, so the whole output
// came out silent.
func TestValidateLostSignal(t *testing.T) {
	silent := writtenAxis{Name: "volume", Actions: []funscript.Action{{At: 0, Pos: 0}, {At: 10000, Pos: 0}}}
	issues := strings.Join(validate(rawActive(240), -40, []writtenAxis{silent}), "\n")
	for _, want := range []string{"above its 99.5th percentile", "signal was lost", "never changes"} {
		if !strings.Contains(issues, want) {
			t.Errorf("missing %q in:\n%s", want, issues)
		}
	}
}

func TestValidateStructure(t *testing.T) {
	bad := writtenAxis{Name: "e1", Actions: []funscript.Action{{At: 0, Pos: 10}, {At: 0, Pos: 120}, {At: 4000, Pos: 5}}}
	issues := strings.Join(validate(rawActive(0.1), -40, []writtenAxis{ramp("volume"), bad, {Name: "e2"}}), "\n")
	for _, want := range []string{"e1: position 120", "e1: ends at 4.0 s", "e2: funscript has no points"} {
		if !strings.Contains(issues, want) {
			t.Errorf("missing %q in:\n%s", want, issues)
		}
	}
}

func TestValidateRestimLoad(t *testing.T) {
	// 10 axes changing every 60 Hz tick is 600 msg/s
	var axes []writtenAxis
	for range 10 {
		a := writtenAxis{Name: "x"}
		for ms := int64(0); ms <= 10000; ms += 100 {
			a.Actions = append(a.Actions, funscript.Action{At: ms, Pos: int(ms/100) % 2 * 100})
		}
		axes = append(axes, a)
	}
	if peak, _ := restimPeakRate(axes, 10); peak < 550 || peak > 620 {
		t.Errorf("peak = %d, want ~600", peak)
	}
	if !strings.Contains(strings.Join(validate(rawActive(0.1), -40, axes), "\n"), "axis updates/s") {
		t.Error("link load not flagged")
	}
}
