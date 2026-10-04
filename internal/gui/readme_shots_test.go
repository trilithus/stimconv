package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/ui/offscreen"
	"github.com/trilithus/stimconv/internal/decode"
)

// TestReadmeShots renders the screenshots used in the top-level README.
// It runs only with STIMCONV_README_SHOTS=<dir>, e.g. docs/images.
func TestReadmeShots(t *testing.T) {
	dir := os.Getenv("STIMCONV_README_SHOTS")
	if dir == "" {
		t.Skip("STIMCONV_README_SHOTS not set")
	}
	shot := func(name string, u *ui, h int) {
		r := offscreen.NewRenderer(winW, h)
		r.Render(u.build())
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, r.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newShotUI := func() *ui {
		u := newUI(nil, nil)
		u.icons = os.DirFS("../../assets")
		return u
	}

	// Main window after a finished conversion.
	u := newShotUI()
	u.input.Set("D:/stim/session_04.mp3")
	u.info.Set("mpeg-layer3 · 22050 Hz · 104.4 s · 2.8 MB")
	u.logOpen = true // Start opens the log
	u.logf("session_04.mp3: mpeg-layer3, 22050 Hz, 104.4 s, analysed in 1.9s (topology dual)")
	u.logf("%s", "warning: frequency: 67.9% of values outside the funscript range 500..1000 (data spans 660.5..2000); widen with --range frequency=500:2000 and set the same range in restim's funscript kit")
	u.logf("wrote 10 funscripts and README.md to D:/stim/session_04.default")
	u.logf("restim: use device FOC-Stim 4-phase; volume is strength-duration matched, so keep restim's tau at 355 us and its maximum carrier at 2000 Hz; funscript kit ranges: restim defaults")
	shot("main", u, winH)

	// Expert mode, tall enough to show the option groups.
	u = newShotUI()
	u.expert = true
	u.input.Set("D:/stim/session_04.mp3")
	u.info.Set("mpeg-layer3 · 22050 Hz · 104.4 s · 2.8 MB")
	u.logOpen = true // Start opens the log
	shot("expert", u, 1500)

	// Video file with several audio tracks.
	u = newShotUI()
	u.input.Set("D:/stim/session_07.mkv")
	u.info.Set("matroska,webm file · 3 audio track(s) · 812.4 MB")
	u.tracks = []decode.Track{
		{Index: 0, Codec: "aac", Language: "eng", Layout: "stereo", Channels: 2, SampleRate: 48000, Title: "Commentary"},
		{Index: 1, Codec: "aac", Layout: "stereo", Channels: 2, SampleRate: 44100, Title: "Stim A/B", Default: true},
		{Index: 2, Codec: "ac3", Layout: "5.1(side)", Channels: 6, SampleRate: 48000},
	}
	shot("tracks", u, winH)

	// Batch run with the content check and log levels.
	u = newShotUI()
	u.inputs = []string{"D:/stim/a.mp3", "D:/stim/b.mkv", "D:/stim/c.wav"}
	u.input.Set("3 files selected (batch)")
	u.info.Set(capLines([]string{"a.mp3 — mpeg-layer3 · 4.1 MB", "b.mkv — matroska,webm file · 2 audio track(s) · 812.4 MB", "c.wav — wav · 12.0 MB"}, 8))
	u.contentWarn.Set("⚠ c.wav: This looks like music or speech, not a stim drive signal.")
	u.logf("— 1/3 · a.mp3")
	u.logf("a.mp3: mpeg-layer3, 48000 Hz, 812.0 s, analysed in 3.1s (topology dual)")
	u.logf("%s", "warning: 0.1% of the time the original carrier exceeds FOC-Stim's 2000 Hz maximum and was clamped")
	u.logf("wrote 10 funscripts and README.md to D:/stim/a.default")
	u.logf("— 2/3 · b.mkv")
	u.logf("b.mkv: matroska,webm file, 44100 Hz, 1630.2 s, analysed in 6.4s (topology dual)")
	u.logf("wrote 10 funscripts and README.md to D:/stim/b.default")
	u.logf("— 3/3 · c.wav")
	u.logf("%s", "warning: this may not be a stim drive signal: 4% of sampled frames are carrier-like")
	shot("batch", u, winH)
}
