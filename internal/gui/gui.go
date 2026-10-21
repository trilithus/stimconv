// Package gui is the graphical front end: one window laid out as
// input → options → output, built with Fyne.
package gui

import (
	"io/fs"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/pipeline"
	"github.com/trilithus/stimconv/internal/presets"
)

const winW, winH = 960, 860

// ui holds all window state. Widgets and values are only touched on the UI
// thread; other goroutines hand work over with post.
type ui struct {
	win   fyne.Window // nil in tests that only build the content
	live  bool        // post schedules drain via fyne.Do (false in tests, which drain themselves)
	icons fs.FS       // icon PNGs and their attribution .txt files

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

	input, outDir, presetName *value[string]
	outName                   *value[string] // output name override, used when outNameOn
	outNameOn                 *value[bool]
	subfolder                 *value[bool] // default output goes to a folder next to the input
	presetSuffix              *value[bool] // that folder ends in .<preset>
	info, status              *value[string]
	contentWarn               *value[string] // stim/music check of the input
	guidance                  *value[string] // stim and wiring hint for the input
	wiringCancel              func()         // stops the running wiring check
	stats, dryRun, running    *value[bool]
	optErr                    *value[string]
	logLines                  []logEntry
	logRev                    int // bumped on every console change
	cancel                    func()

	// refreshers copy state into the widgets of the current tree; build
	// fills them and relayout runs them.
	refreshers  []func()
	refreshing  bool
	optScroll   *container.Scroll // kept so a rebuild restores the position
	split       *container.Split  // options above, output below
	splitOffset float64
	root        fyne.CanvasObject

	// askOverwrite asks whether existing files may be overwritten or
	// deleted (all = also for the rest of a batch); runs off the UI thread.
	askOverwrite func(c []pipeline.Conflict, batch bool) (ok, all bool)

	mu    sync.Mutex
	queue []func()
}

// Run opens the window and blocks until it is closed. iconPNG is the window
// and application icon.
func Run(icons fs.FS, iconPNG []byte) error {
	a := app.NewWithID("io.github.trilithus.stimconv")
	a.Settings().SetTheme(appTheme())
	if iconPNG != nil {
		a.SetIcon(fyne.NewStaticResource("icon.png", iconPNG))
	}
	w := a.NewWindow("stimconv")
	u := newUI(w)
	u.live = true
	u.icons = icons
	w.Resize(fyne.NewSize(winW, winH))
	w.SetMainMenu(u.mainMenu())
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		var paths []string
		for _, p := range uris {
			if p.Scheme() == storage.NewFileURI("/").Scheme() {
				paths = append(paths, p.Path())
			}
		}
		if len(paths) > 0 {
			u.setInputs(paths)
		}
	})
	u.rebuild()
	w.ShowAndRun()
	return nil
}

func newUI(w fyne.Window) *ui {
	u := &ui{win: w, cfg: config.Default(), preset: presets.Default, askOverwrite: askOverwrite}
	u.input, u.outDir, u.presetName, u.outName = newValue(u, ""), newValue(u, ""), newValue(u, ""), newValue(u, "")
	u.outNameOn, u.subfolder, u.presetSuffix = newValue(u, false), newValue(u, false), newValue(u, false)
	u.contentWarn, u.guidance, u.optErr = newValue(u, ""), newValue(u, ""), newValue(u, "")
	u.info = newValue(u, "No input selected — browse or drop an audio file onto the window.")
	u.status = newValue(u, "Ready")
	u.stats, u.dryRun, u.running = newValue(u, false), newValue(u, false), newValue(u, false)
	return u
}

// post runs fn on the UI thread. Safe from any goroutine.
func (u *ui) post(fn func()) {
	u.mu.Lock()
	u.queue = append(u.queue, fn)
	u.mu.Unlock()
	if u.live {
		fyne.Do(u.drain)
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
}

// relayout copies the current state into the widgets.
func (u *ui) relayout() {
	if u.refreshing {
		return // a widget callback fired while being updated
	}
	u.refreshing = true
	defer func() { u.refreshing = false }()
	for _, r := range u.refreshers {
		r()
	}
}

// rebuild recreates the widget tree, used when the set of visible options
// changes. All values live in u, so nothing is lost but focus.
func (u *ui) rebuild() {
	var off fyne.Position
	if u.optScroll != nil {
		off = u.optScroll.Offset
	}
	u.root = u.build()
	if u.optScroll != nil {
		u.optScroll.Offset = off
	}
	if u.win != nil {
		u.win.SetContent(u.root)
	}
}

// mainMenu is the window's menu bar. Fyne adds Quit to the first menu.
func (u *ui) mainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(
		fyne.NewMenu("File",
			fyne.NewMenuItem("Open audio or video…", u.browseInput),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Export config…", u.exportConfig),
			fyne.NewMenuItem("Import settings from report…", u.importReport),
			fyne.NewMenuItem("Open Preset Folder", u.openPresetFolder),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About stimconv…", u.showAbout),
		),
	)
}
