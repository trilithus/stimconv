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

// iconButton is a square button that shows an image (gogpu/ui buttons only
// take text, and its Image primitive draws a placeholder).
type iconButton struct {
	widget.WidgetBase
	img     image.Image
	alt     string // drawn instead of img when the icon failed to load
	size    float32
	onClick func()
	hover   bool
	pressed bool
}

func newIconButton(img image.Image, alt string, size float32, onClick func()) *iconButton {
	b := &iconButton{img: img, alt: alt, size: size, onClick: onClick}
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
	canvas.DrawImage(b.img, at)
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
