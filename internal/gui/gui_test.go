package gui

import (
	"github.com/trilithus/stimconv/internal/decode"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogpu/ui/offscreen"
)

// TestBuild renders the window headlessly in beginner and expert mode, for
// both output types. Set STIMCONV_SHOT=<dir> to keep the PNGs.
func TestBuild(t *testing.T) {
	dir := os.Getenv("STIMCONV_SHOT")
	for _, c := range []struct {
		name   string
		expert bool
		topo   string
	}{{"beginner", false, "dual"}, {"expert", true, "dual"}, {"expert-tri", true, "joined"}} {
		u := newUI(nil, nil)
		u.icons = os.DirFS("../../assets")
		u.expert, u.cfg.Topology = c.expert, c.topo
		u.info.Set("⚠ cannot decode: native decode (mp3): unsupported layer; ffmpeg fallback: ffmpeg not found (use --ffmpeg, STIMCONV_FFMPEG, libs/ffmpeg/bin or PATH): C:/Users/someone/Downloads/stimconv/libs/ffmpeg/bin/ffmpeg.exe")
		u.logf("restim: use device FOC-Stim 4-phase; volume is strength-duration matched, so keep restim's tau at 355 us and its maximum carrier at 2000 Hz; funscript kit ranges: restim defaults")
		r := offscreen.NewRenderer(winW, winH)
		r.Render(u.build())
		if dir == "" {
			continue
		}
		f, err := os.Create(filepath.Join(dir, c.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, r.Image())
		f.Close()
	}
}

func TestWrap(t *testing.T) {
	for _, c := range []struct {
		in   string
		cols int
		want []string
	}{
		{"restim: use device FOC-Stim 4-phase; keep tau", 20, []string{"restim: use device", "FOC-Stim 4-phase;", "keep tau"}},
		// break at the space before a long token, not inside it
		{"fallback: C:/libs/ffmpeg/bin/ffmpeg.exe: not found", 30, []string{"fallback:", "C:/libs/ffmpeg/bin/ffmpeg.exe:", "not found"}},
		// a word wider than the line is cut only because it would be clipped
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"short", 20, []string{"short"}},
	} {
		got := wrap(c.in, c.cols)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrap(%q, %d) = %q, want %q", c.in, c.cols, got, c.want)
		}
	}
}

func TestBuildAboutShot(t *testing.T) {
	dir := os.Getenv("STIMCONV_SHOT")
	u := newUI(nil, nil)
	u.icons = os.DirFS("../../assets")
	u.about = true
	r := offscreen.NewRenderer(winW, winH)
	r.Render(u.build())
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, "about.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, r.Image())
	f.Close()
}

// TestTrackSelector renders the input section with a multi-track file.
func TestTrackSelector(t *testing.T) {
	u := newUI(nil, nil)
	u.icons = os.DirFS("../../assets")
	u.input.Set("/videos/session.mkv")
	u.info.Set("matroska,webm file · 3 audio track(s) · 812.4 MB")
	u.contentWarn.Set("⚠ This looks like music or speech, not a stim drive signal: only 0% of sampled frames are carrier-like, 80% of the energy is below 150 Hz. The conversion assumes an amplifier drive signal; music or speech will give a meaningless result.")
	u.tracks = []decode.Track{
		{Index: 0, Codec: "aac", Language: "eng", Layout: "stereo", Channels: 2, SampleRate: 48000, Title: "Commentary"},
		{Index: 1, Codec: "aac", Layout: "stereo", Channels: 2, SampleRate: 44100, Title: "Stim A/B", Default: true},
		{Index: 2, Codec: "ac3", Layout: "5.1(side)", Channels: 6, SampleRate: 48000},
	}
	w := u.build()
	r := offscreen.NewRenderer(winW, winH)
	r.Render(w)
	if findText(w, "Audio track") == nil {
		t.Fatal("track selector missing")
	}
	if dir := os.Getenv("STIMCONV_SHOT"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "tracks.png"))
		png.Encode(f, r.Image())
		f.Close()
	}
}

func TestBatchShot(t *testing.T) {
	u := newUI(nil, nil)
	u.icons = os.DirFS("../../assets")
	u.inputs = []string{"/s/a.mp3", "/s/b.mkv", "/s/c.wav"}
	u.input.Set("3 files selected (batch)")
	u.info.Set(capLines([]string{"a.mp3 — mpeg-layer3 · 4.1 MB", "b.mkv — matroska,webm file · 2 audio track(s) · 812.4 MB", "c.wav — wav · 12.0 MB"}, 8))
	u.contentWarn.Set("⚠ c.wav: This looks like music or speech, not a stim drive signal.")
	u.logf("— 1/3 · a.mp3")
	u.logf("a.mp3: mpeg-layer3, 48000 Hz, 812.0 s, analysed in 3.1s (topology dual)")
	u.logf("%s", "warning: 29.7% of the time the original carrier exceeds FOC-Stim's 2000 Hz maximum and was clamped; restim will apply its own limits, so check the frequency range in its funscript kit before playing")
	u.logf("wrote 10 funscripts to /s/a.default")
	u.logf("error: native decode (unknown): no native decoder; ffmpeg fallback: ffmpeg not found")
	r := offscreen.NewRenderer(winW, winH)
	r.Render(u.build())
	if dir := os.Getenv("STIMCONV_SHOT"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "batch.png"))
		png.Encode(f, r.Image())
		f.Close()
	}
}
