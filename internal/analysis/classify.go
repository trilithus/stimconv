package analysis

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/cmplx"

	"github.com/trilithus/stimconv/internal/decode"
)

// Kind is the verdict of Classify.
type Kind int

const (
	KindStim      Kind = iota // carrier-like drive signal
	KindUncertain             // mixed evidence
	KindAudio                 // looks like music or speech
	KindSilent                // too little signal to judge
)

// Check is the result of Classify.
type Check struct {
	Kind     Kind
	StimFrac float64 // share of sampled active frames that look like a drive signal
	Frames   int     // active frames examined
	LowFreq  float64 // median share of energy below 150 Hz
	Peak     float64 // median share of energy in the two strongest narrow peaks
}

func (c Check) String() string {
	switch c.Kind {
	case KindSilent:
		return "too little signal at the start to check the content"
	case KindStim:
		return fmt.Sprintf("looks like a stim drive signal (carrier-like in %.0f%% of sampled frames)", 100*c.StimFrac)
	case KindAudio:
		return fmt.Sprintf("this looks like music or speech, not a stim drive signal: only %.0f%% of sampled frames are carrier-like, %.0f%% of the energy is below 150 Hz", 100*c.StimFrac, 100*c.LowFreq)
	}
	return fmt.Sprintf("this may not be a stim drive signal: %.0f%% of sampled frames are carrier-like", 100*c.StimFrac)
}

const (
	clsN        = 4096 // FFT size
	clsHopS     = 0.5  // seconds between analysed frames
	clsMinRMS   = 3e-3 // ~ -50 dBFS: quieter frames are skipped
	clsLowHz    = 150  // drive carriers sit above this; bass and voice below
	clsPeakHz   = 60   // half-width of a "narrow" peak (keeps AM sidebands)
	clsMaxLow   = 0.05
	clsMinPeak  = 0.6
	clsStimFrac = 0.6
	clsAudioFr  = 0.25
	clsMinFrame = 6
)

// CheckFile opens path and classifies its first maxSeconds (see Classify).
func CheckFile(path string, opt decode.Options, maxSeconds float64) (Check, error) {
	src, _, err := decode.Open(path, opt)
	if err != nil {
		return Check{}, err
	}
	defer src.Close() // stopping ffmpeg early makes Close report an error; ignored
	return Classify(src, maxSeconds)
}

// Classify estimates from the first maxSeconds of src whether it is a stim
// drive signal or ordinary audio. A drive signal is a carrier: nearly all
// energy sits in one or two narrow spectral peaks (fundamental and odd
// harmonics) and almost none below 150 Hz. Music and speech are broadband
// with strong low-frequency content. It reads from src (and consumes it).
// Limitation: a nearly pure melody without bass (whistling, a solo flute)
// can pass as a carrier; this is only used for a warning.
func Classify(src decode.Source, maxSeconds float64) (Check, error) {
	sr := float64(src.SampleRate())
	hop := int(clsHopS * sr)
	if hop < clsN {
		hop = clsN
	}
	limit := int(maxSeconds * sr)
	win := make([]float64, clsN)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(clsN-1))
	}
	var frame [2][]float64
	frame[0], frame[1] = make([]float64, 0, clsN), make([]float64, 0, clsN)
	buf := make([]float32, 8192)
	pos := 0 // frame index within the file
	var stim, active int
	var lows, peaks []float64
	for pos < limit {
		n, err := src.Read(buf)
		for i := 0; i < n; i++ {
			k := pos % hop
			pos++
			if k >= clsN {
				continue
			}
			frame[0] = append(frame[0], float64(buf[2*i]))
			frame[1] = append(frame[1], float64(buf[2*i+1]))
			if len(frame[0]) < clsN {
				continue
			}
			ok, any := true, false
			for c := 0; c < 2; c++ {
				low, peak, act := spectrumShape(frame[c], win, sr)
				if !act {
					continue
				}
				any = true
				lows, peaks = append(lows, low), append(peaks, peak)
				if low > clsMaxLow || peak < clsMinPeak {
					ok = false
				}
			}
			if any {
				active++
				if ok {
					stim++
				}
			}
			frame[0], frame[1] = frame[0][:0], frame[1][:0]
		}
		if errors.Is(err, io.EOF) || n == 0 {
			break
		}
		if err != nil {
			return Check{}, err
		}
	}
	c := Check{Frames: active, LowFreq: medianOf(lows), Peak: medianOf(peaks)}
	if active < clsMinFrame {
		c.Kind = KindSilent
		return c, nil
	}
	c.StimFrac = float64(stim) / float64(active)
	switch {
	case c.StimFrac >= clsStimFrac:
		c.Kind = KindStim
	case c.StimFrac <= clsAudioFr:
		c.Kind = KindAudio
	default:
		c.Kind = KindUncertain
	}
	return c, nil
}

// spectrumShape returns the share of energy below clsLowHz and in the two
// strongest narrow peaks, and whether the frame is loud enough to judge.
func spectrumShape(x, win []float64, sr float64) (low, peak float64, active bool) {
	var e float64
	for _, v := range x {
		e += v * v
	}
	if math.Sqrt(e/float64(len(x))) < clsMinRMS {
		return 0, 0, false
	}
	z := make([]complex128, len(x))
	for i, v := range x {
		z[i] = complex(v*win[i], 0)
	}
	fft(z)
	binHz := sr / float64(len(x))
	lo := int(20 / binHz)
	p := make([]float64, len(x)/2)
	var total, lowE float64
	for k := lo; k < len(p); k++ {
		m := real(z[k])*real(z[k]) + imag(z[k])*imag(z[k])
		p[k] = m
		total += m
		if float64(k)*binHz < clsLowHz {
			lowE += m
		}
	}
	if total == 0 {
		return 0, 0, false
	}
	w := int(math.Ceil(clsPeakHz / binHz))
	var peakE float64
	for n := 0; n < 2; n++ {
		best := lo
		for k := lo; k < len(p); k++ {
			if p[k] > p[best] {
				best = k
			}
		}
		for k := max(lo, best-w); k <= min(len(p)-1, best+w); k++ {
			peakE += p[k]
			p[k] = 0
		}
	}
	return lowE / total, peakE / total, true
}

func medianOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	for i := 1; i < len(s); i++ { // insertion sort; a few hundred values
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s[len(s)/2]
}

// fft is an in-place iterative radix-2 FFT; len(a) must be a power of two.
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[start+k], a[start+k+size/2]*w
				a[start+k], a[start+k+size/2] = u+v, u-v
				w *= step
			}
		}
	}
}
