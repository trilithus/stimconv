package gui

import (
	"strings"

	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/widget"
)

// charW is the average glyph advance per font size used to turn a width
// into a column count (gogpu/ui exposes no text measurement).
const charW = 0.6

// wrapLabel is multi-line text that wraps to the width it is laid out at.
// gogpu/ui text widgets neither wrap nor honour '\n', so it is a column of
// one-line rows that are filled during Layout. Content changes become
// visible on the next ui.relayout.
type wrapLabel struct {
	*primitives.BoxWidget
	src     func() string
	size    float32
	maxRows int
	tail    bool        // keep the last rows instead of the first
	onWrap  func(n int) // optional, called with the row count after wrapping
	lines   []string
}

func newWrapLabel(src func() string, size float32, color widget.Color, maxRows int) *wrapLabel {
	w := &wrapLabel{src: src, size: size, maxRows: maxRows}
	rows := make([]widget.Widget, maxRows)
	for i := range rows {
		rows[i] = primitives.TextFn(func() string {
			if i < len(w.lines) {
				return w.lines[i]
			}
			return ""
		}).FontSize(size).LineHeight(logLine).Color(color)
	}
	w.BoxWidget = primitives.VBox(rows...)
	return w
}

// label wraps static text.
func label(s string, size float32, color widget.Color) *wrapLabel {
	return newWrapLabel(func() string { return s }, size, color, 8)
}

func (w *wrapLabel) Layout(ctx widget.Context, c geometry.Constraints) geometry.Size {
	cols := 1 << 30
	if c.MaxWidth < geometry.Infinity && c.MaxWidth > 0 {
		cols = max(8, int(c.MaxWidth/(w.size*charW)))
	}
	var out []string
	if s := w.src(); s != "" {
		for _, l := range strings.Split(s, "\n") {
			if l == "" {
				l = " " // keep blank lines; empty rows have no height
			}
			out = append(out, wrap(l, cols)...)
		}
	}
	if len(out) > w.maxRows {
		if w.tail {
			out = out[len(out)-w.maxRows:]
		} else {
			out = out[:w.maxRows]
		}
	}
	w.lines = out
	if w.onWrap != nil {
		w.onWrap(len(out))
	}
	return w.BoxWidget.Layout(ctx, c)
}

// wrap breaks s into lines of at most cols runes at the last space that
// fits; a word longer than a whole line is cut only because it would
// otherwise be clipped.
func wrap(s string, cols int) []string {
	r := []rune(s)
	var out []string
	for len(r) > cols {
		cut := -1
		for i := cols; i > 0; i-- {
			if r[i] == ' ' {
				cut = i
				break
			}
		}
		if cut < 0 {
			cut = cols
		}
		out = append(out, strings.TrimRight(string(r[:cut]), " "))
		r = r[cut:]
		for len(r) > 0 && r[0] == ' ' {
			r = r[1:]
		}
	}
	return append(out, string(r))
}

// consoleView is the console: one-line rows like wrapLabel, but each entry gets
// its channel symbol and colour, and wrapped continuations are indented.
type consoleView struct {
	*primitives.BoxWidget
	src    func() []logEntry
	set    symbolSet
	onWrap func(n int)
	rows   []*primitives.TextWidget
	lines  []string
}

func newConsoleView(src func() []logEntry, set symbolSet) *consoleView {
	v := &consoleView{src: src, set: set}
	ws := make([]widget.Widget, logRows)
	v.rows = make([]*primitives.TextWidget, logRows)
	for i := range ws {
		v.rows[i] = primitives.TextFn(func() string {
			if i < len(v.lines) {
				return v.lines[i]
			}
			return ""
		}).FontSize(logFont).LineHeight(logLine).Color(colLog)
		ws[i] = v.rows[i]
	}
	v.BoxWidget = primitives.VBox(ws...)
	return v
}

func (v *consoleView) Layout(ctx widget.Context, c geometry.Constraints) geometry.Size {
	cols := 1 << 30
	if c.MaxWidth < geometry.Infinity && c.MaxWidth > 0 {
		cols = max(8, int(c.MaxWidth/(logFont*charW)))
	}
	indent := strings.Repeat(" ", min(len(v.set.sym[levelWarn])+1, 6))
	type row struct {
		text string
		lvl  level
	}
	var out []row
	for _, e := range v.src() {
		head := v.set.sym[e.lvl] + " "
		for j, l := range wrap(head+e.text, cols) {
			if j > 0 {
				l = indent + l
			}
			out = append(out, row{l, e.lvl})
		}
	}
	if len(out) > logRows {
		out = out[len(out)-logRows:]
	}
	v.lines = v.lines[:0]
	for i, r := range out {
		v.lines = append(v.lines, r.text)
		v.rows[i].Color(levelColor(r.lvl))
	}
	if v.onWrap != nil {
		v.onWrap(len(out))
	}
	return v.BoxWidget.Layout(ctx, c)
}

func levelColor(l level) widget.Color {
	switch l {
	case levelWarn:
		return colLogWarn
	case levelError:
		return colLogErr
	}
	return colLog
}
