package gui

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/gogpu/ui/core/button"
	"github.com/gogpu/ui/core/checkbox"
	"github.com/gogpu/ui/core/dropdown"
	"github.com/gogpu/ui/core/menu"
	"github.com/gogpu/ui/core/radio"
	"github.com/gogpu/ui/core/scrollview"
	"github.com/gogpu/ui/core/textfield"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/state"
	"github.com/gogpu/ui/widget"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/pipeline"
	"github.com/trilithus/stimconv/internal/presets"
)

var (
	colText    = widget.RGBA8(30, 30, 36, 255)
	colMuted   = widget.RGBA8(110, 110, 120, 255)
	colAccent  = widget.RGBA8(63, 90, 168, 255)
	colError   = widget.RGBA8(180, 40, 40, 255)
	colWarn    = widget.RGBA8(176, 96, 0, 255)
	colPanel   = widget.RGBA8(255, 255, 255, 255)
	colBack    = widget.RGBA8(238, 240, 245, 255)
	colConsole = widget.RGBA8(24, 26, 32, 255)
	colStatus  = widget.RGBA8(222, 226, 234, 255)
	colLog     = widget.RGBA8(215, 220, 228, 255)
	colLogWarn = widget.RGBA8(255, 190, 90, 255)
	colLogErr  = widget.RGBA8(255, 120, 120, 255)
)

func heading(s string) *primitives.TextWidget {
	return primitives.Text(s).FontSize(15).Bold().Color(colAccent)
}

func muted(s string) *wrapLabel {
	return label(s, 11, colMuted)
}

func panel(children ...widget.Widget) *primitives.BoxWidget {
	return primitives.VBox(children...).Padding(14).Gap(8).Background(colPanel).Rounded(8)
}

func btn(label string, v button.Variant, onClick func()) *button.Widget {
	return button.New(button.Text(label), button.VariantOpt(v), button.OnClick(onClick))
}

func (u *ui) build() widget.Widget {
	if u.about {
		return primitives.VBox(
			u.menuBar(),
			primitives.Expanded(primitives.VBox(primitives.Expanded(u.aboutPage())).Padding(12)),
		).Background(colBack)
	}
	return primitives.VBox(
		u.menuBar(),
		primitives.Expanded(primitives.VBox(
			u.inputSection(),
			primitives.Expanded(u.optionsSection()),
			u.outputSection(),
		).Padding(12).Gap(10)),
		u.statusBar(),
	).Background(colBack)
}

// statusBar is the bottom line of the window; it wraps long messages such
// as output paths instead of widening the layout.
func (u *ui) statusBar() widget.Widget {
	return primitives.HBox(primitives.Expanded(newWrapLabel(func() string {
		if s := u.status.Get(); s != "" {
			return s
		}
		return "Ready"
	}, 12, colText, 2))).PaddingXY(12, 4).Background(colStatus)
}

// ---- input ----

func (u *ui) inputSection() widget.Widget {
	items := []widget.Widget{
		heading("1  Input"),
		primitives.HBox(
			primitives.Expanded(textfield.New(
				textfield.ValueSignal(u.input),
				textfield.Placeholder("Audio or video file (mp3, wav, flac, mkv, mp4, …)"),
				textfield.OnSubmit(u.setInput),
			)),
			btn("Browse…", button.Tonal, u.browseInput),
		).Gap(8).CrossAlign(primitives.CrossAxisCenter),
		newWrapLabel(u.info.Get, 12, colMuted, 200),
		newWrapLabel(u.guidance.Get, 12, colText, 200),
		newWrapLabel(u.contentWarn.Get, 12, colWarn, 200),
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
func (u *ui) trackRow() widget.Widget {
	labels := make([]string, len(u.tracks))
	for i, t := range u.tracks {
		labels[i] = fmt.Sprintf("%d · %s", t.Index+1, t)
	}
	sel := u.track - 1
	if sel < 0 {
		sel = decode.DefaultTrack(u.tracks)
	}
	return primitives.VBox(
		primitives.HBox(
			primitives.Box(primitives.Text("Audio track").FontSize(13).Color(colText)).Width(240),
			primitives.Expanded(dropdown.New(dropdown.Items(labels...), dropdown.Selected(sel),
				dropdown.OnChange(func(i int, _ string) { u.post(func() { u.selectTrack(i + 1) }) }))),
		).Gap(12).CrossAlign(primitives.CrossAxisCenter),
		muted("This file has several audio tracks. Pick the one that carries the stim signal (left = channel A, right = channel B)."),
	).Gap(2)
}

// ---- options ----

func (u *ui) optionsSection() widget.Widget {
	items := []widget.Widget{heading("2  Options"), u.presetRow()}
	group := ""
	for i := range config.Options {
		o := &config.Options[i]
		if (o.Advanced && !u.expert) || (o.OnlyIf != nil && !o.OnlyIf(u.cfg)) {
			continue
		}
		if o.Group != group {
			group = o.Group
			items = append(items, primitives.Box(primitives.Text(group).FontSize(13).Bold().Color(colText)).PaddingTop(6))
		}
		items = append(items, u.optionRow(o))
	}
	if !u.expert {
		items = append(items, muted("More settings (mixing, circuit model, funscript ranges) are available in expert mode."))
	}
	items = append(items, newWrapLabel(u.optErr.Get, 12, colError, 8))
	body := primitives.VBox(items...).Gap(8).PaddingRight(12)
	return panel(primitives.Expanded(scrollview.New(body, scrollview.ScrollYSignal(u.optScroll))))
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

func (u *ui) presetRow() widget.Widget {
	names := presets.List()
	sel := max(slices.Index(names, u.preset), 0)
	return primitives.VBox(
		primitives.HBox(
			primitives.Text("Preset").FontSize(13).Color(colText),
			dropdown.New(dropdown.Items(names...), dropdown.Selected(sel),
				dropdown.OnChange(func(_ int, v string) { u.post(func() { u.loadPreset(v) }) })),
			primitives.Box(textfield.New(textfield.ValueSignal(u.presetName), textfield.Placeholder("preset name"),
				textfield.Validation(func(s string) string {
					if s == "" {
						return ""
					}
					if err := presets.ValidateName(s); err != nil {
						return err.Error()
					}
					return ""
				}))).Width(180),
			btn("Save", button.Outlined, u.savePreset),
			btn("Delete", button.TextOnly, u.deletePreset),
			primitives.Expanded(primitives.Box()),
			checkbox.New(checkbox.Label("Expert mode"), checkbox.Checked(u.expert),
				checkbox.OnToggle(func(b bool) { u.post(func() { u.expert = b; u.rebuild() }) })),
		).Gap(8).CrossAlign(primitives.CrossAxisCenter),
		muted(u.presetHelp()),
	).Gap(4)
}

func (u *ui) optionRow(o *config.Option) widget.Widget {
	var ctl widget.Widget
	switch o.Kind {
	case config.Enum:
		cur := o.Get(&u.cfg).(string)
		if o.Key == "topology" {
			defs := make([]radio.ItemDef, len(o.Choices))
			for i, c := range o.Choices {
				defs[i] = radio.ItemDef{Value: c.Value, Label: c.Label}
			}
			ctl = radio.NewGroup(radio.Items(defs...), radio.Selected(cur), radio.DirectionOpt(radio.Vertical),
				radio.OnChange(func(v string) { u.post(func() { u.setOpt(o, v, true) }) }))
		} else {
			labels := make([]string, len(o.Choices))
			sel := 0
			for i, c := range o.Choices {
				labels[i] = c.Label
				if c.Value == cur {
					sel = i
				}
			}
			ctl = dropdown.New(dropdown.Items(labels...), dropdown.Selected(sel),
				dropdown.OnChange(func(i int, _ string) { u.post(func() { u.setOpt(o, o.Choices[i].Value, true) }) }))
		}
	case config.Bool:
		ctl = checkbox.New(checkbox.Label("enabled"), checkbox.Checked(o.Get(&u.cfg).(bool)),
			checkbox.OnToggle(func(b bool) { u.post(func() { u.setOpt(o, b, true) }) }))
	case config.Float:
		ctl = textfield.New(
			textfield.InitialValue(strconv.FormatFloat(o.Get(&u.cfg).(float64), 'g', -1, 64)),
			textfield.Validation(func(s string) string {
				if _, err := strconv.ParseFloat(s, 64); err != nil {
					return "not a number"
				}
				return ""
			}),
			textfield.OnChange(func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					u.setOpt(o, v, false)
				}
			}),
		)
	}
	def := config.Default()
	label := primitives.Text("").ContentSignal(state.NewComputed(func() string {
		_ = u.rev.Get() // re-evaluate after every change
		if fmt.Sprint(o.Get(&u.cfg)) != fmt.Sprint(o.Get(&def)) {
			return o.Label + " •"
		}
		return o.Label
	}, u.rev)).FontSize(13).Color(colText)
	return primitives.VBox(
		primitives.HBox(
			primitives.Box(label).Width(240),
			primitives.Expanded(ctl),
			btn("↺", button.TextOnly, func() { u.post(func() { u.setOpt(o, o.Get(&def), true) }) }),
		).Gap(12).CrossAlign(primitives.CrossAxisCenter),
		muted(o.Help),
	).Gap(2)
}

// setOpt stores a value; rebuild re-creates the tree so dependent options
// appear or disappear and controls show the new value.
func (u *ui) setOpt(o *config.Option, v any, rebuild bool) {
	o.Set(&u.cfg, v)
	u.rev.Set(u.rev.Get() + 1)
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

func (u *ui) outputSection() widget.Widget {
	startLabel := func() string {
		if u.running.Get() {
			return "Cancel"
		}
		return "Start"
	}
	return panel(
		heading("3  Output"),
		primitives.HBox(
			primitives.Expanded(textfield.New(
				textfield.ValueSignal(u.outDir),
				textfield.Placeholder("Output folder (empty = default below)"),
				textfield.OnChange(func(string) { u.relayout() }),
			)),
			btn("Browse…", button.Tonal, u.browseOutDir),
		).Gap(8).CrossAlign(primitives.CrossAxisCenter),
		primitives.HBox(
			checkbox.New(checkbox.Label("Output name"), checkbox.CheckedSignal(u.outNameOn),
				checkbox.OnToggle(func(bool) { u.post(u.relayout) })),
			primitives.Expanded(textfield.New(
				textfield.ValueSignal(u.outName),
				textfield.Placeholder("name without extension, e.g. PEP11 for PEP11.fr.mp3"),
				textfield.DisabledFn(func() bool { return !u.outNameOn.Get() }),
				textfield.Validation(func(s string) string {
					if err := pipeline.ValidateOutName(s); err != nil {
						return err.Error()
					}
					return ""
				}),
				textfield.OnChange(func(string) { u.relayout() }),
			)),
		).Gap(8).CrossAlign(primitives.CrossAxisCenter),
		primitives.HBox(
			checkbox.New(checkbox.Label("Write to a sub-folder"), checkbox.CheckedSignal(u.subfolder),
				checkbox.OnToggle(func(bool) { u.post(u.relayout) })),
			checkbox.New(checkbox.Label("Add preset to folder name"), checkbox.CheckedSignal(u.presetSuffix),
				checkbox.DisabledFn(func() bool { return !u.subfolder.Get() }),
				checkbox.OnToggle(func(bool) { u.post(u.relayout) })),
		).Gap(12).CrossAlign(primitives.CrossAxisCenter),
		newWrapLabel(u.defaultOutText, 11, colMuted, 4),
		primitives.HBox(
			checkbox.New(checkbox.Label("Print statistics"), checkbox.CheckedSignal(u.stats)),
			checkbox.New(checkbox.Label("Dry run (analyse only)"), checkbox.CheckedSignal(u.dryRun)),
			primitives.Expanded(primitives.Box()),
			button.New(button.TextReadonlySignal(state.NewComputed(startLabel, u.running)), button.OnClick(u.startOrCancel),
				button.DisabledReadonlySignal(state.NewComputed(func() bool {
					return !u.running.Get() && (u.input.Get() == "" || u.optErr.Get() != "" || u.outNameErr() != "")
				}, u.running, u.input, u.optErr, u.outName, u.outNameOn))),
		).Gap(12).CrossAlign(primitives.CrossAxisCenter),
		u.logSection(),
	)
}

// logSection is the collapsible log console with its Clear/Copy buttons.
func (u *ui) logSection() widget.Widget {
	toggle := func(label string, open bool) widget.Widget {
		return btn(label, button.TextOnly, func() { u.post(func() { u.logOpen = open; u.rebuild() }) })
	}
	if !u.logOpen {
		return primitives.HBox(toggle("Show log", true), primitives.Expanded(primitives.Box()))
	}
	return primitives.VBox(
		primitives.HBox(toggle("Hide log", false), primitives.Expanded(primitives.Box())),
		primitives.HBox(
			primitives.Expanded(primitives.Box(scrollview.New(u.console(), scrollview.ScrollYSignal(u.logScroll))).Height(150).Padding(8).Background(colConsole).Rounded(6)),
			primitives.VBox(
				newIconButton(u.icon("trash.png"), "Clear", 32, u.clearLog),
				newIconButton(u.icon("copy.png"), "Copy", 32, u.copyLog),
			).Gap(6),
		).Gap(8),
	).Gap(4)
}

// console is the log view; it follows the tail whenever the row count
// changes.
func (u *ui) console() widget.Widget {
	w := newConsoleView(func() []logEntry { return u.logLines }, consoleSymbols())
	last := -1
	w.onWrap = func(n int) {
		if n != last {
			last = n
			u.post(func() { u.logScroll.Set(max(0, float32(n)*logFont*logLine-logView)) })
		}
	}
	return w
}

func (u *ui) menuBar() widget.Widget {
	return menu.NewBar([]menu.TopMenu{
		menu.BarMenu("File",
			menu.Item("Open audio or video…", "", u.browseInput),
			menu.Sep(),
			menu.Item("Export config…", "", u.exportConfig),
			menu.Item("Open Preset Folder", "", u.openPresetFolder),
			menu.Sep(),
			menu.Item("Quit", "", func() {
				if u.gapp != nil {
					u.gapp.Quit()
				}
			}),
		),
		menu.BarMenu("Help",
			menu.Item("About stimconv…", "", func() { u.post(u.showAbout) }),
		),
	})
}
