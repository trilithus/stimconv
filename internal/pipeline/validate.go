package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/trilithus/stimconv/internal/analysis"
	"github.com/trilithus/stimconv/internal/funscript"
)

// Validation thresholds.
const (
	// output silent this much more often than the input → signal was lost
	maxSilenceGap = 0.2
	// input peak this far above its 99.5th percentile → one spike dominates
	spikeDB = 20
	// restim sends at 60 Hz over a 115200-baud UART (~520 move_to/s at
	// ~20 B each); warn well before that
	restimTickHz   = 60
	maxRestimMsgPS = 400
)

// writtenAxis is one funscript as it ended up on disk.
type writtenAxis struct {
	Name    string
	Actions []funscript.Action
}

// validate checks the written funscripts against the input analysis and
// returns the problems found. It looks for what a careful person would
// notice by comparing input and output: lost signal, flat output, broken
// files and a stream restim could not deliver.
func validate(raw *analysis.Raw, silenceDB float64, axes []writtenAxis) []string {
	var issues []string
	dur := raw.Duration

	for _, a := range axes {
		issues = append(issues, checkStructure(a, dur)...)
	}

	in, ref, peak := inputActivity(raw, silenceDB)
	if peak > 0 && ref > 0 {
		if r := 20 * math.Log10(peak/ref); r > spikeDB {
			issues = append(issues, fmt.Sprintf(
				"the input's peak is %.0f dB above its 99.5th percentile: a short spike or glitch dominates the level, check the source around the loudest moment", r))
		}
	}

	var vol *writtenAxis
	for i := range axes {
		if axes[i].Name == "volume" {
			vol = &axes[i]
		}
	}
	if vol != nil && len(vol.Actions) > 0 && in > 0.05 {
		out := activeFraction(vol.Actions, dur)
		if in-out > maxSilenceGap {
			issues = append(issues, fmt.Sprintf(
				"the input carries signal %.0f%% of the time but the output volume is above zero only %.0f%%: signal was lost in the conversion", 100*in, 100*out))
		}
		if flat(vol.Actions) {
			issues = append(issues, fmt.Sprintf(
				"the volume never changes although the input carries signal %.0f%% of the time", 100*in))
		}
	}

	if peak, at := restimPeakRate(axes, dur); peak > maxRestimMsgPS {
		issues = append(issues, fmt.Sprintf(
			"restim would send up to %d axis updates/s (at %.0f s), close to the FOC-Stim link limit of ~520/s; expect stutter on WiFi", peak, at))
	}
	return issues
}

func checkStructure(a writtenAxis, dur float64) []string {
	if len(a.Actions) == 0 {
		return []string{a.Name + ": funscript has no points"}
	}
	var issues []string
	for i, p := range a.Actions {
		if p.Pos < 0 || p.Pos > 100 {
			issues = append(issues, fmt.Sprintf("%s: position %d at %d ms is outside 0..100", a.Name, p.Pos, p.At))
			break
		}
		if p.At < 0 || (i > 0 && p.At <= a.Actions[i-1].At) {
			issues = append(issues, fmt.Sprintf("%s: timestamps not strictly increasing at %d ms", a.Name, p.At))
			break
		}
	}
	if end := float64(a.Actions[len(a.Actions)-1].At) / 1000; end < dur-1 {
		issues = append(issues, fmt.Sprintf("%s: ends at %.1f s, the input runs %.1f s", a.Name, end, dur))
	}
	return issues
}

// inputActivity returns the fraction of 1 ms buckets above the silence
// threshold relative to a robust reference (99.5th percentile of the louder
// channel, so a spike cannot set it), the reference and the true peak.
func inputActivity(raw *analysis.Raw, silenceDB float64) (frac, ref, peak float64) {
	n := min(len(raw.Bucket[0]), len(raw.Bucket[1]))
	if n == 0 {
		return 0, 0, 0
	}
	lvl := make([]float64, n)
	for i := range n {
		lvl[i] = math.Max(float64(raw.Bucket[0][i]), float64(raw.Bucket[1][i]))
		peak = math.Max(peak, lvl[i])
	}
	sorted := append([]float64(nil), lvl...)
	sort.Float64s(sorted)
	ref = sorted[int(0.995*float64(n-1))]
	gate := ref * math.Pow(10, silenceDB/20)
	active := 0
	for _, v := range lvl {
		if v > gate && v > 0 {
			active++
		}
	}
	return float64(active) / float64(n), ref, peak
}

// sampleAt walks a funscript at a fixed rate, linearly interpolated as
// restim plays it, calling fn with each tick's position.
func sampleAt(acts []funscript.Action, dur, hz float64, fn func(tick int, pos float64)) {
	j := 0
	for k := 0; float64(k)/hz < dur; k++ {
		ms := float64(k) / hz * 1000
		for j+1 < len(acts) && float64(acts[j+1].At) <= ms {
			j++
		}
		pos := float64(acts[j].Pos)
		if j+1 < len(acts) && float64(acts[j].At) < ms {
			a, b := acts[j], acts[j+1]
			pos += float64(b.Pos-a.Pos) * (ms - float64(a.At)) / float64(b.At-a.At)
		}
		fn(k, pos)
	}
}

func activeFraction(acts []funscript.Action, dur float64) float64 {
	on, all := 0, 0
	sampleAt(acts, dur, 100, func(_ int, pos float64) {
		all++
		if pos > 0 {
			on++
		}
	})
	if all == 0 {
		return 0
	}
	return float64(on) / float64(all)
}

func flat(acts []funscript.Action) bool {
	for _, p := range acts[1:] {
		if p.Pos != acts[0].Pos {
			return false
		}
	}
	return true
}

// restimPeakRate simulates restim's FOC-Stim sender: every 60 Hz tick sends
// the axes whose value changed, and once a second it resends all of them.
// It returns the busiest second's message count and when it started.
func restimPeakRate(axes []writtenAxis, dur float64) (int, float64) {
	secs := int(math.Ceil(dur))
	if secs == 0 {
		return 0, 0
	}
	perSec := make([]int, secs+1)
	for _, a := range axes {
		if len(a.Actions) == 0 {
			continue
		}
		last := math.NaN()
		sampleAt(a.Actions, dur, restimTickHz, func(k int, pos float64) {
			if pos != last || k%restimTickHz == 0 {
				perSec[k/restimTickHz]++
			}
			last = pos
		})
	}
	peak, at := 0, 0
	for s, n := range perSec {
		if n > peak {
			peak, at = n, s
		}
	}
	return peak, float64(at)
}
