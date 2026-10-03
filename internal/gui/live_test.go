package gui

import (
	"strings"
	"testing"

	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/widget"
)

func findText(w widget.Widget, want string) *primitives.TextWidget {
	if t, ok := w.(*primitives.TextWidget); ok && t.Content() == want {
		return t
	}
	if p, ok := w.(interface{ Children() []widget.Widget }); ok {
		for _, c := range p.Children() {
			if r := findText(c, want); r != nil {
				return r
			}
		}
	}
	return nil
}

// TestLiveUpdates checks that signal-driven text is laid out on the next
// frame without a rebuild (it used to stay at zero size until one).
func TestLiveUpdates(t *testing.T) {
	a := app.New()
	u := newUI(nil, a)
	a.SetRoot(u.build())
	a.Frame()

	long := "⚠ cannot decode: native decode (mp3): no frames; ffmpeg fallback: ffmpeg not found (use --ffmpeg, STIMCONV_FFMPEG, libs/ffmpeg/bin or PATH) and some more words to force wrapping"
	u.info.Set(long)
	u.logf("hello console")
	u.status.Set("Done — 12 files in /some/long/output/directory")
	u.relayout()
	a.Frame()

	for _, s := range []string{"● hello console", "Done — 12 files in /some/long/output/directory"} {
		r := findText(a.Window().Root(), s)
		if r == nil {
			t.Fatalf("%q not in tree", s)
		}
		if b := r.Bounds(); b.Width() < 50 || b.Height() <= 0 {
			t.Errorf("%q not laid out: %v", s, b)
		}
	}
}

// TestDynamicTextWraps checks that a long info line wraps to several rows
// that fit the window instead of being clipped.
func TestDynamicTextWraps(t *testing.T) {
	a := app.New()
	u := newUI(nil, a)
	a.SetRoot(u.build())
	a.Frame()
	u.info.Set(strings.Repeat("cannot decode this file because of reasons ", 8))
	u.relayout()
	a.Frame()
	rows := 0
	var walk func(widget.Widget)
	walk = func(w widget.Widget) {
		if tw, ok := w.(*primitives.TextWidget); ok && strings.Contains(tw.Content(), "decode") {
			rows++
			if b := tw.Bounds(); b.Max.X > float32(a.Window().WindowSize().Width) {
				t.Errorf("row clipped: %v", b)
			}
		}
		if p, ok := w.(interface{ Children() []widget.Widget }); ok {
			for _, c := range p.Children() {
				walk(c)
			}
		}
	}
	walk(a.Window().Root())
	if rows < 2 {
		t.Errorf("info wrapped into %d rows, want several", rows)
	}
}
