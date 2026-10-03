package gui

import (
	"bytes"
	"testing"

	"github.com/gogpu/ui/offscreen"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/widget"
)

// TestSymbolTiers checks glyph detection against the font gogpu/ui embeds
// (Inter): no emoji, but the unicode symbols. If a gogpu/ui update drops
// those glyphs this fails, and the app would need the text tier.
func TestSymbolTiers(t *testing.T) {
	missing := renderGlyphs("\u0378")
	if bytes.Equal(missing, renderGlyphs("A")) {
		t.Fatal("offscreen renderer cannot tell glyphs apart")
	}
	if symbolTiers[0].drawable(missing) {
		t.Error("emoji tier reported drawable; Inter has no emoji")
	}
	if !symbolTiers[1].drawable(missing) {
		t.Error("unicode tier (● ⚠ ✗) not drawable with the UI font")
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

// renderGlyphs draws s on a small white canvas and returns the pixels.
func renderGlyphs(s string) []byte {
	r := offscreen.NewRenderer(32, 24, offscreen.WithBackground(widget.ColorWhite))
	r.Render(primitives.Text(s).FontSize(16).Color(widget.ColorBlack))
	if img := r.Image(); img != nil {
		return img.Pix
	}
	return nil
}
