package decode

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Track is one audio stream of a file. Index counts audio streams only
// (0-based, as in -map 0:a:<Index>); users see and pass Index+1.
type Track struct {
	Index      int
	Codec      string
	Language   string
	Title      string
	Layout     string // ffmpeg channel layout: mono, stereo, 5.1(side), …
	Channels   int
	SampleRate int
	Default    bool
}

// Info describes a file's container and audio tracks.
type Info struct {
	Format    Format
	Container string // ffmpeg demuxer name for files decoded by ffmpeg
	Tracks    []Track
}

// String is a one-line label such as `aac · eng · stereo 48 kHz · "Stim" (default)`.
func (t Track) String() string {
	parts := []string{t.Codec}
	if t.Language != "" && t.Language != "und" {
		parts = append(parts, t.Language)
	}
	ch := t.Layout
	if ch == "" {
		ch = fmt.Sprintf("%d ch", t.Channels)
	}
	if t.SampleRate > 0 {
		ch += fmt.Sprintf(" %g kHz", float64(t.SampleRate)/1000)
	}
	parts = append(parts, ch)
	if t.Title != "" {
		parts = append(parts, strconv.Quote(t.Title))
	}
	s := strings.Join(parts, " · ")
	if t.Default {
		s += " (default)"
	}
	return s
}

// Probe lists the audio tracks. Natively decoded formats have exactly one
// track and are not passed to ffmpeg; anything else (video containers, …)
// is inspected with ffmpeg.
func Probe(path string) (Info, error) {
	format, err := Sniff(path)
	if err != nil {
		return Info{Format: format}, err
	}
	switch format {
	case FormatMP2, FormatMP3, FormatFLAC, FormatWAV:
		return Info{Format: format, Tracks: []Track{{Codec: string(format), Default: true}}}, nil
	}
	info, err := ffprobe(path)
	info.Format = format
	return info, err
}

func ffprobe(path string) (Info, error) {
	bin, err := findFFmpeg()
	if err != nil {
		return Info{}, err
	}
	cmd := exec.Command(bin, "-hide_banner", "-i", path)
	tail := &tailBuffer{}
	cmd.Stderr = tail
	hideWindow(cmd)
	_ = cmd.Run() // exits 1 without an output file; the listing is on stderr
	info := parseProbe(string(tail.b))
	if info.Container == "" {
		return info, fmt.Errorf("ffmpeg cannot read the file: %s", lastLine(tail.String()))
	}
	if len(info.Tracks) == 0 {
		return info, fmt.Errorf("no audio tracks in this file")
	}
	return info, nil
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

var (
	reInput  = regexp.MustCompile(`^Input #0, ([^ ]+?),? from `)
	reStream = regexp.MustCompile(`^\s*Stream #0:\d+(?:\[[^\]]*\])?(?:\(([^)]*)\))?: (\w+): (.*)$`)
	reRate   = regexp.MustCompile(`(\d+) Hz`)
	reTitle  = regexp.MustCompile(`^\s+title\s*: (.*)$`)
	reCh     = regexp.MustCompile(`^(\d+) channels`)
)

// parseProbe reads the stream listing that `ffmpeg -i` prints to stderr.
func parseProbe(out string) Info {
	var info Info
	var cur *Track // track whose metadata block follows
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := reInput.FindStringSubmatch(line); m != nil {
			info.Container = strings.TrimSuffix(m[1], ",")
			continue
		}
		if m := reStream.FindStringSubmatch(line); m != nil {
			cur = nil
			if m[2] != "Audio" {
				continue
			}
			t := Track{Index: len(info.Tracks), Language: m[1]}
			desc := m[3]
			t.Default = strings.Contains(desc, "(default)")
			fields := splitTop(desc)
			if len(fields) > 0 {
				t.Codec = strings.Fields(fields[0])[0]
			}
			for _, f := range fields[1:] {
				f = strings.TrimSpace(f)
				if r := reRate.FindStringSubmatch(f); r != nil && t.SampleRate == 0 {
					t.SampleRate, _ = strconv.Atoi(r[1])
				} else if t.Layout == "" && t.SampleRate > 0 {
					t.Layout = f
					t.Channels = layoutChannels(f)
				}
			}
			info.Tracks = append(info.Tracks, t)
			cur = &info.Tracks[len(info.Tracks)-1]
			continue
		}
		if cur != nil && cur.Title == "" {
			if m := reTitle.FindStringSubmatch(line); m != nil {
				cur.Title = strings.TrimSpace(m[1])
			}
		}
	}
	return info
}

// splitTop splits on commas outside parentheses.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// layoutChannels converts an ffmpeg channel layout name to a channel count.
func layoutChannels(l string) int {
	if m := reCh.FindStringSubmatch(l); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	base, _, _ := strings.Cut(l, "(")
	switch base {
	case "mono":
		return 1
	case "stereo", "downmix":
		return 2
	case "2.1", "3.0":
		return 3
	case "quad", "4.0", "3.1":
		return 4
	case "5.0", "4.1":
		return 5
	case "5.1", "6.0", "hexagonal":
		return 6
	case "6.1", "7.0":
		return 7
	case "7.1", "octagonal":
		return 8
	}
	return 0
}
