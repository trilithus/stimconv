// Package analysis extracts electrical features from the drive signal:
// carrier frequency, waveform form factor, peak-current envelope, A/B
// covariance, and (offline) the rhythm of the envelope.
package analysis

import (
	"errors"
	"io"
	"math"

	"github.com/trilithus/stimconv/internal/circuit"
	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
)

const (
	// EnvRate is the envelope sample rate (Hz).
	EnvRate = 1000
	// HopRate is the rate of carrier/covariance measurements (Hz).
	HopRate = 100
	// minCarrierHz rejects slow zero-crossings (noise, DC wander).
	minCarrierHz = 150
)

// Raw holds the streaming measurements of both channels.
type Raw struct {
	SampleRate int
	Duration   float64 // seconds
	Units      string  // "A" (circuit model) or "fs" (full-scale audio)

	// 1 kHz, per channel: max |x| within each 1 ms bucket
	Bucket [2][]float32

	// HopRate, per channel
	Carrier    [2][]float32 // Hz, 0 when no valid half-cycles
	FormFactor [2][]float32 // mean|x| / peak over half-cycles
	// HopRate, stereo covariance of the (current) waveforms
	CLL, CLR, CRR []float32
}

type chanTracker struct {
	sign       int
	runPeak    float64
	decay      float64
	start      int
	halfPeak   float64
	halfCharge float64 // sum |x|
	// hop accumulators
	nHalf      int
	sumDur     float64
	sumCharge  float64
	sumPeakDur float64
	bucketMax  float64
}

func (c *chanTracker) sample(x float64, i int, sr float64) {
	a := math.Abs(x)
	if a > c.bucketMax {
		c.bucketMax = a
	}
	c.runPeak = math.Max(a, c.runPeak*c.decay)
	h := 0.02 * c.runPeak
	s := c.sign
	if x > h {
		s = 1
	} else if x < -h {
		s = -1
	}
	if s != c.sign && s != 0 {
		if c.sign != 0 {
			dur := float64(i-c.start) / sr
			if dur > 0 && dur < 1.0/(2*minCarrierHz) && c.halfPeak > 0.05*c.runPeak {
				c.nHalf++
				c.sumDur += dur
				c.sumCharge += c.halfCharge / sr
				c.sumPeakDur += c.halfPeak * dur
			}
		}
		c.sign, c.start, c.halfPeak, c.halfCharge = s, i, 0, 0
	}
	if a > c.halfPeak {
		c.halfPeak = a
	}
	c.halfCharge += a
}

func (c *chanTracker) hop() (carrier, ff float32) {
	if c.nHalf >= 2 && c.sumDur > 0 {
		carrier = float32(float64(c.nHalf) / (2 * c.sumDur))
		if c.sumPeakDur > 0 {
			ff = float32(c.sumCharge / c.sumPeakDur)
		}
	}
	c.nHalf, c.sumDur, c.sumCharge, c.sumPeakDur = 0, 0, 0, 0
	return
}

// Measure streams the source through the (optional) circuit model and the
// trackers.
func Measure(src decode.Source, cc config.Circuit) (*Raw, error) {
	sr := src.SampleRate()
	fsr := float64(sr)
	raw := &Raw{SampleRate: sr, Units: "fs"}
	var models [2]*circuit.Model
	if cc.Enabled {
		raw.Units = "A"
		models = [2]*circuit.Model{circuit.New(cc, sr), circuit.New(cc, sr)}
	}
	// DC blocker when no circuit model (the transformer already blocks DC)
	dcR := math.Exp(-2 * math.Pi * 5 / fsr)
	var dcX, dcY [2]float64

	var tr [2]chanTracker
	for c := range tr {
		tr[c].decay = math.Exp(-1 / (2 * fsr))
	}
	bucketLen := sr / EnvRate
	hopLen := sr / HopRate
	var sLL, sLR, sRR float64

	buf := make([]float32, 2*8192)
	idx := 0
	for {
		n, err := src.Read(buf)
		for k := 0; k < n; k++ {
			var y [2]float64
			for c := 0; c < 2; c++ {
				x := float64(buf[2*k+c])
				if models[c] != nil {
					y[c] = models[c].Step(x)
				} else {
					dcY[c] = x - dcX[c] + dcR*dcY[c]
					dcX[c] = x
					y[c] = dcY[c]
				}
				tr[c].sample(y[c], idx, fsr)
			}
			sLL += y[0] * y[0]
			sLR += y[0] * y[1]
			sRR += y[1] * y[1]
			idx++
			if idx%bucketLen == 0 {
				for c := 0; c < 2; c++ {
					raw.Bucket[c] = append(raw.Bucket[c], float32(tr[c].bucketMax))
					tr[c].bucketMax = 0
				}
			}
			if idx%hopLen == 0 {
				for c := 0; c < 2; c++ {
					f, ff := tr[c].hop()
					raw.Carrier[c] = append(raw.Carrier[c], f)
					raw.FormFactor[c] = append(raw.FormFactor[c], ff)
				}
				inv := 1 / float64(hopLen)
				raw.CLL = append(raw.CLL, float32(sLL*inv))
				raw.CLR = append(raw.CLR, float32(sLR*inv))
				raw.CRR = append(raw.CRR, float32(sRR*inv))
				sLL, sLR, sRR = 0, 0, 0
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if n == 0 {
			break
		}
	}
	raw.Duration = float64(idx) / fsr
	return raw, nil
}
