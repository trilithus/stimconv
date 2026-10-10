// Package gui is the graphical front end: one window laid out as
// input → options → output, built with gogpu/ui.
package gui

import (
	"image"
	"io/fs"
	"sync"

	"github.com/gogpu/gogpu"
	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/desktop"
	"github.com/gogpu/ui/dnd"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/state"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/presets"
)

const winW, winH = 960, 860

// ui holds all window state. Widgets and signals are only touched on the UI
// thread; other goroutines hand work over with post.
type ui struct {
	gapp  *gogpu.App
	icons fs.FS // icon PNGs and their attribution .txt files
	// iconCache keeps decoded icons across rebuilds: the renderer caches
	// textures per image, and a fresh image each rebuild drew blank until hover.
	iconCache map[string]image.Image
	app       *app.App

	cfg    config.Config
	expert bool
	// logOpen shows the log console; it opens on Start.
	logOpen bool
	about   bool // About page shown instead of the main screen

	tracks []decode.Track // audio tracks of the input when it has several
	track  int            // selected track, 1-based; 0 = default

	inputs []string // batch: two or more input files (nil for a single file)
	selGen int      // bumped on every selection; stale probe results are dropped
	preset string   // selected preset in the dropdown

	input, outDir, presetName state.Signal[string]
	outName                   state.Signal[string] // output name override, used when outNameOn
	outNameOn                 state.Signal[bool]
	subfolder                 state.Signal[bool] // default output goes to a folder next to the input
	presetSuffix              state.Signal[bool] // that folder ends in .<preset>
	info, status              state.Signal[string]
	contentWarn               state.Signal[string] // stim/music check of the input
	stats, dryRun, running    state.Signal[bool]
	optScroll, logScroll      state.Signal[float32]
	aboutScroll               state.Signal[float32]
	optErr                    state.Signal[string]
	rev                       state.Signal[int] // bumped on every option change
	logLines                  []logEntry
	cancel                    func()

	mu    sync.Mutex
	queue []func()
}

// Run opens the window and blocks until it is closed. appIcon is the window
// icon (X11; on Windows it comes from the exe resource, see setWindowIcon).
func Run(icons fs.FS, appIcon image.Image) error {
	m3 := material3.New(widget.Hex(0x3F5AA8))
	preferX11OnWSL()
	g := gogpu.NewApp(graphicsAPI(gogpu.DefaultConfig().WithTitle("stimconv").WithSize(winW, winH).WithIcon(appIcon)))
	a := app.New(
		app.WithWindowProvider(g),
		app.WithPlatformProvider(g),
		app.WithEventSource(g.EventSource()),
		app.WithTheme(m3.AsTheme()),
	)
	u := newUI(g, a)
	u.icons = icons
	iconSet := false
	g.OnUpdate(func(float64) {
		if !iconSet {
			iconSet = true
			setWindowIcon("stimconv")
		}
		u.drain()
	})
	g.OnResize(func(w, h int) {
		u.registerDrop(w, h)
		u.post(u.relayout)
	})
	u.registerDrop(winW, winH)
	u.rebuild()
	return desktop.Run(g, a)
}

func newUI(g *gogpu.App, a *app.App) *ui {
	return &ui{
		gapp: g, app: a, cfg: config.Default(), preset: presets.Default,
		input: state.NewSignal(""), outDir: state.NewSignal(""), presetName: state.NewSignal(""),
		outName: state.NewSignal(""), outNameOn: state.NewSignal(false), presetSuffix: state.NewSignal(false), subfolder: state.NewSignal(false),
		contentWarn: state.NewSignal(""),
		info:        state.NewSignal("No input selected — browse or drop an audio file onto the window."),
		status:      state.NewSignal("Ready"),
		stats:       state.NewSignal(false), dryRun: state.NewSignal(false), running: state.NewSignal(false),
		optScroll: state.NewSignal[float32](0), logScroll: state.NewSignal[float32](0), aboutScroll: state.NewSignal[float32](0),
		optErr: state.NewSignal(""), rev: state.NewSignal(0),
	}
}

// post runs fn on the UI thread at the next frame. Safe from any goroutine.
func (u *ui) post(fn func()) {
	u.mu.Lock()
	u.queue = append(u.queue, fn)
	u.mu.Unlock()
	if u.gapp != nil {
		u.gapp.RequestRedraw()
	}
}

func (u *ui) drain() {
	u.mu.Lock()
	q := u.queue
	u.queue = nil
	u.mu.Unlock()
	for _, fn := range q {
		fn()
	}
	if len(q) > 0 {
		u.relayout()
	}
}

// relayout schedules a full layout pass. Signal-bound text only repaints and
// containers cache child sizes (widget.LayoutChild), so text whose size
// changes (console rows, status lines) stays at its old size without this.
func (u *ui) relayout() {
	if u.app == nil {
		return
	}
	win := u.app.Window()
	invalidateLayout(win.Root())
	win.Context().Invalidate()
}

func invalidateLayout(w widget.Widget) {
	if w == nil {
		return
	}
	if c, ok := w.(interface{ InvalidateLayoutCache() }); ok {
		c.InvalidateLayoutCache()
	}
	if r, ok := w.(interface{ SetNeedsRedraw(bool) }); ok {
		r.SetNeedsRedraw(true) // function-backed text has no signal to mark it
	}
	if p, ok := w.(interface{ Children() []widget.Widget }); ok {
		for _, ch := range p.Children() {
			invalidateLayout(ch)
		}
	}
}

// rebuild recreates the widget tree, used when the set of visible options
// changes. All values live in u, so nothing is lost but focus.
func (u *ui) rebuild() {
	u.app.SetRoot(u.build())
	if u.gapp != nil {
		u.gapp.RequestRedraw()
	}
}

func (u *ui) registerDrop(w, h int) {
	mgr := u.app.Window().DndManager()
	mgr.UnregisterTarget(u)
	mgr.RegisterTarget(u, geometry.NewRect(0, 0, float32(w), float32(h)))
}

// dnd.DropTarget: an audio file dropped anywhere on the window becomes the input.

func (u *ui) CanAccept(d dnd.DragData) bool                        { return d.Kind == dnd.KindFile }
func (u *ui) DragEnter(dnd.DragData)                               {}
func (u *ui) DragOver(dnd.DragData, geometry.Point) dnd.DropEffect { return dnd.DropCopy }
func (u *ui) DragLeave()                                           {}
func (u *ui) Drop(d dnd.DragData, _ geometry.Point) bool {
	p, ok := d.Payload.(dnd.FilePayload)
	if !ok || len(p.Paths) == 0 {
		return false
	}
	u.post(func() { u.setInputs(p.Paths) })
	return true
}
