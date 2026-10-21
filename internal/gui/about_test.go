package gui

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestAboutText(t *testing.T) {
	txt := aboutText(os.DirFS("../../assets"))
	for _, want := range []string{"Magnific - Flaticon", "Midev - Flaticon", "https://www.flaticon.com/free-icons/copy",
		"Noto Sans", "SIL Open Font License", "EmojiOne", "restim", "FFmpeg", "Copyright",
		"fyne.io/fyne/v2", "github.com/ncruces/zenity", "Apache License"} {
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

// TestClearButton taps the clear icon.
func TestClearButton(t *testing.T) {
	u := newTestUI(t)
	u.logOpen = true // collapsed by default
	w := shot(t, u, "", "", winW, winH)
	u.logf("something")
	var clear *widget.Button
	walk(w.Content(), func(c fyne.CanvasObject) {
		if b, ok := c.(*widget.Button); ok && b.Icon != nil && b.Icon.Name() == "trash.png" {
			clear = b
		}
	})
	if clear == nil {
		t.Fatal("clear button or its icon missing")
	}
	test.Tap(clear)
	if len(u.logLines) != 0 {
		t.Errorf("log not cleared: %v", u.logLines)
	}
}

func TestShowAbout(t *testing.T) {
	u := newTestUI(t)
	w := shot(t, u, "", "", winW, winH)
	u.showAbout()
	var body *widget.Label
	walk(w.Content(), func(c fyne.CanvasObject) {
		if l, ok := c.(*widget.Label); ok && strings.Contains(l.Text, "Third-party components and attributions:") {
			body = l
		}
	})
	if body == nil || body.Size().Width < 100 || body.Size().Height <= 0 {
		t.Fatalf("about text not laid out: %v", body)
	}
}
