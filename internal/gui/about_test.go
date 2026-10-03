package gui

import (
	"os"
	"strings"
	"testing"

	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/event"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/widget"
)

func TestAboutText(t *testing.T) {
	txt := aboutText(os.DirFS("../../assets"))
	for _, want := range []string{"Magnific - Flaticon", "Midev - Flaticon", "https://www.flaticon.com/free-icons/copy",
		"Inter", "SIL Open Font License", "JetBrains", "restim", "FFmpeg", "Copyright",
		"github.com/gogpu/ui", "github.com/ncruces/zenity", "Apache License"} {
		if !strings.Contains(txt, want) {
			t.Errorf("about text lacks %q", want)
		}
	}
}

func TestEveryCreditHasLicenseText(t *testing.T) {
	for p := range moduleCredits {
		f := "licenses/" + strings.ReplaceAll(p, "/", "_") + ".txt"
		if _, err := licenseFS.ReadFile(f); err != nil {
			t.Errorf("%s: missing %s", p, f)
		}
	}
}

func findIconButton(w widget.Widget, alt string) *iconButton {
	if b, ok := w.(*iconButton); ok && b.alt == alt {
		return b
	}
	if p, ok := w.(interface{ Children() []widget.Widget }); ok {
		for _, c := range p.Children() {
			if b := findIconButton(c, alt); b != nil {
				return b
			}
		}
	}
	return nil
}

// TestClearButton clicks the clear icon through the window's event dispatch.
func TestClearButton(t *testing.T) {
	a := app.New()
	u := newUI(nil, a)
	u.icons = os.DirFS("../../assets")
	a.SetRoot(u.build())
	u.logf("something")
	a.Frame()
	b := findIconButton(a.Window().Root(), "Clear")
	if b == nil || b.img == nil {
		t.Fatal("clear button or its icon missing")
	}
	c := globalCenter(b)
	for _, typ := range []event.MouseEventType{event.MouseMove, event.MousePress, event.MouseRelease} {
		a.HandleEvent(event.NewMouseEvent(typ, event.ButtonLeft, 0, c, c, 0))
	}
	if len(u.logLines) != 0 {
		t.Errorf("log not cleared: %v", u.logLines)
	}
}

// globalCenter returns w's center in window coordinates (bounds are
// relative to the parent).
func globalCenter(w interface {
	Bounds() geometry.Rect
	Parent() widget.Widget
}) geometry.Point {
	p := w.Bounds().Center()
	for par := w.Parent(); par != nil; {
		b, ok := par.(interface {
			Bounds() geometry.Rect
			Parent() widget.Widget
		})
		if !ok {
			break
		}
		p = p.Add(b.Bounds().Min)
		par = b.Parent()
	}
	return p
}

func TestShowAbout(t *testing.T) {
	a := app.New()
	u := newUI(nil, a)
	u.icons = os.DirFS("../../assets")
	a.SetRoot(u.build())
	a.Frame()
	u.showAbout()
	a.Frame()
	r := findText(a.Window().Root(), "Third-party components and attributions:")
	if r == nil || r.Bounds().Height() <= 0 || r.Bounds().Width() < 100 {
		t.Fatalf("about text not laid out: %v", r)
	}
}
