package decode

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// probeMKV is `ffmpeg -hide_banner -i` output for a video with three audio
// tracks (stereo default with title, 5.1 commentary, mono).
const probeMKV = `Input #0, matroska,webm, from 'video.mkv':
  Metadata:
    ENCODER         : Lavf61.7.100
  Duration: 00:10:00.02, start: 0.000000, bitrate: 2251 kb/s
  Stream #0:0: Video: h264 (High), yuv420p(progressive), 1920x1080, 30 fps, 30 tbr, 1k tbn (default)
      Metadata:
        DURATION        : 00:10:00.000000000
  Stream #0:1(eng): Audio: aac (LC), 48000 Hz, stereo, fltp (default)
      Metadata:
        title           : Stim A/B
        DURATION        : 00:10:00.020000000
  Stream #0:2(jpn): Audio: ac3, 44100 Hz, 5.1(side), fltp, 448 kb/s
      Metadata:
        title           : Commentary
  Stream #0:3: Audio: pcm_s16le, 22050 Hz, mono, s16, 352 kb/s
At least one output file must be specified
`

func TestParseProbe(t *testing.T) {
	info := parseProbe(probeMKV)
	if info.Container != "matroska,webm" {
		t.Errorf("container = %q", info.Container)
	}
	want := []Track{
		{Index: 0, Codec: "aac", Language: "eng", Title: "Stim A/B", Layout: "stereo", Channels: 2, SampleRate: 48000, Default: true},
		{Index: 1, Codec: "ac3", Language: "jpn", Title: "Commentary", Layout: "5.1(side)", Channels: 6, SampleRate: 44100},
		{Index: 2, Codec: "pcm_s16le", Layout: "mono", Channels: 1, SampleRate: 22050},
	}
	if len(info.Tracks) != len(want) {
		t.Fatalf("tracks = %+v", info.Tracks)
	}
	for i, w := range want {
		if info.Tracks[i] != w {
			t.Errorf("track %d = %+v, want %+v", i, info.Tracks[i], w)
		}
	}
	if s := info.Tracks[0].String(); s != `aac · eng · stereo 48 kHz · "Stim A/B" (default)` {
		t.Errorf("label = %s", s)
	}
}

func TestParseProbeVideoOnly(t *testing.T) {
	info := parseProbe("Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'x.mp4':\n  Stream #0:0[0x1](und): Video: h264, yuv420p, 640x480 (default)\n")
	if info.Container != "mov,mp4,m4a,3gp,3g2,mj2" || len(info.Tracks) != 0 {
		t.Errorf("info = %+v", info)
	}
}

// fakeFFmpeg answers probes with probeMKV and records decode arguments.
func fakeFFmpeg(t *testing.T) (argsFile string) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script ffmpeg")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	probe := filepath.Join(dir, "probe.txt")
	os.WriteFile(probe, []byte(probeMKV), 0o644)
	script := "#!/bin/sh\nif [ \"$1\" = \"-hide_banner\" ]; then cat '" + probe + "' >&2; exit 1; fi\necho \"$@\" > '" + argsFile + "'\n"
	bin := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := FFmpegPath
	FFmpegPath = bin
	t.Cleanup(func() { FFmpegPath = old })
	return argsFile
}

func decodeArgs(t *testing.T, track int) (string, error) {
	argsFile := fakeFFmpeg(t)
	src, err := openFFmpeg("video.mkv", track)
	if err != nil {
		return "", err
	}
	src.Read(make([]float32, 64))
	src.Close()
	b, _ := os.ReadFile(argsFile)
	return string(b), nil
}

func TestTrackSelectionArgs(t *testing.T) {
	for _, c := range []struct {
		track int
		want  []string
		not   string
	}{
		{0, []string{"-map 0:a:0", "-ac 2"}, "pan="},                   // default-flagged stereo track
		{2, []string{"-map 0:a:1", "pan=stereo|c0=c0|c1=c1"}, "-ac 2"}, // 5.1: keep FL/FR, no downmix
		{3, []string{"-map 0:a:2", "-ac 2"}, "pan="},                   // mono is duplicated
	} {
		args, err := decodeArgs(t, c.track)
		if err != nil {
			t.Fatalf("track %d: %v", c.track, err)
		}
		for _, w := range c.want {
			if !strings.Contains(args, w) {
				t.Errorf("track %d: args %q lack %q", c.track, args, w)
			}
		}
		if strings.Contains(args, c.not) {
			t.Errorf("track %d: args %q contain %q", c.track, args, c.not)
		}
	}
	if _, err := decodeArgs(t, 4); err == nil || !strings.Contains(err.Error(), "3 audio track") {
		t.Errorf("out-of-range track: err = %v", err)
	}
}
