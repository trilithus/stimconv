package gui

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/pipeline"
	"github.com/trilithus/stimconv/internal/presets"
)

const labelW = 240 // width of the option name column

func heading(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.TextStyle.Bold = true
	l.Importance = widget.HighImportance
	return l
}

// text is a word-wrapped label.
func text(s string, imp widget.Importance) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = imp
	return l
}

func muted(s string) *widget.Label {
	l := text(s, widget.LowImportance)
	l.SizeName = theme.SizeNameCaptionText
	return l
}

// dynText is a wrapped label that follows src; it is hidden while empty.
func (u *ui) dynText(src func() string, imp widget.Importance) *widget.Label {
	l := text("", imp)
	u.onRefresh(func() {
		s := src()
		if l.Text != s {
			l.SetText(s)
		}
		if s == "" {
			l.Hide()
		} else {
			l.Show()
		}
	})
	return l
}

func (u *ui) onRefresh(f func()) { u.refreshers = append(u.refreshers, f) }

func panel(children ...fyne.CanvasObject) fyne.CanvasObject {
	return widget.NewCard("", "", container.NewVBox(children...))
}

// fixedWidth lays out its single child at a fixed width.
type fixedWidth float32

func (w fixedWidth) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(float32(w), objs[0].MinSize().Height)
}

func (w fixedWidth) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(s)
}

func withWidth(w float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(fixedWidth(w), o)
}

// check is a checkbox bound to v.
func (u *ui) check(label string, v *value[bool], disabled func() bool) *widget.Check {
	c := widget.NewCheck(label, nil)
	c.SetChecked(v.Get())
	c.OnChanged = func(b bool) { v.Set(b) }
	u.onRefresh(func() {
		if c.Checked != v.Get() {
			c.SetChecked(v.Get())
		}
		if disabled != nil && disabled() {
			c.Disable()
		} else {
			c.Enable()
		}
	})
	return c
}

// entry is a single-line text field bound to v.
func (u *ui) entry(v *value[string], placeholder string) *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder(placeholder)
	e.SetText(v.Get())
	e.OnChanged = func(s string) { v.Set(s) }
	u.onRefresh(func() {
		if e.Text != v.Get() {
			e.SetText(v.Get())
		}
	})
	return e
}

func (u *ui) build() fyne.CanvasObject {
	u.refreshers = nil
	u.optScroll = nil
	var root fyne.CanvasObject
	if u.about {
		root = container.NewPadded(u.aboutPage())
	} else {
		// input and output keep their size (output grows with the log);
		// the options fill the rest
		root = container.NewBorder(u.inputSection(), container.NewVBox(u.outputSection(), u.statusBar()), nil, nil, u.optionsSection())
	}
	u.relayout()
	return root
}

// statusBar is the bottom line of the window, drawn like a classic
// Windows status bar: a grey strip holding a sunken field. Long messages
// are cut with an ellipsis.
func (u *ui) statusBar() fyne.CanvasObject {
	t := canvas.NewText("", colStatusText)
	t.TextSize = theme.CaptionTextSize()
	field := newSunken(t)
	u.onRefresh(func() {
		s := u.status.Get()
		if s == "" {
			s = "Ready"
		}
		if field.full != s {
			field.full = s
			field.Refresh()
		}
	})
	return container.NewStack(canvas.NewRectangle(colStatusFace), container.New(statusPad{}, field))
}

var (
	colStatusFace   = color.NRGBA{0xc0, 0xc0, 0xc0, 0xff}
	colStatusShadow = color.NRGBA{0x80, 0x80, 0x80, 0xff}
	colStatusLight  = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	colStatusText   = color.NRGBA{0x00, 0x00, 0x00, 0xff}
)

// statusPad insets the status field by 2 px.
type statusPad struct{}

func (statusPad) MinSize(o []fyne.CanvasObject) fyne.Size {
	return o[0].MinSize().Add(fyne.NewSize(4, 4))
}
func (statusPad) Layout(o []fyne.CanvasObject, s fyne.Size) {
	o[0].Move(fyne.NewPos(2, 2))
	o[0].Resize(s.Subtract(fyne.NewSize(4, 4)))
}

// sunken is a one-line text field with a 1 px inset bevel (dark top/left,
// white bottom/right).
type sunken struct {
	widget.BaseWidget
	text *canvas.Text
	full string
}

func newSunken(t *canvas.Text) *sunken {
	s := &sunken{text: t}
	s.ExtendBaseWidget(s)
	return s
}

func (s *sunken) CreateRenderer() fyne.WidgetRenderer {
	r := &sunkenRenderer{s: s,
		top: canvas.NewRectangle(colStatusShadow), left: canvas.NewRectangle(colStatusShadow),
		bottom: canvas.NewRectangle(colStatusLight), right: canvas.NewRectangle(colStatusLight)}
	return r
}

type sunkenRenderer struct {
	s                        *sunken
	top, left, bottom, right *canvas.Rectangle
}

func (r *sunkenRenderer) MinSize() fyne.Size {
	h := fyne.MeasureText("Ag", r.s.text.TextSize, r.s.text.TextStyle).Height
	return fyne.NewSize(20, h+4)
}

func (r *sunkenRenderer) Layout(sz fyne.Size) {
	r.top.Move(fyne.NewPos(0, 0))
	r.top.Resize(fyne.NewSize(sz.Width, 1))
	r.left.Move(fyne.NewPos(0, 0))
	r.left.Resize(fyne.NewSize(1, sz.Height))
	r.bottom.Move(fyne.NewPos(0, sz.Height-1))
	r.bottom.Resize(fyne.NewSize(sz.Width, 1))
	r.right.Move(fyne.NewPos(sz.Width-1, 0))
	r.right.Resize(fyne.NewSize(1, sz.Height))
	t := r.s.text
	t.Text = ellipsize(r.s.full, sz.Width-10, t.TextSize, t.TextStyle)
	t.Move(fyne.NewPos(5, 2))
	t.Resize(fyne.NewSize(sz.Width-10, sz.Height-4))
}

func (r *sunkenRenderer) Refresh() {
	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}

func (r *sunkenRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.text, r.top, r.left, r.bottom, r.right}
}

func (r *sunkenRenderer) Destroy() {}

// ellipsize shortens s with "…" until it fits in w.
func ellipsize(s string, w, size float32, style fyne.TextStyle) string {
	if fyne.MeasureText(s, size, style).Width <= w {
		return s
	}
	rs := []rune(s)
	for n := len(rs) - 1; n > 0; n-- {
		if c := string(rs[:n]) + "…"; fyne.MeasureText(c, size, style).Width <= w {
			return c
		}
	}
	return ""
}

// ---- input ----

func (u *ui) inputSection() fyne.CanvasObject {
	in := u.entry(u.input, "Audio or video file (mp3, wav, flac, mkv, mp4, …)")
	in.OnSubmitted = u.setInput
	items := []fyne.CanvasObject{
		heading("1  Input"),
		container.NewBorder(nil, nil, nil, widget.NewButton("Browse…", u.browseInput), in),
		u.dynText(u.info.Get, widget.LowImportance),
		u.dynText(u.guidance.Get, widget.MediumImportance),
		u.dynText(u.contentWarn.Get, widget.WarningImportance),
	}
	if len(u.inputs) > 0 {
		items = append(items, muted("Batch: the files are converted one after another, each with its default audio track. Type a single path and press Enter to leave batch mode."))
	}
	if len(u.tracks) > 1 {
		items = append(items, u.trackRow())
	}
	return panel(items...)
}

// trackRow selects the audio track of a multi-track (video) file.
func (u *ui) trackRow() fyne.CanvasObject {
	labels := make([]string, len(u.tracks))
	for i, t := range u.tracks {
		labels[i] = fmt.Sprintf("%d · %s", t.Index+1, t)
	}
	sel := u.track - 1
	if sel < 0 {
		sel = decode.DefaultTrack(u.tracks)
	}
	s := widget.NewSelect(labels, nil)
	s.SetSelectedIndex(sel)
	s.OnChanged = func(string) { u.selectTrack(s.SelectedIndex() + 1) }
	return container.NewVBox(
		container.NewBorder(nil, nil, withWidth(labelW, widget.NewLabel("Audio track")), nil, s),
		muted("This file has several audio tracks. Pick the one that carries the stim signal (left = channel A, right = channel B)."),
	)
}

// ---- options ----

func (u *ui) optionsSection() fyne.CanvasObject {
	items := []fyne.CanvasObject{u.presetRow()}
	group := ""
	for i := range config.Options {
		o := &config.Options[i]
		if (o.Advanced && !u.expert) || (o.OnlyIf != nil && !o.OnlyIf(u.cfg)) {
			continue
		}
		if o.Group != group {
			if group == config.GroupOutput {
				items = append(items, u.outputFiles()...)
			}
			group = o.Group
			g := widget.NewLabel(group)
			g.TextStyle.Bold = true
			items = append(items, widget.NewSeparator(), g)
		}
		items = append(items, u.optionRow(o))
	}
	if !u.expert {
		items = append(items, muted("More settings (mixing, circuit model, funscript ranges) are available in expert mode."))
	}
	items = append(items, u.dynText(u.optErr.Get, widget.DangerImportance))
	u.optScroll = vscroll(container.NewVBox(items...))
	return widget.NewCard("", "", container.NewBorder(heading("2  Options"), nil, nil, nil, u.optScroll))
}

// presetHelp describes the selected built-in preset (if any) and how presets
// work.
func (u *ui) presetHelp() string {
	h := "Picking a preset loads it; the bracketed ones are built in. Save stores the current options under the typed name (letters, digits, - and _). With “Add preset to folder name”, the preset name is also added to the default output folder."
	if b, ok := presets.Lookup(u.preset); ok {
		h = b.Desc + " " + h
	}
	return h
}

func (u *ui) presetRow() fyne.CanvasObject {
	names := presets.List()
	sel := widget.NewSelect(names, nil)
	sel.SetSelectedIndex(max(slices.Index(names, u.preset), 0))
	sel.OnChanged = func(v string) { u.post(func() { u.loadPreset(v) }) }
	name := u.entry(u.presetName, "preset name")
	name.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		return presets.ValidateName(s)
	}
	expert := widget.NewCheck("Expert mode", nil)
	expert.SetChecked(u.expert)
	expert.OnChanged = func(b bool) { u.post(func() { u.expert = b; u.rebuild() }) }
	return container.NewVBox(
		container.NewHBox(
			widget.NewLabel("Preset"), sel, withWidth(180, name),
			widget.NewButton("Save", u.savePreset),
			widget.NewButton("Delete", u.deletePreset),
			layout.NewSpacer(), expert,
		),
		muted(u.presetHelp()),
	)
}

func (u *ui) optionRow(o *config.Option) fyne.CanvasObject {
	var ctl fyne.CanvasObject
	switch o.Kind {
	case config.Enum:
		cur := o.Get(&u.cfg).(string)
		labels := make([]string, len(o.Choices))
		sel := 0
		for i, c := range o.Choices {
			labels[i] = c.Label
			if c.Value == cur {
				sel = i
			}
		}
		if o.Key == "topology" {
			r := widget.NewRadioGroup(labels, nil)
			r.SetSelected(labels[sel])
			r.OnChanged = func(l string) {
				if i := slices.Index(labels, l); i >= 0 {
					u.post(func() { u.setOpt(o, o.Choices[i].Value, true) })
				}
			}
			ctl = r
		} else {
			s := widget.NewSelect(labels, nil)
			s.SetSelectedIndex(sel)
			s.OnChanged = func(string) {
				i := s.SelectedIndex()
				u.post(func() { u.setOpt(o, o.Choices[i].Value, true) })
			}
			ctl = s
		}
	case config.Bool:
		c := widget.NewCheck("enabled", nil)
		c.SetChecked(o.Get(&u.cfg).(bool))
		c.OnChanged = func(b bool) { u.post(func() { u.setOpt(o, b, true) }) }
		ctl = c
	case config.Float:
		e := widget.NewEntry()
		e.SetText(strconv.FormatFloat(o.Get(&u.cfg).(float64), 'g', -1, 64))
		e.Validator = func(s string) error {
			if _, err := strconv.ParseFloat(s, 64); err != nil {
				return fmt.Errorf("not a number")
			}
			return nil
		}
		e.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				u.setOpt(o, v, false)
			}
		}
		ctl = e
	}
	def := config.Default()
	label := widget.NewLabel(o.Label)
	label.Truncation = fyne.TextTruncateEllipsis
	u.onRefresh(func() {
		s := o.Label
		if fmt.Sprint(o.Get(&u.cfg)) != fmt.Sprint(o.Get(&def)) {
			s += " •"
		}
		if label.Text != s {
			label.SetText(s)
		}
	})
	reset := widget.NewButtonWithIcon("", theme.ContentUndoIcon(), func() { u.post(func() { u.setOpt(o, o.Get(&def), true) }) })
	reset.Importance = widget.LowImportance
	return container.NewVBox(
		container.NewBorder(nil, nil, withWidth(labelW, label), reset, ctl),
		muted(o.Help),
	)
}

// setOpt stores a value; rebuild re-creates the tree so dependent options
// appear or disappear and controls show the new value.
func (u *ui) setOpt(o *config.Option, v any, rebuild bool) {
	o.Set(&u.cfg, v)
	u.validate()
	if rebuild {
		u.rebuild()
	}
}

func (u *ui) validate() {
	if err := u.cfg.Validate(); err != nil {
		u.optErr.Set("⚠ " + err.Error())
	} else {
		u.optErr.Set("")
	}
	u.relayout()
}

// ---- output ----

// outputFiles is the options block for where the funscripts go and how
// they are named.
func (u *ui) outputFiles() []fyne.CanvasObject {
	name := u.entry(u.outName, "name without extension, e.g. PEP11 for PEP11.fr.mp3")
	name.Validator = func(s string) error { return pipeline.ValidateOutName(s) }
	u.onRefresh(func() {
		if u.outNameOn.Get() {
			name.Enable()
		} else {
			name.Disable()
		}
	})
	g := widget.NewLabel("Output files")
	g.TextStyle.Bold = true
	return []fyne.CanvasObject{
		widget.NewSeparator(), g,
		container.NewBorder(nil, nil, withWidth(labelW, widget.NewLabel("Output folder")), widget.NewButton("Browse…", u.browseOutDir),
			u.entry(u.outDir, "empty = default below")),
		container.NewBorder(nil, nil, withWidth(labelW, u.check("Output name", u.outNameOn, nil)), nil, name),
		container.NewHBox(
			u.check("Write to a sub-folder", u.subfolder, nil),
			u.check("Add preset to folder name", u.presetSuffix, func() bool { return !u.subfolder.Get() }),
		),
		u.dynText(u.defaultOutText, widget.LowImportance),
	}
}

func (u *ui) outputSection() fyne.CanvasObject {
	start := widget.NewButton("Start", u.startOrCancel)
	start.Importance = widget.HighImportance
	u.onRefresh(func() {
		if u.running.Get() {
			start.SetText("Cancel")
		} else {
			start.SetText("Start")
		}
		if !u.running.Get() && (u.input.Get() == "" || u.optErr.Get() != "" || u.outNameErr() != "") {
			start.Disable()
		} else {
			start.Enable()
		}
	})
	return panel(
		container.NewHBox(
			heading("3  Output"),
			u.check("Print statistics", u.stats, nil),
			u.check("Dry run (analyse only)", u.dryRun, nil),
			layout.NewSpacer(),
			start,
		),
		u.logSection(),
	)
}

// logSection is the collapsible log console with its Clear/Copy buttons.
func (u *ui) logSection() fyne.CanvasObject {
	toggle := func(label string, open bool) fyne.CanvasObject {
		b := widget.NewButton(label, func() { u.post(func() { u.logOpen = open; u.rebuild() }) })
		b.Importance = widget.LowImportance
		return container.NewHBox(b)
	}
	if !u.logOpen {
		return toggle("Show log", true)
	}
	clear := widget.NewButtonWithIcon("", u.icon("trash.png", theme.DeleteIcon()), u.clearLog)
	clear.Importance = widget.LowImportance
	cp := widget.NewButtonWithIcon("", u.icon("copy.png", theme.ContentCopyIcon()), u.copyLog)
	cp.Importance = widget.LowImportance
	return container.NewVBox(
		toggle("Hide log", false),
		container.NewBorder(nil, nil, nil, container.NewVBox(clear, cp), u.console()),
	)
}

// console is the log view; it follows the tail when lines are added.
func (u *ui) console() fyne.CanvasObject {
	rt := widget.NewRichText()
	rt.Wrapping = fyne.TextWrapWord
	scroll := vscroll(rt)
	scroll.SetMinSize(fyne.NewSize(0, 150))
	sym := consoleSymbols()
	shown := -1
	u.onRefresh(func() {
		if shown == u.logRev {
			return
		}
		shown = u.logRev
		lines := u.logLines[max(0, len(u.logLines)-logRows):]
		segs := make([]widget.RichTextSegment, len(lines))
		for i, e := range lines {
			col := theme.ColorNameForeground
			switch e.lvl {
			case levelWarn:
				col = theme.ColorNameWarning
			case levelError:
				col = theme.ColorNameError
			}
			segs[i] = &widget.TextSegment{Text: sym.sym[e.lvl] + " " + e.text,
				Style: widget.RichTextStyle{ColorName: col, SizeName: theme.SizeNameCaptionText, TextStyle: fyne.TextStyle{Monospace: true}}}
		}
		rt.Segments = segs
		rt.Refresh()
		scroll.ScrollToBottom()
	})
	return scroll
}

// vscroll is a vertical scroll whose content leaves room on the right for
// the scroll bar, which Fyne draws over the content.
func vscroll(o fyne.CanvasObject) *container.Scroll {
	gap := canvas.NewRectangle(color.Transparent)
	gap.SetMinSize(fyne.NewSize(theme.ScrollBarSize()+theme.Padding(), 0))
	return container.NewVScroll(container.NewBorder(nil, nil, nil, gap, o))
}
