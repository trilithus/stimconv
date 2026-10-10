// Package mapping turns analysed features into restim funscript axes for
// FOC-Stim, using the configurable strategies in config.Config.
package mapping

import (
	"fmt"
	"math"
	"sort"

	"github.com/trilithus/stimconv/internal/analysis"
	"github.com/trilithus/stimconv/internal/circuit"
	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/funscript"
	"github.com/trilithus/stimconv/internal/physio"
	"github.com/trilithus/stimconv/internal/threephase"
)

// Rendering modes of a channel's rhythm.
const (
	ModeSilent = iota
	ModeSteady // no fast rhythm: continuous-style pulses, volume follows the full envelope
	ModePulses // rhythm rendered as FOC pulse parameters
	ModeVolume // rhythm rendered through the volume axis
	ModeIFC    // joined: interferential beat rendered as pulse rate
	numModes
)

var modeNames = [numModes]string{"silent", "continuous", "pulses", "volume", "ifc"}

// Result is the mapped output.
type Result struct {
	Axes     []*funscript.Axis
	Warnings []string
	// ModeTime is the fraction of time spent in each rendering mode.
	ModeTime map[string]float64
	// Multiplexed is the fraction of time the A/B pairs were alternated.
	Multiplexed float64
}

type chanState struct {
	level    float64 // intensity driving volume/position
	carrier  float64 // original carrier (Hz)
	mode     int
	pf, w, r float64 // FOC pulse frequency (Hz), width (cycles), rise (cycles)
	rnd      float64
}

type counters struct {
	n                 int
	carrierLow, carHi int
	widthClamp        int
	rateConflict      int
	riseClamp         int
}

// Map converts features into axes.
func Map(f *analysis.Features, cfg config.Config) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	step := cfg.StepMS / 1000
	n := int(f.Duration/step) + 1
	times := make([]float64, n)
	vol := make([]float64, n)
	carrier := make([]float64, n)
	pf := make([]float64, n)
	pw := make([]float64, n)
	pr := make([]float64, n)
	rnd := make([]float64, n)
	var e [4][]float64
	for i := range e {
		e[i] = make([]float64, n)
	}
	alpha := make([]float64, n)
	beta := make([]float64, n)
	var modeCount [numModes]int
	var cnt counters
	tau := cfg.TauUS * 1e-6
	foc := config.FOCLimits
	prevAlpha, prevBeta := 0.0, 0.0
	var prevE = [4]float64{1, 1, 1, 1}

	// pass 1: per-channel states and A/B independence
	states := make([][2]chanState, n)
	indep := make([]bool, n)
	for i := 0; i < n; i++ {
		t := float64(i) * step
		times[i] = t
		for c := 0; c < 2; c++ {
			states[i][c] = channelAt(f, c, t, step, cfg, tau, &cnt)
		}
		indep[i] = independent(f, states[i], t, cfg)
	}
	if cfg.Overlap == "auto" {
		indep = majority(indep, int(math.Round(cfg.AutoWindowS/step)))
	}
	muxSteps := 0

	// pass 2: render
	for i := 0; i < n; i++ {
		t := times[i]
		cs := states[i]
		dom := 0
		if cs[1].level > cs[0].level {
			dom = 1
		}
		lA, lB := cs[0].level, cs[1].level
		v := math.Max(lA, lB)
		mux := cfg.Topology == "dual" && v > 0 &&
			((cfg.Overlap == "multiplex" && math.Min(lA, lB) > 0.05*v) ||
				(cfg.Overlap == "auto" && indep[i] && math.Min(lA, lB) > 0.2*v))

		// shared carrier / pulse parameters
		var sh chanState
		switch cfg.Params {
		case "dominant", "constant":
			sh = cs[dom]
		case "weighted":
			sh = cs[dom]
			if lA+lB > 0 && cs[0].mode == cs[1].mode {
				wa, wb := lA/(lA+lB), lB/(lA+lB)
				sh.carrier = wa*cs[0].carrier + wb*cs[1].carrier
				sh.w = wa*cs[0].w + wb*cs[1].w
				sh.r = wa*cs[0].r + wb*cs[1].r
				sh.rnd = wa*cs[0].rnd + wb*cs[1].rnd
				if !mux && sh.mode == ModePulses && math.Min(lA, lB) > 0.2*v &&
					math.Abs(cs[0].pf-cs[1].pf) > 0.2*math.Max(cs[0].pf, cs[1].pf) {
					cnt.rateConflict++
				}
			} else if !mux && lA+lB > 0 && cs[1-dom].level > 0.2*v && cs[0].mode != cs[1].mode {
				cnt.rateConflict++
			}
		}

		switch cfg.Topology {
		case "dual":
			var pos [4]float64
			switch {
			case v <= 0:
				pos = prevE
			case cfg.Overlap == "dominant":
				pos = pairPos(dom, 0)
			case mux:
				muxSteps++
				c := 0
				if math.Mod(t*cfg.MuxHz, 1) >= 0.5 {
					c = 1
				}
				pos = pairPos(c, 0)
				v = cs[c].level
				sh.carrier = cs[c].carrier
				sh.mode, sh.pf, sh.w, sh.r, sh.rnd = cs[c].mode, cs[c].pf, cs[c].w, cs[c].r, cs[c].rnd
			default: // blend (also multiplex/auto when A and B are not independent)
				pos = pairPos(dom, math.Min(lA, lB)/v)
			}
			for k := range pos {
				e[k][i] = pos[k]
			}
			prevE = pos
		case "joined":
			ll := analysis.At(f.CLL, analysis.HopRate, t)
			lr := analysis.At(f.CLR, analysis.HopRate, t)
			rr := analysis.At(f.CRR, analysis.HopRate, t)
			if cfg.Position == "ab" {
				// FOC-Stim's A-B edge: currents (1, 1, 0) on outputs A, B, C
				prevAlpha, prevBeta = 0.5, math.Sqrt(3)/2
			} else if a, b, ok := threephase.Inverse(ll, lr, rr); ok && v > 0 {
				prevAlpha, prevBeta = a, b
			}
			alpha[i], beta[i] = prevAlpha, prevBeta
			// one current path: no common on which A and B could beat
			if cfg.IFC == "beat" && cfg.Position != "ab" && math.Min(lA, lB) > 0.2*v && v > 0 {
				beat := physio.Beat(cs[0].carrier, cs[1].carrier)
				if beat >= 3 && beat <= foc["pulse_frequency"].Max {
					fc := clamp(sh.carrier, foc["frequency"])
					sh.mode = ModeIFC
					sh.pf = beat
					sh.w = clamp(0.5*fc/beat, foc["pulse_width"])
					sh.r = clamp(0.25*fc/beat, foc["pulse_rise_time"])
					sh.rnd = 0
				}
			}
		}
		if v <= 0 {
			sh.mode = ModeSilent
		}
		modeCount[sh.mode]++
		vol[i] = v
		carrier[i] = clampCount(sh.carrier, foc["frequency"], &cnt.carrierLow, &cnt.carHi)
		pf[i], pw[i], pr[i], rnd[i] = sh.pf, sh.w, sh.r, sh.rnd
		cnt.n++
	}
	holdThroughSilence(vol, carrier, pf, pw, pr, rnd)
	normalizeVolume(vol, cfg)

	res := &Result{ModeTime: map[string]float64{}, Multiplexed: float64(muxSteps) / float64(n)}
	for m, c := range modeCount {
		res.ModeTime[modeNames[m]] = float64(c) / float64(n)
	}
	add := func(name string, vals []float64) {
		r := cfg.Ranges[name]
		res.Axes = append(res.Axes, &funscript.Axis{Name: name, Min: r.Min, Max: r.Max, Times: times, Values: vals})
	}
	add("volume", vol)
	if cfg.Topology == "dual" {
		for k := 0; k < 4; k++ {
			add(fmt.Sprintf("e%d", k+1), e[k])
		}
	} else {
		add("alpha", alpha)
		add("beta", beta)
	}
	if cfg.Params != "constant" {
		add("frequency", carrier)
		add("pulse_frequency", pf)
		add("pulse_width", pw)
		add("pulse_rise_time", pr)
		add("pulse_interval_random", rnd)
	}
	res.Warnings = append(res.Warnings, cnt.warnings()...)
	res.Warnings = append(res.Warnings, rangeWarnings(res.Axes)...)
	return res, nil
}

// channelAt evaluates one channel at time t.
func channelAt(f *analysis.Features, c int, t, step float64, cfg config.Config, tau float64, cnt *counters) chanState {
	ch := &f.Ch[c]
	foc := config.FOCLimits
	var s chanState
	s.carrier = analysis.At(ch.Carrier, analysis.HopRate, t)
	ff := analysis.At(ch.FormFactor, analysis.HopRate, t)
	if ff <= 0 {
		ff = physio.SineFormFactor
	}
	rate := analysis.At(ch.Rate, analysis.FrameRate, t)
	mode := ModeSteady
	if rate > 0 {
		switch cfg.Rhythm {
		case "pulses":
			mode = ModePulses
		case "volume":
			mode = ModeVolume
		default:
			if physio.Fused(rate, cfg.FusionHz) {
				mode = ModePulses
			} else {
				mode = ModeVolume
			}
		}
	}
	var raw float64
	if mode == ModePulses {
		raw = analysis.At(ch.S, analysis.EnvRate, t)
	} else {
		raw = maxIn(ch.E, analysis.EnvRate, t-step/2, t+step/2)
	}
	if raw <= 0 {
		s.mode = ModeSilent
		s.pf, s.w, s.r = cfg.ContinuousPulseHz, 3, 2
		return s
	}
	if cfg.Intensity == "effective" {
		s.level = physio.Effective(raw, ff, s.carrier, cfg.RefCarrierHz, tau)
	} else {
		s.level = raw
	}
	fc := clamp(s.carrier, foc["frequency"])
	s.mode = mode
	if mode == ModePulses {
		duty := analysis.At(ch.Duty, analysis.FrameRate, t)
		attack := analysis.At(ch.Attack, analysis.FrameRate, t)
		s.pf = clamp(rate, foc["pulse_frequency"])
		w := duty / rate * fc
		s.w = clampCount(w, foc["pulse_width"], nil, &cnt.widthClamp)
		if 0.035*fc < s.w {
			s.w = 0.035 * fc
			cnt.widthClamp++
		}
		s.r = clampCount(attack*fc, foc["pulse_rise_time"], nil, &cnt.riseClamp)
		if j := analysis.At(ch.Jitter, analysis.FrameRate, t); j > 0.05 {
			s.rnd = math.Min(j, 1)
		}
	} else {
		s.pf = cfg.ContinuousPulseHz
		s.w = clamp(0.9*fc/s.pf, foc["pulse_width"])
		s.r = foc["pulse_rise_time"].Min
	}
	return s
}

// independent reports whether A and B carry different content at time t:
// both active and differing in carrier, rhythm or waveform correlation.
// Mono (or merely scaled) content is not independent; alternating pairs
// would only add an artificial rhythm there.
func independent(f *analysis.Features, cs [2]chanState, t float64, cfg config.Config) bool {
	lA, lB := cs[0].level, cs[1].level
	v := math.Max(lA, lB)
	if v <= 0 || math.Min(lA, lB) < 0.2*v {
		return false
	}
	if math.Abs(cs[0].carrier-cs[1].carrier) > 3 {
		return true
	}
	if cs[0].mode != cs[1].mode {
		return true
	}
	if cs[0].mode == ModePulses && math.Abs(cs[0].pf-cs[1].pf) > 0.2*math.Max(cs[0].pf, cs[1].pf) {
		return true
	}
	ll := analysis.At(f.CLL, analysis.HopRate, t)
	lr := analysis.At(f.CLR, analysis.HopRate, t)
	rr := analysis.At(f.CRR, analysis.HopRate, t)
	if ll > 0 && rr > 0 && lr/math.Sqrt(ll*rr) < cfg.AutoCorr {
		return true
	}
	return false
}

// majority smooths a boolean series with a centred majority vote over w
// samples, so the overlap strategy doesn't flicker.
func majority(in []bool, w int) []bool {
	if w <= 1 {
		return in
	}
	n := len(in)
	pre := make([]int, n+1)
	for i, b := range in {
		pre[i+1] = pre[i]
		if b {
			pre[i+1]++
		}
	}
	out := make([]bool, n)
	for i := range out {
		lo, hi := max(i-w/2, 0), min(i+w/2+1, n)
		out[i] = 2*(pre[hi]-pre[lo]) > hi-lo
	}
	return out
}

// pairPos returns the 4-phase position with pair `c` at full power and the
// other pair at `other` (0..1). FOC-Stim constrains positions so that the
// maximum is 1 and the three smallest sum to >= 1; these always satisfy it.
func pairPos(c int, other float64) [4]float64 {
	if c == 0 {
		return [4]float64{1, 1, other, other}
	}
	return [4]float64{other, other, 1, 1}
}

func maxIn(series []float32, rate, t0, t1 float64) float64 {
	i0, i1 := int(t0*rate), int(t1*rate)
	if i0 < 0 {
		i0 = 0
	}
	if i1 >= len(series) {
		i1 = len(series) - 1
	}
	m := 0.0
	for i := i0; i <= i1; i++ {
		if v := float64(series[i]); v > m {
			m = v
		}
	}
	return m
}

func clamp(v float64, r config.Range) float64 {
	return math.Max(r.Min, math.Min(r.Max, v))
}

func clampCount(v float64, r config.Range, lo, hi *int) float64 {
	if v < r.Min {
		if lo != nil {
			*lo++
		}
		return r.Min
	}
	if v > r.Max {
		if hi != nil {
			*hi++
		}
		return r.Max
	}
	return v
}

// holdThroughSilence keeps carrier/pulse parameters constant while silent so
// the parameter axes don't jump around.
func holdThroughSilence(vol []float64, series ...[]float64) {
	last := -1
	for i := range vol {
		if vol[i] > 0 {
			if last < 0 {
				for j := 0; j < i; j++ {
					for _, s := range series {
						s[j] = s[i]
					}
				}
			}
			last = i
			continue
		}
		if last >= 0 {
			for _, s := range series {
				s[i] = s[last]
			}
		}
	}
}

// RefLevel is the level abs normalisation maps to volume 1: cfg.RefLevel, or
// when that is 0 the level of full-scale audio. That is a full-scale square
// wave, the strongest signal the audio can carry, at FOC-Stim's lowest
// carrier: longer phases count as stronger in effective intensity, so no
// playable full-scale signal exceeds it. Its edges drive current spikes
// through the skin capacitance, so it is measured through the circuit model
// the way the analysis measures (peak and mean/peak form factor) rather than
// taken from the steady-state sine gain. Without the circuit model the audio
// is the level itself (peak 1, form factor 1).
func RefLevel(cfg config.Config) float64 {
	if cfg.RefLevel > 0 {
		return cfg.RefLevel
	}
	return fullScaleLevel(cfg, config.FOCLimits["frequency"].Min)
}

// fullScaleLevel is the analysis level of a full-scale square wave at f Hz
// through the circuit model: its peak current, or with effective intensity
// that peak weighted by form factor and phase efficacy like chanState does.
func fullScaleLevel(cfg config.Config, f float64) float64 {
	tau := cfg.TauUS * 1e-6
	if !cfg.Circuit.Enabled {
		if cfg.Intensity != "effective" {
			return 1
		}
		return physio.Effective(1, 1, f, cfg.RefCarrierHz, tau)
	}
	const sr = 48000
	m := circuit.New(cfg.Circuit, sr)
	n := int(0.2 * sr) // settle, then measure the last half
	var peak, sum float64
	for i := 0; i < n; i++ {
		x := 1.0
		if math.Mod(float64(i)*f/sr, 1) >= 0.5 {
			x = -1
		}
		y := math.Abs(m.Step(x))
		if i >= n/2 {
			peak = math.Max(peak, y)
			sum += y
		}
	}
	if peak <= 0 {
		return 1
	}
	if cfg.Intensity != "effective" {
		return peak
	}
	ff := sum / float64(n-n/2) / peak
	return physio.Effective(peak, ff, f, cfg.RefCarrierHz, tau)
}

func normalizeVolume(vol []float64, cfg config.Config) {
	var ref float64
	switch cfg.Normalize {
	case "abs":
		ref = RefLevel(cfg)
	case "peak", "p99":
		var nz []float64
		for _, v := range vol {
			if v > 0 {
				nz = append(nz, v)
			}
		}
		if len(nz) == 0 {
			return
		}
		sort.Float64s(nz)
		ref = nz[len(nz)-1]
		if cfg.Normalize == "p99" {
			ref = nz[len(nz)*99/100]
		}
	}
	if ref <= 0 {
		return
	}
	for i, v := range vol {
		x := math.Min(v/ref, 1)
		if cfg.Gamma != 1 {
			x = math.Pow(x, cfg.Gamma)
		}
		vol[i] = x
	}
}

func (c counters) warnings() []string {
	var w []string
	pct := func(k int) float64 { return 100 * float64(k) / float64(max(c.n, 1)) }
	if c.carHi > 0 {
		w = append(w, fmt.Sprintf("%.1f%% of the time the original carrier exceeds FOC-Stim's %g Hz maximum and was clamped", pct(c.carHi), config.FOCLimits["frequency"].Max))
	}
	if c.carrierLow > 0 {
		w = append(w, fmt.Sprintf("%.1f%% of the time the original carrier is below FOC-Stim's %g Hz minimum and was clamped", pct(c.carrierLow), config.FOCLimits["frequency"].Min))
	}
	if c.widthClamp > 0 {
		w = append(w, fmt.Sprintf("%.1f%% of the time rhythm bursts are longer than FOC-Stim's pulse width limit (20 cycles / 35 ms) and were shortened; --rhythm volume keeps their full length", pct(c.widthClamp)))
	}
	if c.riseClamp > 0 {
		w = append(w, fmt.Sprintf("%.1f%% of the time burst attacks exceed FOC-Stim's rise time limit (10 cycles) and were clamped", pct(c.riseClamp)))
	}
	if c.rateConflict > 0 {
		w = append(w, fmt.Sprintf("%.1f%% of the time A and B have different rhythms; FOC-Stim has one pulse rate, the louder channel's was used (--params)", pct(c.rateConflict)))
	}
	return w
}

// rangeWarnings reports values outside the funscript kit range (they are
// clamped when written) and suggests a wider range.
func rangeWarnings(axes []*funscript.Axis) []string {
	var w []string
	for _, a := range axes {
		lo, hi := math.Inf(1), math.Inf(-1)
		out := 0
		for _, v := range a.Values {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
			if v < a.Min-1e-9 || v > a.Max+1e-9 {
				out++
			}
		}
		if out > 0 {
			w = append(w, fmt.Sprintf("%s: %.1f%% of values outside the funscript range %g..%g (data spans %.4g..%.4g); widen with --range %s=%.4g:%.4g and set the same range in restim's funscript kit",
				a.Name, 100*float64(out)/float64(len(a.Values)), a.Min, a.Max, lo, hi, a.Name, math.Min(lo, a.Min), math.Max(hi, a.Max)))
		}
	}
	return w
}
