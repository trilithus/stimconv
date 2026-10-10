package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Wiring estimates whether a track was authored for a shared common electrode
// (3-phase, restim "joined") or for two isolated pairs (4-phase, "dual").
//
// The physical argument: on isolated pairs the relative phase of A and B has
// no electrical effect, so deliberate phase content (a steady offset other
// than 0°/180°, or carriers detuned by a few Hz so the phase rotates) only
// makes sense with a common electrode, where it moves the current between the
// three electrodes. Conversely, A and B with differing envelopes or taking
// turns are separate content, which suits isolated pairs (though a stereostim
// box with a common plays it too). Mono content works either way.
type Wiring struct {
	Active float64 // share of the track with any signal

	// shares of the active time
	Mono      float64 // same carrier, in phase (or antiphase), same envelope
	Phased    float64 // same carrier with a phase offset or slowly rotating phase
	EnvDiff   float64 // same carrier, different envelopes
	Detuned   float64 // carriers too far apart to stay coherent within 10 ms
	OneSided  float64 // only A or only B active
	Antiphase float64 // part of Mono with B ≈ −A (no current through a common)

	Verdict WiringVerdict
}

// WiringVerdict is the conclusion of Wiring.
type WiringVerdict int

const (
	WiringUnknown WiringVerdict = iota // too little signal
	WiringThree                        // phase-coded: authored for a common electrode
	WiringFour                         // separate A/B content: suits isolated pairs
	WiringMono                         // A ≡ ±B: works either way
	WiringDetuned                      // different carriers: beat on joined, two sensations on dual
	WiringMixed                        // no clear pattern
)

// Topology is the matching config.Topology value, "" when either works.
func (v WiringVerdict) Topology() string {
	switch v {
	case WiringThree:
		return "joined"
	case WiringFour:
		return "dual"
	}
	return ""
}

// singleChannelMono is the mono share from which a track counts as mono
// throughout (a mono file decodes to L = R and scores 100%).
const singleChannelMono = 0.9

// SingleChannel reports a mono track. The original box played those on one
// channel with two electrodes, which config.Position "ab" reproduces.
func (w Wiring) SingleChannel() bool {
	return w.Verdict == WiringMono && w.Mono >= singleChannelMono
}

func (w Wiring) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "A/B relation (of active time): mono %.0f%%", 100*w.Mono)
	if w.Antiphase > 0.01 {
		fmt.Fprintf(&b, " (antiphase %.0f%%)", 100*w.Antiphase)
	}
	fmt.Fprintf(&b, ", phase-coded %.0f%%, different envelopes %.0f%%, detuned carriers %.0f%%, one channel only %.0f%%\n",
		100*w.Phased, 100*w.EnvDiff, 100*w.Detuned, 100*w.OneSided)
	b.WriteString("intended wiring: ")
	switch w.Verdict {
	case WiringUnknown:
		b.WriteString("unknown (too little signal)")
	case WiringThree:
		b.WriteString("3-phase (common electrode): A/B phase carries position, which isolated pairs ignore; topology joined keeps it")
	case WiringFour:
		b.WriteString("likely 4-phase (two pairs): A and B carry separate content; topology dual keeps it apart")
	case WiringMono:
		if w.SingleChannel() {
			b.WriteString("mono: one channel with two electrodes on the original box; preset [mono-original] (FOC-Stim outputs A and B) reproduces that")
		} else {
			b.WriteString("either: A and B carry the same signal")
		}
	case WiringDetuned:
		b.WriteString("either: different carriers on A and B (an interferential beat with joined, two separate sensations with dual)")
	default:
		b.WriteString("unclear: no dominant A/B pattern")
	}
	return b.String()
}

const (
	wireBlockHops = HopRate / 2 // 0.5 s blocks
	wireCoh       = 0.7         // 10 ms coherence of one carrier (|Δf| ≲ 30 Hz)
	wirePhaseDeg  = 15          // steady offsets beyond 0°/180° ± this count as phase-coded
	wireSpread    = 0.1         // phase spread within a block (1−|Σz|/Σ|z|, ±25°) that counts as rotating
	wireEnvDiff   = 0.15        // envelope shape distance (0..1) for "different envelopes"
	wireOneSided  = 0.1         // the quieter channel below this share of the louder (-20 dB)
	wireSilent    = 0.05        // block level below this share of the 99.5th percentile (-26 dB)
)

// AnalyseWiring classifies 0.5 s blocks of raw by their A/B relationship.
func AnalyseWiring(raw *Raw) Wiring {
	n := min(len(raw.CLL), len(raw.CLQ)) / wireBlockHops
	type block struct {
		la, lb, coh, phase, spread, env float64
	}
	bl := make([]block, n)
	lv := make([]float64, n)
	for b := range bl {
		var ll, rr, lr, lq, cohW, wSum, zAbs float64
		for i := b * wireBlockHops; i < (b+1)*wireBlockHops; i++ {
			l, r := float64(raw.CLL[i]), float64(raw.CRR[i])
			z := complex(float64(raw.CLR[i]), float64(raw.CLQ[i]))
			ll, rr, lr, lq = ll+l, rr+r, lr+real(z), lq+imag(z)
			if l > 0 && r > 0 {
				w := math.Sqrt(l * r)
				cohW += math.Min(math.Hypot(real(z), imag(z))/w, 1) * w
				wSum += w
				zAbs += math.Hypot(real(z), imag(z))
			}
		}
		k := &bl[b]
		k.la, k.lb = math.Sqrt(ll/wireBlockHops), math.Sqrt(rr/wireBlockHops)
		lv[b] = math.Max(k.la, k.lb)
		if wSum > 0 {
			// per-hop coherence, so differing envelopes don't lower it
			k.coh = cohW / wSum
		}
		if ll > 0 && rr > 0 {
			k.phase = math.Abs(math.Atan2(lq, lr)) * 180 / math.Pi
		}
		if zAbs > 0 {
			// how far the phase spreads within the block: 0 for a steady
			// phase, near 1 for a carrier detuned by a few Hz
			k.spread = 1 - math.Hypot(lr, lq)/zAbs
		}
		k.env = envDistance(raw, b)
	}
	var w Wiring
	ref := percentile(lv, 0.995)
	if n == 0 || ref <= 0 {
		return w
	}
	var active, mono, anti, phased, envd, detuned, one float64
	for _, k := range bl {
		m := math.Max(k.la, k.lb)
		if m < wireSilent*ref {
			continue
		}
		active++
		switch {
		case math.Min(k.la, k.lb) < wireOneSided*m:
			one++
		case k.coh < wireCoh:
			detuned++
		case k.env > wireEnvDiff:
			envd++
		case k.spread > wireSpread || (k.phase > wirePhaseDeg && k.phase < 180-wirePhaseDeg):
			phased++
		default:
			mono++
			if k.phase >= 90 {
				anti++
			}
		}
	}
	w.Active = active / float64(n)
	if active < 10 {
		return w
	}
	w.Mono, w.Antiphase, w.Phased = mono/active, anti/active, phased/active
	w.EnvDiff, w.Detuned, w.OneSided = envd/active, detuned/active, one/active
	sep := w.EnvDiff + w.OneSided
	switch {
	case w.Phased >= 0.15:
		w.Verdict = WiringThree
	case sep >= 0.2 && w.Phased < 0.05:
		w.Verdict = WiringFour
	case w.Mono >= 0.6:
		w.Verdict = WiringMono
	case w.Detuned >= 0.5:
		w.Verdict = WiringDetuned
	default:
		w.Verdict = WiringMixed
	}
	return w
}

// envDistance compares the shapes of the 1 kHz envelopes in block b:
// half the L1 distance of the normalised envelopes, 0 = same shape, 1 = disjoint.
func envDistance(raw *Raw, b int) float64 {
	per := EnvRate * wireBlockHops / HopRate
	lo, hi := b*per, min((b+1)*per, len(raw.Bucket[0]), len(raw.Bucket[1]))
	var ma, mb float64
	for i := lo; i < hi; i++ {
		ma += float64(raw.Bucket[0][i])
		mb += float64(raw.Bucket[1][i])
	}
	if ma <= 0 || mb <= 0 {
		return 0
	}
	var d float64
	for i := lo; i < hi; i++ {
		d += math.Abs(float64(raw.Bucket[0][i])/ma - float64(raw.Bucket[1][i])/mb)
	}
	return d / 2
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(p*float64(len(s)-1))]
}
