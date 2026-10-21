package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/trilithus/stimconv/internal/decode"
)

// newTestUI builds the window content with Fyne's headless test app.
func newTestUI(t *testing.T) *ui {
	test.NewTempApp(t)
	u := newUI(nil)
	u.icons = os.DirFS("../../assets")
	return u
}

// shot renders u at w×h and, with dir set, writes <dir>/<name>.png.
func shot(t *testing.T, u *ui, dir, name string, w, h float32) fyne.Window {
	win := test.NewTempWindow(t, u.build())
	win.SetPadded(false)
	win.Resize(fyne.NewSize(w, h))
	u.win = win
	img := win.Canvas().Capture()
	if dir == "" {
		return win
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return win
}

// walk visits o and everything below it, widgets included.
func walk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			walk(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			walk(c, fn)
		}
	}
}

// findText returns the visible label showing want.
func findText(o fyne.CanvasObject, want string) *widget.Label {
	var r *widget.Label
	walk(o, func(c fyne.CanvasObject) {
		if l, ok := c.(*widget.Label); ok && l.Text == want && l.Visible() && r == nil {
			r = l
		}
	})
	return r
}

// TestBuild renders the window headlessly in beginner and expert mode, for
// both output types. Set STIMCONV_SHOT=<dir> to keep the PNGs.
func TestBuild(t *testing.T) {
	dir := os.Getenv("STIMCONV_SHOT")
	for _, c := range []struct {
		name   string
		expert bool
		topo   string
	}{{"beginner", false, "dual"}, {"expert", true, "dual"}, {"expert-tri", true, "joined"}} {
		u := newTestUI(t)
		u.expert, u.cfg.Topology = c.expert, c.topo
		u.info.Set("⚠ cannot decode: native decode (mp3): unsupported layer; ffmpeg fallback: ffmpeg not found (use --ffmpeg, STIMCONV_FFMPEG, libs/ffmpeg/bin or PATH): C:/Users/someone/Downloads/stimconv/libs/ffmpeg/bin/ffmpeg.exe")
		u.logf("restim: use device FOC-Stim 4-phase; volume is strength-duration matched, so keep restim's tau at 355 us and its maximum carrier at 2000 Hz; funscript kit ranges: restim defaults")
		shot(t, u, dir, c.name, winW, winH)
	}
}

func TestBuildAboutShot(t *testing.T) {
	u := newTestUI(t)
	u.about = true
	w := shot(t, u, os.Getenv("STIMCONV_SHOT"), "about", winW, winH)
	if findText(w.Content(), "About stimconv") == nil {
		t.Fatal("about page missing")
	}
}

// TestTrackSelector renders the input section with a multi-track file.
func TestTrackSelector(t *testing.T) {
	u := newTestUI(t)
	u.input.Set("/videos/session.mkv")
	u.info.Set("matroska,webm file · 3 audio track(s) · 812.4 MB")
	u.contentWarn.Set("⚠ This looks like music or speech, not a stim drive signal: only 0% of sampled frames are carrier-like, 80% of the energy is below 150 Hz. The conversion assumes an amplifier drive signal; music or speech will give a meaningless result.")
	u.tracks = []decode.Track{
		{Index: 0, Codec: "aac", Language: "eng", Layout: "stereo", Channels: 2, SampleRate: 48000, Title: "Commentary"},
		{Index: 1, Codec: "aac", Layout: "stereo", Channels: 2, SampleRate: 44100, Title: "Stim A/B", Default: true},
		{Index: 2, Codec: "ac3", Layout: "5.1(side)", Channels: 6, SampleRate: 48000},
	}
	w := shot(t, u, os.Getenv("STIMCONV_SHOT"), "tracks", winW, winH)
	if findText(w.Content(), "Audio track") == nil {
		t.Fatal("track selector missing")
	}
}

func TestBatchShot(t *testing.T) {
	u := newTestUI(t)
	u.inputs = []string{"/s/a.mp3", "/s/b.mkv", "/s/c.wav"}
	u.input.Set("3 files selected (batch)")
	u.info.Set(capLines([]string{"a.mp3 — mpeg-layer3 · 4.1 MB", "b.mkv — matroska,webm file · 2 audio track(s) · 812.4 MB", "c.wav — wav · 12.0 MB"}, 8))
	u.contentWarn.Set("⚠ c.wav: This looks like music or speech, not a stim drive signal.")
	u.logf("— 1/3 · a.mp3")
	u.logf("a.mp3: mpeg-layer3, 48000 Hz, 812.0 s, analysed in 3.1s (topology dual)")
	u.logf("%s", "warning: 29.7% of the time the original carrier exceeds FOC-Stim's 2000 Hz maximum and was clamped; restim will apply its own limits, so check the frequency range in its funscript kit before playing")
	u.logf("wrote 10 funscripts to /s/a.default")
	u.logf("error: native decode (unknown): no native decoder; ffmpeg fallback: ffmpeg not found")
	shot(t, u, os.Getenv("STIMCONV_SHOT"), "batch", winW, winH)
}

// TestLiveUpdates checks that state changes reach the widgets without a
// rebuild.
func TestLiveUpdates(t *testing.T) {
	u := newTestUI(t)
	u.logOpen = true // collapsed by default
	w := shot(t, u, "", "", winW, winH)
	u.info.Set("⚠ cannot decode: no frames")
	u.status.Set("Done — 12 files in /some/long/output/directory")
	for _, s := range []string{"⚠ cannot decode: no frames", "Done — 12 files in /some/long/output/directory"} {
		if findText(w.Content(), s) == nil {
			t.Errorf("%q not shown", s)
		}
	}
	u.logf("hello console")
	found := false
	walk(w.Content(), func(c fyne.CanvasObject) {
		if rt, ok := c.(*widget.RichText); ok && rt.String() == consoleSymbols().sym[levelInfo]+" hello console" {
			found = true
		}
	})
	if !found {
		t.Error("console line not shown")
	}
}
