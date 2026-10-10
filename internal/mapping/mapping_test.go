package mapping

import (
	"io"
	"math"
	"strings"
	"testing"

	"github.com/trilithus/stimconv/internal/analysis"
	"github.com/trilithus/stimconv/internal/circuit"
	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/funscript"
)

type sliceSource struct {
	data []float32
	pos  int
}

func (s *sliceSource) SampleRate() int { return 48000 }
func (s *sliceSource) Close() error    { return nil }
func (s *sliceSource) Read(dst []float32) (int, error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	n := copy(dst, s.data[s.pos:]) &^ 1
	s.pos += n
	return n / 2, nil
}

func run(t *testing.T, seconds float64, cfg config.Config, fn func(t float64) (float64, float64)) *Result {
	t.Helper()
	const sr = 48000
	n := int(seconds * sr)
	d := make([]float32, 2*n)
	for i := 0; i < n; i++ {
		l, r := fn(float64(i) / sr)
		d[2*i], d[2*i+1] = float32(l), float32(r)
	}
	raw, err := analysis.Measure(&sliceSource{data: d}, cfg.Circuit)
	if err != nil {
		t.Fatal(err)
	}
	f := analysis.Derive(raw, cfg.ModSplitHz, cfg.SilenceDB)
	res, err := Map(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func axis(res *Result, name string) *funscript.Axis {
	for _, a := range res.Axes {
		if a.Name == name {
			return a
		}
	}
	return nil
}

// median of an axis between two times
func med(a *funscript.Axis, t0, t1 float64) float64 {
	var v []float64
	for i, t := range a.Times {
		if t >= t0 && t <= t1 {
			v = append(v, a.Values[i])
		}
	}
	for i := range v {
		for j := i + 1; j < len(v); j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
	return v[len(v)/2]
}

func TestStayhLikeDual(t *testing.T) {
	cfg := config.Default()
	cfg.Topology = "dual"
	res := run(t, 6, cfg, func(t float64) (float64, float64) {
		ph := math.Mod(t*20, 1)
		env := 0.0
		if ph < 0.88 {
			env = 0.9 * (1 - 0.65*ph/0.88)
		}
		v := env * math.Copysign(1, math.Sin(2*math.Pi*650*t))
		return v, v
	})
	if f := med(axis(res, "frequency"), 1, 5); math.Abs(f-650) > 15 {
		t.Errorf("frequency %.1f", f)
	}
	if pf := med(axis(res, "pulse_frequency"), 1, 5); math.Abs(pf-20) > 1 {
		t.Errorf("pulse_frequency %.2f, want 20 (fused rhythm -> FOC pulses)", pf)
	}
	if w := med(axis(res, "pulse_width"), 1, 5); w < 15 {
		t.Errorf("pulse_width %.1f cycles, want long bursts (clamped at 20)", w)
	}
	if res.ModeTime["pulses"] < 0.7 {
		t.Errorf("mode time %v", res.ModeTime)
	}
	if e3 := med(axis(res, "e3"), 1, 5); math.Abs(e3-1) > 0.05 {
		t.Errorf("mono signal should drive both pairs equally, e3=%.2f", e3)
	}
	found := false
	for _, w := range res.Warnings {
		found = found || strings.Contains(w, "pulse width limit")
	}
	if !found {
		t.Errorf("expected a pulse width warning, got %v", res.Warnings)
	}
}

func TestCrossfadeDualBlend(t *testing.T) {
	cfg := config.Default()
	cfg.Topology = "dual"
	cfg.Overlap = "blend"
	res := run(t, 8, cfg, func(t float64) (float64, float64) {
		g := 0.5 + 0.45*math.Sin(2*math.Pi*0.5*t)
		return 0.45 * g * math.Sin(2*math.Pi*800*t), 0.45 * (1 - g) * math.Sin(2*math.Pi*900*t)
	})
	e1, e3 := axis(res, "e1"), axis(res, "e3")
	// t=0.5 s: A loudest; t=1.5 s: B loudest
	if v := med(e3, 0.4, 0.6); v > 0.3 {
		t.Errorf("A dominant: e3=%.2f, want low", v)
	}
	if v := med(e1, 1.4, 1.6); v > 0.3 {
		t.Errorf("B dominant: e1=%.2f, want low", v)
	}
	if res.ModeTime["continuous"] < 0.8 {
		t.Errorf("slow crossfade should be continuous, got %v", res.ModeTime)
	}
}

func TestJoinedIFCBeat(t *testing.T) {
	cfg := config.Default()
	cfg.Topology = "joined"
	res := run(t, 4, cfg, func(t float64) (float64, float64) {
		return 0.4 * math.Sin(2*math.Pi*800*t), 0.4 * math.Sin(2*math.Pi*900*t)
	})
	if pf := med(axis(res, "pulse_frequency"), 1, 3); math.Abs(pf-100) > 3 {
		t.Errorf("pulse_frequency %.1f, want 100 Hz interferential beat", pf)
	}
	if res.ModeTime["ifc"] < 0.8 {
		t.Errorf("mode time %v", res.ModeTime)
	}
}

func TestJoinedMonoIsNeutral(t *testing.T) {
	cfg := config.Default()
	cfg.Topology = "joined"
	res := run(t, 3, cfg, func(t float64) (float64, float64) {
		v := 0.4 * math.Sin(2*math.Pi*800*t)
		return v, v
	})
	// L == R: current flows into the tied commons -> alpha = +1 (neutral electrode)
	if a := med(axis(res, "alpha"), 0.5, 2.5); a < 0.95 {
		t.Errorf("alpha %.3f, want ~1", a)
	}
	res = run(t, 3, cfg, func(t float64) (float64, float64) {
		v := 0.4 * math.Sin(2*math.Pi*800*t)
		return v, -v
	})
	if a := med(axis(res, "alpha"), 0.5, 2.5); a > -0.95 {
		t.Errorf("anti-phase alpha %.3f, want ~-1", a)
	}
}

func TestHighCarrierClamped(t *testing.T) {
	cfg := config.Default()
	res := run(t, 4, cfg, func(t float64) (float64, float64) {
		v := 0.7 * (0.55 + 0.45*math.Sin(2*math.Pi*14*t)) * math.Sin(2*math.Pi*2400*t)
		return v, v
	})
	if f := med(axis(res, "frequency"), 1, 3); f != 2000 {
		t.Errorf("frequency %.1f, want clamped to FOC max 2000", f)
	}
	if pf := med(axis(res, "pulse_frequency"), 1, 3); math.Abs(pf-14) > 1 {
		t.Errorf("pulse_frequency %.2f, want 14", pf)
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "exceeds FOC-Stim") || !strings.Contains(joined, "frequency:") {
		t.Errorf("expected carrier and range warnings, got:\n%s", joined)
	}
}

func TestEffectiveIntensityMatchesAcrossCarriers(t *testing.T) {
	// Same peak current, 650 Hz square vs 650 Hz sine: effective intensity
	// should differ by the charge ratio pi/2.
	cfg := config.Default()
	cfg.Circuit.Enabled = false
	cfg.Intensity = "effective"
	cfg.Normalize = "abs"
	cfg.RefLevel = 4 // keep below clipping
	cfg.Gamma = 1    // compare linear volumes
	sq := run(t, 2, cfg, func(t float64) (float64, float64) {
		v := 0.5 * math.Copysign(1, math.Sin(2*math.Pi*650*t))
		return v, v
	})
	sn := run(t, 2, cfg, func(t float64) (float64, float64) {
		v := 0.5 * math.Sin(2*math.Pi*650*t)
		return v, v
	})
	r := med(axis(sq, "volume"), 0.5, 1.5) / med(axis(sn, "volume"), 0.5, 1.5)
	if math.Abs(r-math.Pi/2) > 0.1 {
		t.Errorf("square/sine effective ratio %.3f, want %.3f", r, math.Pi/2)
	}
}

// auto: independent A/B content (different carriers) is multiplexed ...
func TestAutoMultiplexesIndependentContent(t *testing.T) {
	cfg := config.Default() // overlap auto
	cfg.Topology = "dual"
	res := run(t, 6, cfg, func(t float64) (float64, float64) {
		return 0.4 * math.Sin(2*math.Pi*800*t), 0.4 * math.Sin(2*math.Pi*900*t)
	})
	if res.Multiplexed < 0.8 {
		t.Fatalf("multiplexed %.2f, want most of the time", res.Multiplexed)
	}
	e1, e3 := axis(res, "e1"), axis(res, "e3")
	// alternation at mux-hz 4: each pair fully on in its 125 ms slot
	if v := med(e3, 2.01, 2.11); v > 0.05 {
		t.Errorf("A slot: e3=%.2f", v)
	}
	if v := med(e1, 2.135, 2.235); v > 0.05 {
		t.Errorf("B slot: e1=%.2f", v)
	}
	// each slot plays its own carrier
	if f := med(axis(res, "frequency"), 2.01, 2.11); math.Abs(f-800) > 10 {
		t.Errorf("A slot carrier %.0f", f)
	}
	if f := med(axis(res, "frequency"), 2.135, 2.235); math.Abs(f-900) > 10 {
		t.Errorf("B slot carrier %.0f", f)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "different rhythms") {
			t.Errorf("multiplexed steps must not count as rhythm conflicts: %s", w)
		}
	}
}

// ... while mono content (A = B, even at different levels) is blended.
func TestAutoBlendsMono(t *testing.T) {
	cfg := config.Default()
	cfg.Topology = "dual"
	res := run(t, 6, cfg, func(t float64) (float64, float64) {
		v := 0.4 * math.Sin(2*math.Pi*800*t)
		return v, 0.7 * v
	})
	if res.Multiplexed > 0.02 {
		t.Fatalf("multiplexed %.2f, want ~0 for mono content", res.Multiplexed)
	}
	if v := med(axis(res, "e3"), 1, 5); math.Abs(v-0.7) > 0.05 {
		t.Errorf("blend e3=%.2f, want 0.7", v)
	}
}

func TestMajority(t *testing.T) {
	in := []bool{false, true, false, false, true, true, true, false, true, true}
	out := majority(in, 3)
	want := []bool{false, false, false, false, true, true, true, true, true, true}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("majority = %v, want %v", out, want)
		}
	}
}

// abs with the auto reference maps full-scale audio to volume 1: a full-scale
// square at the lowest carrier, through the circuit model when it is on.
func TestAutoRefLevel(t *testing.T) {
	cfg := config.Default()
	if cfg.RefLevel != 0 {
		t.Fatalf("default ref_level = %v, want 0 (auto)", cfg.RefLevel)
	}
	// the default (no circuit model, plain current) treats the audio as the
	// level: full scale is 1
	if got := RefLevel(cfg); got != 1 {
		t.Errorf("auto ref with the default settings = %v, want 1", got)
	}
	// a full-scale square through the circuit: at least the steady-state
	// sine current (its edges add capacitive spikes), and in the tens of mA
	cfg.Circuit.Enabled, cfg.Intensity = true, "effective"
	got := RefLevel(cfg)
	if sine := circuit.Gain(cfg.Circuit, cfg.RefCarrierHz); got < sine || got > 0.5 {
		t.Errorf("auto ref with circuit = %v, want >= %v (sine gain) and < 0.5", got, sine)
	}
	// without the circuit model: a full-scale square is level 1 as current,
	// more with effective intensity (square, 300 Hz count as stronger)
	cfg.Circuit.Enabled = false
	if got := RefLevel(cfg); got <= 1 {
		t.Errorf("auto ref without circuit, effective = %v, want > 1", got)
	}
	cfg.Intensity = "current"
	if got := RefLevel(cfg); got != 1 {
		t.Errorf("auto ref without circuit, current = %v, want 1", got)
	}
	cfg.RefLevel = 0.2
	if got := RefLevel(cfg); got != 0.2 {
		t.Errorf("explicit ref = %v, want 0.2", got)
	}
}
