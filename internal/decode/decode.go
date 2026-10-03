// Package decode turns audio files into a stream of interleaved stereo
// float32 frames. Formats are sniffed from the file header, not the extension.
// Native decoders cover MPEG Layer II/III, FLAC and WAV; everything else is
// piped through ffmpeg when it is available.
package decode

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"
)

// Source yields interleaved stereo frames (mono inputs are duplicated).
type Source interface {
	SampleRate() int
	// Read fills dst (even length) with interleaved L/R samples and returns
	// the number of frames written. It returns io.EOF when exhausted.
	Read(dst []float32) (int, error)
	Close() error
}

// Format is the sniffed container/codec.
type Format string

const (
	FormatMP3     Format = "mpeg-layer3"
	FormatMP2     Format = "mpeg-layer2"
	FormatMP1     Format = "mpeg-layer1"
	FormatFLAC    Format = "flac"
	FormatWAV     Format = "wav"
	FormatUnknown Format = "unknown"
)

// Sniff inspects the start of the file.
func Sniff(path string) (Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return FormatUnknown, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<16)
	head, _ := br.Peek(12)
	switch {
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return FormatWAV, nil
	case len(head) >= 4 && string(head[:4]) == "fLaC":
		return FormatFLAC, nil
	}
	if err := skipID3v2(br); err != nil {
		return FormatUnknown, nil
	}
	// look for two consecutive consistent MPEG headers within the first 64 KiB
	buf, _ := br.Peek(1 << 16)
	for i := 0; i+4 <= len(buf); i++ {
		h, ok := parseHeader(buf[i:])
		if !ok {
			continue
		}
		j := i + h.frameLen
		if j+4 <= len(buf) {
			h2, ok2 := parseHeader(buf[j:])
			if !ok2 || h2.layer != h.layer || h2.sampleRate != h.sampleRate {
				continue
			}
		}
		switch h.layer {
		case 1:
			return FormatMP1, nil
		case 2:
			return FormatMP2, nil
		default:
			return FormatMP3, nil
		}
	}
	return FormatUnknown, nil
}

// Options tune decoder selection.
type Options struct {
	// NativeMP3 decodes MPEG Layer III with go-mp3 instead of ffmpeg.
	// go-mp3 is bit-accurate on typical (long-block) encodes, but produced a
	// corrupted waveform on a short-block-heavy MPEG-2 file during validation,
	// so it is opt-in. Layer II, FLAC and WAV are always decoded natively.
	NativeMP3 bool
	// AudioTrack selects an audio stream, numbered from 1 among the audio
	// streams (Track.Index+1). 0 takes the track flagged default, else the
	// first. Native formats have a single track.
	AudioTrack int
}

// Open sniffs the file and returns a decoding Source. Unknown formats (and
// native decoder failures) fall back to ffmpeg.
func Open(path string, opt Options) (Source, Format, error) {
	format, err := Sniff(path)
	if err != nil {
		return nil, format, err
	}
	native := format == FormatMP2 || format == FormatMP3 || format == FormatFLAC || format == FormatWAV
	if native && opt.AudioTrack > 1 {
		return nil, format, fmt.Errorf("audio track %d requested, but %s files have a single track", opt.AudioTrack, format)
	}
	var src Source
	switch format {
	case FormatMP2:
		src, err = openMP2(path)
	case FormatMP3:
		if opt.NativeMP3 {
			src, err = openMP3(path)
		} else {
			err = errors.New("Layer III is decoded with ffmpeg by default (--native-mp3 enables the experimental go-mp3 decoder)")
		}
	case FormatFLAC:
		src, err = openFLAC(path)
	case FormatWAV:
		src, err = openWAV(path)
	default:
		err = errors.New("no native decoder")
	}
	if err == nil {
		return src, format, nil
	}
	fsrc, ferr := openFFmpeg(path, opt.AudioTrack)
	if ferr != nil {
		return nil, format, fmt.Errorf("native decode (%s): %v; ffmpeg fallback: %w", format, err, ferr)
	}
	return fsrc, format, nil
}

func openMP2(path string) (Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := newMP2Source(f, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// --- Layer III via go-mp3 -------------------------------------------------

type mp3Source struct {
	f   *os.File
	d   *mp3.Decoder
	buf []byte
}

func openMP3(path string) (Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d, err := mp3.NewDecoder(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &mp3Source{f: f, d: d}, nil
}

func (s *mp3Source) SampleRate() int { return s.d.SampleRate() }
func (s *mp3Source) Close() error    { return s.f.Close() }

func (s *mp3Source) Read(dst []float32) (int, error) {
	frames := len(dst) / 2
	need := frames * 4
	if cap(s.buf) < need {
		s.buf = make([]byte, need)
	}
	n, err := io.ReadFull(s.d, s.buf[:need])
	got := n / 4
	for i := 0; i < got; i++ {
		dst[2*i] = float32(int16(binary.LittleEndian.Uint16(s.buf[4*i:]))) / 32768
		dst[2*i+1] = float32(int16(binary.LittleEndian.Uint16(s.buf[4*i+2:]))) / 32768
	}
	if got == 0 && err != nil {
		return 0, io.EOF
	}
	return got, nil
}

// --- FLAC -------------------------------------------------------------------

type flacSource struct {
	s       *flac.Stream
	pending []float32
	scale   float32
}

func openFLAC(path string) (Source, error) {
	s, err := flac.Open(path)
	if err != nil {
		return nil, err
	}
	return &flacSource{s: s, scale: 1 / float32(int64(1)<<(s.Info.BitsPerSample-1))}, nil
}

func (s *flacSource) SampleRate() int { return int(s.s.Info.SampleRate) }
func (s *flacSource) Close() error    { return s.s.Close() }

func (s *flacSource) Read(dst []float32) (int, error) {
	frames := 0
	for frames*2 < len(dst)-1 {
		if len(s.pending) == 0 {
			fr, err := s.s.ParseNext()
			if err != nil {
				if frames == 0 {
					return 0, io.EOF
				}
				break
			}
			n := int(fr.BlockSize)
			l := fr.Subframes[0].Samples
			r := l
			if len(fr.Subframes) > 1 {
				r = fr.Subframes[1].Samples
			}
			s.pending = make([]float32, 2*n)
			for i := 0; i < n; i++ {
				s.pending[2*i] = float32(l[i]) * s.scale
				s.pending[2*i+1] = float32(r[i]) * s.scale
			}
		}
		n := copy(dst[frames*2:], s.pending) &^ 1
		s.pending = s.pending[n:]
		frames += n / 2
	}
	return frames, nil
}

// --- WAV (integer PCM) ------------------------------------------------------

type wavSource struct {
	f     *os.File
	d     *wav.Decoder
	ib    *audio.IntBuffer
	nch   int
	scale float32
}

func openWAV(path string) (Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d := wav.NewDecoder(f)
	if !d.IsValidFile() || d.WavAudioFormat != 1 {
		f.Close()
		return nil, errors.New("unsupported wav (only integer PCM natively)")
	}
	nch := int(d.NumChans)
	return &wavSource{f: f, d: d, nch: nch,
		scale: 1 / float32(int64(1)<<(d.BitDepth-1)),
		ib:    &audio.IntBuffer{Format: &audio.Format{NumChannels: nch, SampleRate: int(d.SampleRate)}}}, nil
}

func (s *wavSource) SampleRate() int { return int(s.d.SampleRate) }
func (s *wavSource) Close() error    { return s.f.Close() }

func (s *wavSource) Read(dst []float32) (int, error) {
	frames := len(dst) / 2
	if cap(s.ib.Data) < frames*s.nch {
		s.ib.Data = make([]int, frames*s.nch)
	}
	s.ib.Data = s.ib.Data[:frames*s.nch]
	n, err := s.d.PCMBuffer(s.ib)
	got := n / s.nch
	for i := 0; i < got; i++ {
		l := float32(s.ib.Data[i*s.nch]) * s.scale
		r := l
		if s.nch > 1 {
			r = float32(s.ib.Data[i*s.nch+1]) * s.scale
		}
		dst[2*i], dst[2*i+1] = l, r
	}
	if got == 0 {
		if err == nil {
			err = io.EOF
		}
		return 0, io.EOF
	}
	return got, nil
}

// --- ffmpeg fallback -------------------------------------------------------

const ffmpegRate = 48000

type ffmpegSource struct {
	cmd    *exec.Cmd
	out    io.ReadCloser
	buf    []byte
	stderr *tailBuffer
}

// tailBuffer keeps the last bytes ffmpeg writes to stderr, for error
// messages. ffmpeg must not inherit our stderr: after the GUI detaches from
// its console (Windows) that handle is invalid and CreateProcess fails with
// "The request is not supported".
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	const keep = 4096
	t.b = append(t.b, p...)
	if len(t.b) > keep {
		t.b = t.b[len(t.b)-keep:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return strings.TrimSpace(string(t.b)) }

// FFmpegPath overrides ffmpeg discovery when non-empty.
var FFmpegPath string

// findFFmpeg looks for ffmpeg in: FFmpegPath, $STIMCONV_FFMPEG,
// libs/ffmpeg/bin next to the executable, its parent directory or the working
// directory, then PATH.
func findFFmpeg() (string, error) {
	for _, p := range []string{FFmpegPath, os.Getenv("STIMCONV_FFMPEG")} {
		if p != "" {
			if _, err := os.Stat(p); err != nil {
				return "", fmt.Errorf("ffmpeg %q: %w", p, err)
			}
			return p, nil
		}
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe), filepath.Dir(filepath.Dir(exe))) // e.g. bin/../libs
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		p := filepath.Join(d, "libs", "ffmpeg", "bin", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", errors.New("ffmpeg not found (use --ffmpeg, STIMCONV_FFMPEG, libs/ffmpeg/bin or PATH)")
}

// openFFmpeg decodes audio track (1-based; 0 = default track) to stereo.
// Tracks with more than two channels keep their first two channels as L/R
// (pan filter) instead of being downmixed, which would blend the A and B
// stim channels.
func openFFmpeg(path string, track int) (Source, error) {
	bin, err := findFFmpeg()
	if err != nil {
		return nil, err
	}
	args := []string{"-v", "error", "-i", path}
	channels := 0
	if info, err := ffprobe(path); err == nil {
		// resolve the track explicitly: ffmpeg's own pick may differ from
		// the default-flagged one, and channels must match the decoded track
		i := DefaultTrack(info.Tracks)
		if track > 0 {
			i = track - 1
		}
		if i >= len(info.Tracks) {
			return nil, fmt.Errorf("audio track %d requested, but the file has %d audio track(s)", track, len(info.Tracks))
		}
		channels = info.Tracks[i].Channels
		args = append(args, "-map", fmt.Sprintf("0:a:%d", i))
	} else if track > 1 {
		return nil, err
	}
	if channels > 2 {
		args = append(args, "-af", "pan=stereo|c0=c0|c1=c1")
	} else {
		args = append(args, "-ac", "2")
	}
	args = append(args, "-f", "f32le", "-ar", fmt.Sprint(ffmpegRate), "-")
	cmd := exec.Command(bin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	tail := &tailBuffer{}
	cmd.Stderr = tail
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &ffmpegSource{cmd: cmd, out: out, stderr: tail}, nil
}

// DefaultTrack is the track ffmpeg plays without -map: the one flagged
// default, else the first.
func DefaultTrack(ts []Track) int {
	for _, t := range ts {
		if t.Default {
			return t.Index
		}
	}
	return 0
}

func (s *ffmpegSource) SampleRate() int { return ffmpegRate }

func (s *ffmpegSource) Close() error {
	s.out.Close()
	if err := s.cmd.Wait(); err != nil {
		if msg := s.stderr.String(); msg != "" {
			return fmt.Errorf("ffmpeg: %w: %s", err, msg)
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}

func (s *ffmpegSource) Read(dst []float32) (int, error) {
	frames := len(dst) / 2
	need := frames * 8
	if cap(s.buf) < need {
		s.buf = make([]byte, need)
	}
	n, _ := io.ReadFull(s.out, s.buf[:need])
	got := n / 8
	for i := 0; i < got*2; i++ {
		dst[i] = float32frombits(binary.LittleEndian.Uint32(s.buf[4*i:]))
	}
	if got == 0 {
		return 0, io.EOF
	}
	return got, nil
}
