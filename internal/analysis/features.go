package analysis

import (
	"math"
	"sort"
)

// FrameRate is the rate of rhythm analysis frames (Hz).
const FrameRate = 5

// Channel holds the derived features of one channel.
type Channel struct {
	E []float32 // 1 kHz peak envelope (silence gated)
	S []float32 // 1 kHz slow envelope (below ModSplitHz)
	F []float32 // 1 kHz fast envelope E/S, 0..1

	Carrier    []float32 // HopRate, smoothed, 0 = unknown
	FormFactor []float32 // HopRate

	// FrameRate rhythm descriptors
	Rate   []float32 // Hz, 0 = no rhythm (steady); detected between the split frequency and 100 Hz
	Duty   []float32 // fraction of the rhythm period the envelope is "on"
	Attack []float32 // seconds, 10%->90% rise
	Jitter []float32 // std/mean of rhythm intervals
	Depth  []float32 // modulation depth of F (p95-p5)
}

// Features is the complete analysis result.
type Features struct {
	Duration float64
	Units    string
	Ch       [2]Channel
	// HopRate stereo covariance, lightly smoothed
	CLL, CLR, CRR []float32
}

// Derive computes envelopes and rhythm descriptors.
func Derive(raw *Raw, modSplitHz, silenceDB float64) *Features {
	f := &Features{Duration: raw.Duration, Units: raw.Units}
	// global gate relative to the louder channel's max
	gmax := float32(0)
	for c := 0; c < 2; c++ {
		for _, v := range raw.Bucket[c] {
			if v > gmax {
				gmax = v
			}
		}
	}
	gate := gmax * float32(math.Pow(10, silenceDB/20))
	for c := 0; c < 2; c++ {
		ch := &f.Ch[c]
		b := raw.Bucket[c]
		n := len(b)
		ch.E = make([]float32, n)
		for i := range b {
			// centred 3 ms max bridges half-cycles up to 1.5 ms (carriers >= ~330 Hz)
			m := b[i]
			if i > 0 && b[i-1] > m {
				m = b[i-1]
			}
			if i+1 < n && b[i+1] > m {
				m = b[i+1]
			}
			if m < gate {
				m = 0
			}
			ch.E[i] = m
		}
		win := int(math.Round(EnvRate / modSplitHz))
		ch.S = movingAvg(movingMax(ch.E, win), win)
		ch.F = make([]float32, n)
		for i := range ch.E {
			if ch.S[i] > gate && ch.S[i] > 0 {
				v := ch.E[i] / ch.S[i]
				if v > 1 {
					v = 1
				}
				ch.F[i] = v
			}
		}
		ch.Carrier = smoothCarrier(raw.Carrier[c], 25)
		ch.FormFactor = raw.FormFactor[c]
		rhythm(ch, win)
	}
	f.CLL = movingAvg(raw.CLL, 3)
	f.CLR = movingAvg(raw.CLR, 3)
	f.CRR = movingAvg(raw.CRR, 3)
	return f
}

// smoothCarrier fills gaps (no half-cycles, e.g. between bursts) by holding
// the last valid value and takes a running median over w hops.
func smoothCarrier(in []float32, w int) []float32 {
	out := make([]float32, len(in))
	held := float32(0)
	filled := make([]float32, len(in))
	for i, v := range in {
		if v > 0 {
			held = v
		}
		filled[i] = held
	}
	// back-fill the leading gap
	first := float32(0)
	for _, v := range filled {
		if v > 0 {
			first = v
			break
		}
	}
	for i := range filled {
		if filled[i] > 0 {
			break
		}
		filled[i] = first
	}
	tmp := make([]float32, 0, w)
	for i := range filled {
		lo, hi := i-w/2, i+w/2+1
		if lo < 0 {
			lo = 0
		}
		if hi > len(filled) {
			hi = len(filled)
		}
		tmp = append(tmp[:0], filled[lo:hi]...)
		sort.Slice(tmp, func(a, b int) bool { return tmp[a] < tmp[b] })
		out[i] = tmp[len(tmp)/2]
	}
	return out
}

// rhythm fills the FrameRate descriptors from F.
// maxLag is the longest rhythm period searched (ms); rhythms slower than the
// slow/fast split live in S, not F.
func rhythm(ch *Channel, maxLag int) {
	n := len(ch.F)
	step := EnvRate / FrameRate
	const winLen = 1000 // 1 s analysis window
	const minLag = 10
	frames := (n + step - 1) / step
	ch.Rate = make([]float32, frames)
	ch.Duty = make([]float32, frames)
	ch.Attack = make([]float32, frames)
	ch.Jitter = make([]float32, frames)
	ch.Depth = make([]float32, frames)
	x := make([]float64, winLen)
	tmp := make([]float64, winLen)
	r := make([]float64, maxLag+2)
	for fi := 0; fi < frames; fi++ {
		c := fi*step + step/2
		lo := c - winLen/2
		if lo < 0 {
			lo = 0
		}
		hi := lo + winLen
		if hi > n {
			hi = n
			lo = hi - winLen
			if lo < 0 {
				lo = 0
			}
		}
		m := hi - lo
		if m < 4*minLag {
			continue
		}
		active := 0
		mean := 0.0
		for i := 0; i < m; i++ {
			x[i] = float64(ch.F[lo+i])
			if ch.S[lo+i] > 0 {
				active++
			}
			mean += x[i]
		}
		if active < m/2 {
			continue
		}
		mean /= float64(m)
		copy(tmp[:m], x[:m])
		sort.Float64s(tmp[:m])
		depth := tmp[m*95/100] - tmp[m*5/100]
		ch.Depth[fi] = float32(depth)
		if depth < 0.35 {
			continue // steady envelope
		}
		// normalised autocorrelation
		var e0 float64
		for i := 0; i < m; i++ {
			x[i] -= mean
			e0 += x[i] * x[i]
		}
		if e0 == 0 {
			continue
		}
		best := 0.0
		for lag := minLag; lag <= maxLag+1 && lag < m/2; lag++ {
			s := 0.0
			for i := 0; i+lag < m; i++ {
				s += x[i] * x[i+lag]
			}
			r[lag] = s / e0 * float64(m) / float64(m-lag)
			if lag <= maxLag && r[lag] > best {
				best = r[lag]
			}
		}
		if best < 0.3 {
			continue
		}
		// smallest local maximum reaching 85% of the best (avoid sub-harmonics)
		lagF := 0.0
		for lag := minLag + 1; lag < maxLag && lag+1 < m/2; lag++ {
			if r[lag] >= 0.85*best && r[lag] >= r[lag-1] && r[lag] >= r[lag+1] {
				// parabolic refinement
				den := r[lag-1] - 2*r[lag] + r[lag+1]
				d := 0.0
				if den != 0 {
					d = 0.5 * (r[lag-1] - r[lag+1]) / den
				}
				lagF = float64(lag) + d
				break
			}
		}
		if lagF <= 0 {
			continue
		}
		rate := EnvRate / lagF
		ch.Rate[fi] = float32(rate)
		// duty, onsets, attack
		on := 0
		var onsets []int
		attackSum, attackN := 0.0, 0
		for i := 0; i < m; i++ {
			v := ch.F[lo+i]
			if v > 0.5 {
				on++
			}
			if i > 0 && ch.F[lo+i-1] <= 0.5 && v > 0.5 {
				onsets = append(onsets, i)
				// walk back to 10% and forward to 90%
				a := i
				for a > 0 && ch.F[lo+a] > 0.1 && i-a < int(lagF) {
					a--
				}
				b := i
				for b < m-1 && ch.F[lo+b] < 0.9 && b-i < int(lagF) {
					b++
				}
				attackSum += float64(b - a)
				attackN++
			}
		}
		ch.Duty[fi] = float32(on) / float32(m)
		if attackN > 0 {
			ch.Attack[fi] = float32(attackSum / float64(attackN) / EnvRate)
		}
		if len(onsets) >= 3 {
			var iv []float64
			for k := 1; k < len(onsets); k++ {
				iv = append(iv, float64(onsets[k]-onsets[k-1]))
			}
			mu, sd := meanStd(iv)
			if mu > 0 {
				ch.Jitter[fi] = float32(sd / mu)
			}
		}
	}
}

func meanStd(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	mu := 0.0
	for _, x := range v {
		mu += x
	}
	mu /= float64(len(v))
	s := 0.0
	for _, x := range v {
		s += (x - mu) * (x - mu)
	}
	return mu, math.Sqrt(s / float64(len(v)))
}

// movingMax is a centred running maximum (monotonic deque, O(n)).
func movingMax(in []float32, w int) []float32 {
	n := len(in)
	out := make([]float32, n)
	if w < 1 {
		copy(out, in)
		return out
	}
	half := w / 2
	dq := make([]int, 0, w+1)
	j := 0 // next index to push
	for i := 0; i < n; i++ {
		hi := i + half
		for ; j <= hi && j < n; j++ {
			for len(dq) > 0 && in[dq[len(dq)-1]] <= in[j] {
				dq = dq[:len(dq)-1]
			}
			dq = append(dq, j)
		}
		for len(dq) > 0 && dq[0] < i-half {
			dq = dq[1:]
		}
		out[i] = in[dq[0]]
	}
	return out
}

// movingAvg is a centred running mean.
func movingAvg(in []float32, w int) []float32 {
	n := len(in)
	out := make([]float32, n)
	if w <= 1 || n == 0 {
		copy(out, in)
		return out
	}
	half := w / 2
	pre := make([]float64, n+1)
	for i, v := range in {
		pre[i+1] = pre[i] + float64(v)
	}
	for i := range out {
		lo, hi := i-half, i+half+1
		if lo < 0 {
			lo = 0
		}
		if hi > n {
			hi = n
		}
		out[i] = float32((pre[hi] - pre[lo]) / float64(hi-lo))
	}
	return out
}

// At helpers: sample a series at time t (seconds) for a given rate.
func At(series []float32, rate float64, t float64) float64 {
	if len(series) == 0 {
		return 0
	}
	i := int(t * rate)
	if i < 0 {
		i = 0
	}
	if i >= len(series) {
		i = len(series) - 1
	}
	return float64(series[i])
}
