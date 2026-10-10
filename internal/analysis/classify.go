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
	Voiced   float64 // share of active frames with a fundamental and its octave (voice, notes)
}

func (c Check) String() string {
	switch c.Kind {
	case KindSilent:
		return "too little signal at the start to check the content"
	case KindStim:
		return fmt.Sprintf("looks like a stim drive signal (%.0f%% of sampled frames)", 100*c.StimFrac)
	case KindAudio:
		if c.Voiced >= 0.2 {
			return fmt.Sprintf("this looks like music or speech, not a stim drive signal: only %.0f%% of sampled frames look like a drive signal, %.0f%% sound like a voice or note (a fundamental with its octave)", 100*c.StimFrac, 100*c.Voiced)
		}
		return fmt.Sprintf("this looks like music or speech, not a stim drive signal: only %.0f%% of sampled frames look like a drive signal (narrow-band, square/sine-like or steady)", 100*c.StimFrac)
	}
	return fmt.Sprintf("this may not be a stim drive signal: only %.0f%% of sampled frames look like a drive signal", 100*c.StimFrac)
}

const (
	clsN        = 4096 // FFT size
	clsHopS     = 0.5  // seconds between analysed frames
	clsMinRMS   = 3e-3 // ~ -50 dBFS: quieter frames are skipped
	clsLowHz    = 150  // bass and the bulk of voice energy sit below this
	clsPeakHz   = 60   // half-width of a "narrow" peak (keeps AM sidebands)
	clsMaxLow   = 0.05
	clsMinPeak  = 0.6
	clsPurePeak = 0.9 // a nearly pure tone counts at any frequency (low carriers)
	clsCrest    = 2.2 // peak/RMS below this: square/sine-like drive waveform
	clsSteadyS  = 0.1 // steadiness compares the spectrum 100 ms later
	clsSteady   = 0.1 // 1 - cosine similarity below this: steady spectrum
	clsStimFrac = 0.6
	clsAudioFr  = 0.4
	clsMinFrame = 6
	// voiced speech: two strongest peaks an octave apart (2:1 within ±3%)
	// with the lower one below 500 Hz. A voice with little bass passes the
	// low-frequency and peak tests (fundamental plus octave hold >90% of the
	// energy); in 58 stim tracks at most 1% of frames looked like this.
	clsVoiceHz   = 500
	clsOctaveTol = 0.06
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
	off := int(clsSteadyS * sr) // second window for the steadiness test
	span := off + clsN
	hop := max(int(clsHopS*sr), span)
	limit := int(maxSeconds * sr)
	win := make([]float64, clsN)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(clsN-1))
	}
	var frame [2][]float64
	frame[0], frame[1] = make([]float64, 0, span), make([]float64, 0, span)
	buf := make([]float32, 8192)
	pos := 0 // frame index within the file
	var stim, active, voicedN int
	var lows, peaks []float64
	for pos < limit {
		n, err := src.Read(buf)
		for i := 0; i < n; i++ {
			k := pos % hop
			pos++
			if k >= span {
				continue
			}
			frame[0] = append(frame[0], float64(buf[2*i]))
			frame[1] = append(frame[1], float64(buf[2*i+1]))
			if len(frame[0]) < span {
				continue
			}
			// judge the louder channel (the other may be silent or quiet)
			c := 0
			if rms(frame[1][:clsN]) > rms(frame[0][:clsN]) {
				c = 1
			}
			if f, ok := frameShape(frame[c][:clsN], frame[c][off:span], win, sr); ok {
				active++
				lows, peaks = append(lows, f.low), append(peaks, f.peak)
				if f.voiced {
					voicedN++
				}
				if f.stim() {
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
	c.Voiced = float64(voicedN) / float64(active)
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

// shape describes one analysed frame.
type shape struct {
	low, peak float64 // energy share below clsLowHz / in the two strongest narrow peaks
	voiced    bool    // fundamental below clsVoiceHz with its octave next
	crest     float64 // peak / RMS of the waveform
	change    float64 // 1 - cosine similarity with the spectrum clsSteadyS later
}

// stim reports whether the frame shows a trait of a drive signal and none
// of speech. Drive signals are narrow-band (a carrier, or a nearly pure low
// tone), or have a low crest factor (square and sine-like waveforms at full
// level), or keep the same bass-free spectrum for long stretches (also
// noise-based stim); music and speech rarely do: their crest factor is 3 or
// more and their spectrum changes every few hundred ms. On 58 stim tracks
// at least 68% of frames passed; on video soundtracks, porn audio and an
// electro music set at most 40%.
func (f shape) stim() bool {
	narrow := (f.peak >= clsMinPeak && f.low <= clsMaxLow) || f.peak >= clsPurePeak
	// held chords are steady too, but carry bass; steady stim has none
	// (low tones count as narrow)
	steady := f.change < clsSteady && f.low <= clsMaxLow
	return !f.voiced && (narrow || f.crest < clsCrest || steady)
}

func rms(x []float64) float64 {
	var e float64
	for _, v := range x {
		e += v * v
	}
	return math.Sqrt(e / float64(len(x)))
}

// frameShape analyses x (and y, the same channel clsSteadyS later); ok is
// false when x is too quiet to judge.
func frameShape(x, y, win []float64, sr float64) (f shape, ok bool) {
	r := rms(x)
	if r < clsMinRMS {
		return f, false
	}
	var pk float64
	for _, v := range x {
		pk = math.Max(pk, math.Abs(v))
	}
	f.crest = pk / r
	px := power(x, win)
	py := power(y, win)
	binHz := sr / float64(len(x))
	lo := int(20 / binHz)
	var total, lowE, dot, nx, ny float64
	for k := lo; k < len(px); k++ {
		total += px[k]
		if float64(k)*binHz < clsLowHz {
			lowE += px[k]
		}
		ax, ay := math.Sqrt(px[k]), math.Sqrt(py[k])
		dot, nx, ny = dot+ax*ay, nx+ax*ax, ny+ay*ay
	}
	if total == 0 {
		return f, false
	}
	f.change = 1
	if nx > 0 && ny > 0 {
		f.change = 1 - dot/math.Sqrt(nx*ny)
	}
	w := int(math.Ceil(clsPeakHz / binHz))
	var peakE float64
	var peakHz [2]float64
	for n := 0; n < 2; n++ {
		best := lo
		for k := lo; k < len(px); k++ {
			if px[k] > px[best] {
				best = k
			}
		}
		peakHz[n] = float64(best) * binHz
		for k := max(lo, best-w); k <= min(len(px)-1, best+w); k++ {
			peakE += px[k]
			px[k] = 0
		}
	}
	// a fundamental below clsVoiceHz with its octave as the next strongest
	// peak: voiced speech or a sung/played note, not a carrier (pure tones
	// and square waves have no even harmonics)
	f0, f1 := min(peakHz[0], peakHz[1]), max(peakHz[0], peakHz[1])
	f.voiced = f0 > 0 && f0 < clsVoiceHz && math.Abs(f1/f0-2) < clsOctaveTol
	f.low, f.peak = lowE/total, peakE/total
	return f, true
}

// power returns the Hann-windowed power spectrum of x (len(x)/2 bins).
func power(x, win []float64) []float64 {
	z := make([]complex128, len(x))
	for i, v := range x {
		z[i] = complex(v*win[i], 0)
	}
	fft(z)
	p := make([]float64, len(x)/2)
	for k := range p {
		p[k] = real(z[k])*real(z[k]) + imag(z[k])*imag(z[k])
	}
	return p
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
