package gui

import (
	"bytes"
	"testing"

	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// TestSymbolTiers checks that consoleTier is the first symbol tier Fyne's
// default theme font draws. If a Fyne update changes its fonts this fails.
func TestSymbolTiers(t *testing.T) {
	test.NewTempApp(t)
	missing := renderGlyphs("\u0378")
	if bytes.Equal(missing, renderGlyphs("A")) {
		t.Fatal("offscreen renderer cannot tell glyphs apart")
	}
	// consoleTier must be the first tier the font can draw
	first := len(symbolTiers) - 1
	for i, tier := range symbolTiers[:len(symbolTiers)-1] {
		if tier.drawable(missing) {
			first = i
			break
		}
	}
	if first != consoleTier {
		t.Errorf("first drawable tier is %s, but consoleTier is %s; update consoleTier", symbolTiers[first].name, symbolTiers[consoleTier].name)
	}
}

func TestClassify(t *testing.T) {
	for in, want := range map[string]logEntry{
		"warning: 5% clamped":   {levelWarn, "5% clamped"},
		"Error: file not found": {levelError, "file not found"},
		"wrote 10 funscripts":   {levelInfo, "wrote 10 funscripts"},
		"restim: use device":    {levelInfo, "restim: use device"},
	} {
		if got := classify(in); got != want {
			t.Errorf("classify(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func (t symbolSet) drawable(missing []byte) bool {
	for _, s := range t.sym {
		for _, r := range s {
			if r == '️' { // emoji presentation selector, not drawn alone
				continue
			}
			if bytes.Equal(renderGlyphs(string(r)), missing) {
				return false
			}
		}
	}
	return true
}

// renderGlyphs draws s with the console's text style and returns the pixels.
func renderGlyphs(s string) []byte {
	rt := widget.NewRichText(&widget.TextSegment{Text: s, Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Monospace: true}}})
	w := test.NewWindow(rt)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(40, 30))
	return w.Canvas().Capture().(*image.NRGBA).Pix
}
