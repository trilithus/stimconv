package gui

import (
	"image"
	"image/png"
	"io/fs"

	xdraw "golang.org/x/image/draw"

	"github.com/gogpu/ui/event"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/widget"
)

// loadIcon decodes a PNG from fsys and scales it to size×size pixels
// (canvas images are drawn at their pixel size in logical units).
func loadIcon(fsys fs.FS, name string, size int) image.Image {
	if fsys == nil {
		return nil
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// icon returns the 20 px icon name, decoded once per ui.
func (u *ui) icon(name string) image.Image {
	if img, ok := u.iconCache[name]; ok {
		return img
	}
	if u.iconCache == nil {
		u.iconCache = map[string]image.Image{}
	}
	img := loadIcon(u.icons, name, 20)
	u.iconCache[name] = img
	return img
}

// pixelRun is a horizontal span of equal-colour icon pixels.
type pixelRun struct {
	x, y, w int
	c       widget.Color
}

// iconRuns turns img into rectangles. Canvas.DrawImage converts the image
// into a new gg buffer (new GPU cache key) on every draw, so repaints churn
// gg's texture cache and an evicted texture leaves an icon blank.
func iconRuns(img image.Image) []pixelRun {
	b := img.Bounds()
	var runs []pixelRun
	for y := b.Min.Y; y < b.Max.Y; y++ {
		var cur *pixelRun
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a>>12 == 0 { // under 1/16 opacity
				cur = nil
				continue
			}
			// un-premultiply and quantise so antialiased edges merge into runs
			q := func(v uint32) uint8 { return uint8(v * 0xffff / a >> 8 &^ 0x0f) }
			c := widget.RGBA8(q(r), q(g), q(bl), uint8(a>>8&^0x0f))
			if cur != nil && cur.c == c {
				cur.w++
				continue
			}
			runs = append(runs, pixelRun{x - b.Min.X, y - b.Min.Y, 1, c})
			cur = &runs[len(runs)-1]
		}
	}
	return runs
}

// iconButton is a square button that shows an image (gogpu/ui buttons only
// take text, and its Image primitive draws a placeholder).
type iconButton struct {
	widget.WidgetBase
	img     image.Image
	runs    []pixelRun
	alt     string // drawn instead of img when the icon failed to load
	size    float32
	onClick func()
	hover   bool
	pressed bool
}

func newIconButton(img image.Image, alt string, size float32, onClick func()) *iconButton {
	b := &iconButton{img: img, alt: alt, size: size, onClick: onClick}
	if img != nil {
		b.runs = iconRuns(img)
	}
	b.SetVisible(true)
	b.SetEnabled(true)
	return b
}

func (b *iconButton) Layout(_ widget.Context, c geometry.Constraints) geometry.Size {
	s := c.Constrain(geometry.Sz(b.size, b.size))
	b.SetBounds(geometry.FromPointSize(b.Position(), s))
	return s
}

func (b *iconButton) Draw(_ widget.Context, canvas widget.Canvas) {
	r := b.Bounds()
	switch {
	case b.pressed:
		canvas.DrawRoundRect(r, widget.RGBA8(200, 205, 220, 255), 6)
	case b.hover:
		canvas.DrawRoundRect(r, widget.RGBA8(225, 228, 238, 255), 6)
	}
	if b.img == nil {
		canvas.DrawText(b.alt, r, 11, colText, false, widget.TextAlignCenter)
		return
	}
	ib := b.img.Bounds()
	at := geometry.Pt(r.Min.X+(r.Width()-float32(ib.Dx()))/2, r.Min.Y+(r.Height()-float32(ib.Dy()))/2)
	for _, p := range b.runs {
		canvas.DrawRect(geometry.NewRect(at.X+float32(p.x), at.Y+float32(p.y), float32(p.w), 1), p.c)
	}
}

func (b *iconButton) Event(ctx widget.Context, e event.Event) bool {
	ev, ok := e.(*event.MouseEvent)
	if !ok {
		return false
	}
	redraw := func() {
		b.SetNeedsRedraw(true)
		ctx.InvalidateRect(b.Bounds())
	}
	switch ev.MouseType {
	case event.MouseEnter:
		b.hover = true
		ctx.SetCursor(widget.CursorPointer)
	case event.MouseLeave:
		b.hover, b.pressed = false, false
		ctx.SetCursor(widget.CursorDefault)
	case event.MousePress:
		if ev.Button != event.ButtonLeft {
			return false
		}
		b.pressed = true
	case event.MouseRelease:
		if ev.Button != event.ButtonLeft {
			return false
		}
		fire := b.pressed && b.Bounds().Contains(ev.Position)
		b.pressed = false
		if fire && b.onClick != nil {
			b.onClick()
		}
	default:
		return false
	}
	redraw()
	return true
}

func (b *iconButton) Children() []widget.Widget { return nil }
