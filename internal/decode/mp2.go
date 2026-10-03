package decode

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
)

// Pure-Go MPEG-1/2 Audio Layer II decoder (ISO/IEC 11172-3, 13818-3 LSF).
// Stim tracks are sometimes Layer II streams with an .mp3 extension, which
// go-mp3 (Layer III only) cannot decode.

var (
	l2BitratesV1  = [15]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
	l2BitratesLSF = [15]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	l3BitratesV1  = [15]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	l1BitratesV1  = [15]int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}
	l1BitratesLSF = [15]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}
	sampleRatesV1 = [3]int{44100, 48000, 32000}
)

// quantization classes: number of steps and bits per sample (negative = grouped code of |bits| for 3 samples)
var (
	quantSteps = [17]int{3, 5, 7, 9, 15, 31, 63, 127, 255, 511, 1023, 2047, 4095, 8191, 16383, 32767, 65535}
	quantBits  = [17]int{-5, -7, 3, -10, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
)

type sbAlloc struct {
	nbal    int
	classes []int // classes[alloc-1] = index into quantSteps
}

var l2Tables [5][]sbAlloc // index from selectTable; len = sblimit

func init() {
	var (
		ab0  = []int{0, 2, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		ab1  = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 16}
		ab2  = []int{0, 1, 2, 3, 4, 5, 16}
		ab3  = []int{0, 1, 16}
		cd0  = []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
		cd1  = []int{0, 1, 3, 4, 5, 6, 7}
		lsf0 = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
		lsf1 = []int{0, 1, 3, 4, 5, 6, 7}
		lsf2 = []int{0, 1, 3}
	)
	ab := func(sblimit int) []sbAlloc {
		t := make([]sbAlloc, sblimit)
		for sb := range t {
			switch {
			case sb < 3:
				t[sb] = sbAlloc{4, ab0}
			case sb < 11:
				t[sb] = sbAlloc{4, ab1}
			case sb < 23:
				t[sb] = sbAlloc{3, ab2}
			default:
				t[sb] = sbAlloc{2, ab3}
			}
		}
		return t
	}
	cd := func(sblimit int) []sbAlloc {
		t := make([]sbAlloc, sblimit)
		for sb := range t {
			if sb < 2 {
				t[sb] = sbAlloc{4, cd0}
			} else {
				t[sb] = sbAlloc{3, cd1}
			}
		}
		return t
	}
	lsf := make([]sbAlloc, 30)
	for sb := range lsf {
		switch {
		case sb < 4:
			lsf[sb] = sbAlloc{4, lsf0}
		case sb < 11:
			lsf[sb] = sbAlloc{3, lsf1}
		default:
			lsf[sb] = sbAlloc{2, lsf2}
		}
	}
	l2Tables = [5][]sbAlloc{ab(27), ab(30), cd(8), cd(12), lsf}
}

// selectTable mirrors ISO 11172-3 Annex B table selection (as in ffmpeg's ff_mpa_l2_select_table).
func selectTable(bitrateKbps, nch, sampleRate int, lsf bool) int {
	if lsf {
		return 4
	}
	chBr := bitrateKbps / nch
	switch {
	case (sampleRate == 48000 && chBr >= 56) || (chBr >= 56 && chBr <= 80):
		return 0
	case sampleRate != 48000 && chBr >= 96:
		return 1
	case sampleRate != 32000 && chBr <= 48:
		return 2
	default:
		return 3
	}
}

type mpaHeader struct {
	lsf        bool
	layer      int // 1, 2, 3
	crc        bool
	bitrate    int // kbps
	sampleRate int
	padding    int
	mode       int // 0 stereo, 1 joint, 2 dual, 3 mono
	modeExt    int
	frameLen   int
}

func (h mpaHeader) channels() int {
	if h.mode == 3 {
		return 1
	}
	return 2
}

func parseHeader(b []byte) (mpaHeader, bool) {
	var h mpaHeader
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return h, false
	}
	version := (b[1] >> 3) & 3 // 3 = MPEG1, 2 = MPEG2, 0 = MPEG2.5
	layerBits := (b[1] >> 1) & 3
	if version == 1 || layerBits == 0 {
		return h, false
	}
	h.layer = 4 - int(layerBits)
	h.lsf = version != 3
	h.crc = b[1]&1 == 0
	brIdx := int(b[2] >> 4)
	srIdx := int((b[2] >> 2) & 3)
	if brIdx == 0 || brIdx == 15 || srIdx == 3 {
		return h, false // free format unsupported
	}
	h.padding = int((b[2] >> 1) & 1)
	h.mode = int(b[3] >> 6)
	h.modeExt = int((b[3] >> 4) & 3)
	h.sampleRate = sampleRatesV1[srIdx]
	if h.lsf {
		h.sampleRate /= 2
		if version == 0 {
			h.sampleRate /= 2
		}
	}
	switch {
	case h.layer == 1 && !h.lsf:
		h.bitrate = l1BitratesV1[brIdx]
	case h.layer == 1:
		h.bitrate = l1BitratesLSF[brIdx]
	case h.lsf:
		h.bitrate = l2BitratesLSF[brIdx]
	case h.layer == 2:
		h.bitrate = l2BitratesV1[brIdx]
	default:
		h.bitrate = l3BitratesV1[brIdx]
	}
	switch {
	case h.layer == 1:
		h.frameLen = (12*h.bitrate*1000/h.sampleRate + h.padding) * 4
	case h.layer == 3 && h.lsf:
		h.frameLen = 72*h.bitrate*1000/h.sampleRate + h.padding
	default:
		h.frameLen = 144*h.bitrate*1000/h.sampleRate + h.padding
	}
	return h, true
}

type bitReader struct {
	b   []byte
	pos int
}

func (r *bitReader) read(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		byteIdx := r.pos >> 3
		bit := 0
		if byteIdx < len(r.b) {
			bit = int(r.b[byteIdx]>>(7-uint(r.pos&7))) & 1
		}
		v = v<<1 | bit
		r.pos++
	}
	return v
}

var scaleFactors [64]float64

func init() {
	for i := 0; i < 63; i++ {
		scaleFactors[i] = 2.0 * math.Pow(2, -float64(i)/3)
	}
}

// synthesis filterbank state for one channel
type polySynth struct {
	v   [1024]float64
	off int
}

var synthN [64][32]float64

func init() {
	for i := 0; i < 64; i++ {
		for k := 0; k < 32; k++ {
			synthN[i][k] = math.Cos(float64((16+i)*(2*k+1)) * math.Pi / 64)
		}
	}
}

// synth consumes 32 subband samples and writes 32 PCM samples to out.
func (p *polySynth) synth(s *[32]float64, out []float64) {
	p.off = (p.off - 64) & 1023
	v := p.v[:]
	for i := 0; i < 64; i++ {
		sum := 0.0
		n := &synthN[i]
		for k := 0; k < 32; k++ {
			sum += n[k] * s[k]
		}
		v[(p.off+i)&1023] = sum
	}
	for j := 0; j < 32; j++ {
		sum := 0.0
		for i := 0; i < 8; i++ {
			// U[i*64+j] = V[i*128+j], U[i*64+32+j] = V[i*128+96+j]
			sum += v[(p.off+i*128+j)&1023] * synthWindow[i*64+j]
			sum += v[(p.off+i*128+96+j)&1023] * synthWindow[i*64+32+j]
		}
		out[j] = sum
	}
}

// mp2Source decodes a Layer II stream into interleaved stereo float32.
type mp2Source struct {
	r          *bufio.Reader
	closer     io.Closer
	sampleRate int
	synth      [2]polySynth
	pending    []float32 // decoded, not yet returned interleaved frames
	frameBuf   []byte
	first      *mpaHeader
	eof        bool
}

func newMP2Source(r io.Reader, c io.Closer) (*mp2Source, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	if err := skipID3v2(br); err != nil {
		return nil, err
	}
	s := &mp2Source{r: br, closer: c}
	h, err := s.syncNext()
	if err != nil {
		return nil, fmt.Errorf("mp2: no valid frame: %w", err)
	}
	if h.layer != 2 {
		return nil, fmt.Errorf("mp2: layer %d stream is not supported natively", h.layer)
	}
	s.first = &h
	s.sampleRate = h.sampleRate
	return s, nil
}

func skipID3v2(br *bufio.Reader) error {
	b, err := br.Peek(10)
	if err != nil || string(b[:3]) != "ID3" {
		return nil
	}
	size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
	if b[5]&0x10 != 0 {
		size += 10 // footer
	}
	_, err = br.Discard(10 + size)
	return err
}

// syncNext positions the reader at the next plausible frame header and returns it.
func (s *mp2Source) syncNext() (mpaHeader, error) {
	for {
		b, err := s.r.Peek(4)
		if err != nil {
			return mpaHeader{}, io.EOF
		}
		if h, ok := parseHeader(b); ok && h.layer == 2 && (s.first == nil || h.sampleRate == s.first.sampleRate) {
			return h, nil
		}
		if s.first == nil {
			if h, ok := parseHeader(b); ok && h.layer != 2 {
				return h, nil
			}
		}
		if _, err := s.r.Discard(1); err != nil {
			return mpaHeader{}, io.EOF
		}
	}
}

func (s *mp2Source) SampleRate() int { return s.sampleRate }

func (s *mp2Source) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func (s *mp2Source) Read(dst []float32) (int, error) {
	frames := 0
	for frames*2 < len(dst)-1 {
		if len(s.pending) == 0 {
			if s.eof {
				break
			}
			if err := s.decodeNext(); err != nil {
				if errors.Is(err, io.EOF) {
					s.eof = true
					break
				}
				return frames, err
			}
			continue
		}
		n := copy(dst[frames*2:], s.pending)
		n &^= 1
		s.pending = s.pending[n:]
		frames += n / 2
	}
	if frames == 0 && s.eof {
		return 0, io.EOF
	}
	return frames, nil
}

func (s *mp2Source) decodeNext() error {
	h, err := s.syncNext()
	if err != nil {
		return err
	}
	if cap(s.frameBuf) < h.frameLen {
		s.frameBuf = make([]byte, h.frameLen)
	}
	buf := s.frameBuf[:h.frameLen]
	n, err := io.ReadFull(s.r, buf)
	if err != nil && n < 8 {
		return io.EOF
	}
	pcm := decodeL2Frame(h, buf[:n], &s.synth)
	s.pending = pcm
	return nil
}

// decodeL2Frame decodes one frame to interleaved stereo float32 (mono is duplicated).
func decodeL2Frame(h mpaHeader, frame []byte, synth *[2]polySynth) []float32 {
	nch := h.channels()
	table := l2Tables[selectTable(h.bitrate, nch, h.sampleRate, h.lsf)]
	sblimit := len(table)
	bound := sblimit
	if h.mode == 1 {
		bound = (h.modeExt + 1) * 4
		if bound > sblimit {
			bound = sblimit
		}
	}
	r := &bitReader{b: frame, pos: 32}
	if h.crc {
		r.pos += 16
	}
	var alloc [2][32]int
	for sb := 0; sb < sblimit; sb++ {
		if sb < bound {
			for ch := 0; ch < nch; ch++ {
				alloc[ch][sb] = r.read(table[sb].nbal)
			}
		} else {
			v := r.read(table[sb].nbal)
			alloc[0][sb], alloc[1][sb] = v, v
		}
	}
	var scfsi [2][32]int
	for sb := 0; sb < sblimit; sb++ {
		for ch := 0; ch < nch; ch++ {
			if alloc[ch][sb] != 0 {
				scfsi[ch][sb] = r.read(2)
			}
		}
	}
	var sf [2][32][3]int
	for sb := 0; sb < sblimit; sb++ {
		for ch := 0; ch < nch; ch++ {
			if alloc[ch][sb] == 0 {
				continue
			}
			switch scfsi[ch][sb] {
			case 0:
				sf[ch][sb] = [3]int{r.read(6), r.read(6), r.read(6)}
			case 1:
				a, b := r.read(6), r.read(6)
				sf[ch][sb] = [3]int{a, a, b}
			case 2:
				a := r.read(6)
				sf[ch][sb] = [3]int{a, a, a}
			case 3:
				a, b := r.read(6), r.read(6)
				sf[ch][sb] = [3]int{a, b, b}
			}
		}
	}
	var samples [2][32][36]float64
	readTriple := func(q int) (v [3]float64) {
		steps := quantSteps[q]
		bits := quantBits[q]
		var c [3]int
		if bits < 0 {
			code := r.read(-bits)
			for i := 0; i < 3; i++ {
				c[i] = code % steps
				code /= steps
			}
		} else {
			for i := 0; i < 3; i++ {
				c[i] = r.read(bits)
			}
		}
		for i := 0; i < 3; i++ {
			v[i] = float64(2*c[i]-(steps-1)) / float64(steps)
		}
		return
	}
	for gr := 0; gr < 12; gr++ {
		part := gr / 4
		for sb := 0; sb < sblimit; sb++ {
			if sb < bound {
				for ch := 0; ch < nch; ch++ {
					a := alloc[ch][sb]
					if a == 0 {
						continue
					}
					v := readTriple(table[sb].classes[a-1])
					scale := scaleFactors[sf[ch][sb][part]]
					for i := 0; i < 3; i++ {
						samples[ch][sb][gr*3+i] = v[i] * scale
					}
				}
			} else {
				a := alloc[0][sb]
				if a == 0 {
					continue
				}
				v := readTriple(table[sb].classes[a-1])
				for ch := 0; ch < nch; ch++ {
					scale := scaleFactors[sf[ch][sb][part]]
					for i := 0; i < 3; i++ {
						samples[ch][sb][gr*3+i] = v[i] * scale
					}
				}
			}
		}
	}
	out := make([]float32, 1152*2)
	var sv [32]float64
	var pcm [32]float64
	for ch := 0; ch < nch; ch++ {
		for s := 0; s < 36; s++ {
			for sb := 0; sb < 32; sb++ {
				sv[sb] = samples[ch][sb][s]
			}
			synth[ch].synth(&sv, pcm[:])
			for j := 0; j < 32; j++ {
				out[(s*32+j)*2+ch] = float32(pcm[j])
			}
		}
	}
	if nch == 1 {
		for i := 0; i < 1152; i++ {
			out[i*2+1] = out[i*2]
		}
	}
	return out
}
