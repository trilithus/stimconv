// Command icon draws the stimconv application icon and writes
// assets/icon.png (window icon) and assets/icon.ico (Windows exe icon).
// Regenerate with `go generate` (see assets.go).
//
// The motif: an audio waveform (left) turning into stimulation pulses
// (right) on a rounded blue tile.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/vector"
)

var (
	bgTop    = color.RGBA{0x4A, 0x68, 0xC0, 0xFF}
	bgBottom = color.RGBA{0x2A, 0x3F, 0x80, 0xFF}
	stroke   = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	accent   = color.RGBA{0xFF, 0xA9, 0x4D, 0xFF}
)

// render draws the icon at s×s pixels. Strokes get relatively thicker at
// small sizes so the motif stays legible at 16 px.
func render(s int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	f := float32(s)
	pad := f * 0.04
	radius := f * 0.2

	// background: vertical gradient clipped to a rounded square
	mask := vector.NewRasterizer(s, s)
	roundRect(mask, pad, pad, f-pad, f-pad, radius)
	grad := image.NewRGBA(img.Bounds())
	for y := 0; y < s; y++ {
		t := float64(y) / float64(s-1)
		c := color.RGBA{lerp(bgTop.R, bgBottom.R, t), lerp(bgTop.G, bgBottom.G, t), lerp(bgTop.B, bgBottom.B, t), 0xFF}
		draw.Draw(grad, image.Rect(0, y, s, y+1), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	mask.Draw(img, img.Bounds(), grad, image.Point{})

	w := f * 0.06
	if s <= 32 {
		w = f * 0.1
	}
	mid := f * 0.52
	amp := f * 0.2
	x0, x1, x2 := f*0.16, f*0.5, f*0.84

	// small sizes get a simpler motif so the strokes stay apart
	halfPeriods, nPulses := 3.0, 2
	if s <= 32 {
		halfPeriods, nPulses = 2, 1
	}

	// left half: the audio waveform
	var pts [][2]float32
	for i := 0; i <= 48; i++ {
		t := float64(i) / 48
		x := x0 + float32(t)*(x1-x0)
		y := mid - amp*float32(math.Sin(t*halfPeriods*math.Pi))
		pts = append(pts, [2]float32{x, y})
	}
	polyline(img, pts, w, stroke)

	// right half: rectangular pulses (the stimulation)
	pw := (x2 - x1) / float32(2*nPulses)
	pulses := [][2]float32{{x1, mid}}
	for i := 0; i < nPulses; i++ {
		a := x1 + pw*(float32(2*i)+0.5)
		pulses = append(pulses, [2]float32{a, mid}, [2]float32{a, mid - amp}, [2]float32{a + pw, mid - amp}, [2]float32{a + pw, mid})
	}
	pulses = append(pulses, [2]float32{x2, mid})
	polyline(img, pulses, w, accent)
	return img
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5) }

func roundRect(r *vector.Rasterizer, x0, y0, x1, y1, rad float32) {
	const k = 0.5523 // cubic approximation of a quarter circle
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.CubeTo(x1-rad+rad*k, y0, x1, y0+rad-rad*k, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.CubeTo(x1, y1-rad+rad*k, x1-rad+rad*k, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.CubeTo(x0+rad-rad*k, y1, x0, y1-rad+rad*k, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.CubeTo(x0, y0+rad-rad*k, x0+rad-rad*k, y0, x0+rad, y0)
	r.ClosePath()
}

// polyline strokes pts with round joins and caps. Each piece is filled on
// its own: overlapping pieces in one rasterizer can cancel by winding.
func polyline(img *image.RGBA, pts [][2]float32, w float32, c color.Color) {
	s := img.Bounds().Dx()
	src := &image.Uniform{c}
	fill := func(build func(r *vector.Rasterizer)) {
		r := vector.NewRasterizer(s, s)
		build(r)
		r.Draw(img, img.Bounds(), src, image.Point{})
	}
	h := w / 2
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b[0]-a[0], b[1]-a[1]
		l := float32(math.Hypot(float64(dx), float64(dy)))
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*h, dx/l*h
		fill(func(r *vector.Rasterizer) {
			r.MoveTo(a[0]+nx, a[1]+ny)
			r.LineTo(b[0]+nx, b[1]+ny)
			r.LineTo(b[0]-nx, b[1]-ny)
			r.LineTo(a[0]-nx, a[1]-ny)
			r.ClosePath()
		})
	}
	for _, p := range pts { // joins and caps
		fill(func(r *vector.Rasterizer) { circle(r, p[0], p[1], h) })
	}
}

func circle(r *vector.Rasterizer, cx, cy, rad float32) {
	const n = 16
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / n
		x, y := cx+rad*float32(math.Cos(a)), cy+rad*float32(math.Sin(a))
		if i == 0 {
			r.MoveTo(x, y)
		} else {
			r.LineTo(x, y)
		}
	}
	r.ClosePath()
}

// writeICO writes PNG-compressed icon entries (supported since Vista).
func writeICO(path string, sizes []int) error {
	var imgs [][]byte
	for _, s := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, render(s)); err != nil {
			return err
		}
		imgs = append(imgs, b.Bytes())
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0 // 0 means 256
		}
		binary.Write(&out, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, BPP            uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(imgs[i])), uint32(offset)})
		offset += len(imgs[i])
	}
	for _, b := range imgs {
		out.Write(b)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}

func run() error {
	var b bytes.Buffer
	if err := png.Encode(&b, render(256)); err != nil {
		return err
	}
	if err := os.WriteFile("assets/icon.png", b.Bytes(), 0o644); err != nil {
		return err
	}
	return writeICO("assets/icon.ico", []int{16, 20, 24, 32, 40, 48, 64, 128, 256})
}
