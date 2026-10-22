package gui

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// License texts of everything linked into the binary (copied from the
// module cache), the fonts embedded by Fyne, and restim, whose
// formulas the mapping follows. Refresh them when dependencies change.
//
//go:embed licenses/*.txt
var licenseFS embed.FS

// credit is one attribution entry.
type credit struct {
	What, Who, License, URL string
	Text                    string // licenses/<Text> holds the full license
}

// moduleCredits maps linked Go modules to their authors and licenses.
var moduleCredits = map[string]credit{
	"fyne.io/fyne/v2":                  {Who: "Fyne.io developers", License: "BSD-3-Clause"},
	"fyne.io/systray":                  {Who: "Fyne.io developers, getlantern", License: "Apache-2.0"},
	"github.com/BurntSushi/toml":       {Who: "TOML authors", License: "MIT"},
	"github.com/anthonynsimon/bild":    {Who: "Anthony Najjar Simon", License: "MIT"},
	"github.com/clipperhouse/uax29/v2": {Who: "Matt Sherman", License: "MIT"},
	"github.com/fsnotify/fsnotify":     {Who: "The Go Authors, fsnotify Authors", License: "BSD-3-Clause"},
	"github.com/fyne-io/image":         {Who: "Fyne.io", License: "BSD-3-Clause"},
	"github.com/fyne-io/oksvg":         {Who: "Steven R Wiley", License: "BSD-3-Clause"},
	"github.com/go-audio/audio":        {Who: "go-audio authors", License: "Apache-2.0"},
	"github.com/go-audio/riff":         {Who: "go-audio authors", License: "Apache-2.0"},
	"github.com/go-audio/wav":          {Who: "go-audio authors", License: "Apache-2.0"},
	"github.com/go-gl/gl":              {Who: "Eric Woroshow", License: "MIT"},
	"github.com/go-gl/glfw/v3.4/glfw":  {Who: "The glfw3-go Authors, Marcus Geelnard, Camilla Löwy (GLFW)", License: "BSD-3-Clause, zlib (GLFW)"},
	"github.com/go-text/render":        {Who: "go-text authors", License: "Unlicense or BSD-3-Clause"},
	"github.com/go-text/typesetting":   {Who: "go-text authors", License: "Unlicense or BSD-3-Clause"},
	"github.com/godbus/dbus/v5":        {Who: "Georg Reinke, Google", License: "BSD-2-Clause"},
	"github.com/hajimehoshi/go-mp3":    {Who: "Hajime Hoshi", License: "Apache-2.0"},
	"github.com/icza/bitio":            {Who: "Andras Belicza", License: "Apache-2.0"},
	"github.com/jeandeaual/go-locale":  {Who: "Alexis Jeandeau", License: "MIT"},
	"github.com/jsummers/gobmp":        {Who: "Jason Summers", License: "MIT"},
	"github.com/mattn/go-runewidth":    {Who: "Yasuhiro Matsumoto", License: "MIT"},
	"github.com/mewkiz/flac":           {Who: "mewkiz", License: "Unlicense (public domain)"},
	"github.com/mewkiz/pkg":            {Who: "mewkiz", License: "Unlicense (public domain)"},
	"github.com/mewpkg/term":           {Who: "mewpkg", License: "Unlicense (public domain)"},
	"github.com/ncruces/zenity":        {Who: "Nuno Cruces", License: "MIT"},
	"github.com/nfnt/resize":           {Who: "Jan Schlicht", License: "ISC"},
	"github.com/nicksnyder/go-i18n/v2": {Who: "Nick Snyder", License: "MIT"},
	"github.com/rymdport/portal":       {Who: "Jacob Alzén", License: "Apache-2.0"},
	"github.com/srwiley/oksvg":         {Who: "Steven R Wiley", License: "BSD-3-Clause"},
	"github.com/srwiley/rasterx":       {Who: "Steven R Wiley", License: "BSD-3-Clause"},
	"github.com/yuin/goldmark":         {Who: "Yusuke Inuzuka", License: "MIT"},
	"golang.org/x/image":               {Who: "The Go Authors", License: "BSD-3-Clause"},
	"golang.org/x/net":                 {Who: "The Go Authors", License: "BSD-3-Clause"},
	"golang.org/x/sys":                 {Who: "The Go Authors", License: "BSD-3-Clause"},
	"golang.org/x/text":                {Who: "The Go Authors", License: "BSD-3-Clause"},
}

var (
	reHref = regexp.MustCompile(`href="([^"]+)"`)
	reText = regexp.MustCompile(`>([^<]+)</a>`)
)

// iconCredits reads the Flaticon attribution snippets next to the icons.
func iconCredits(icons fs.FS) []credit {
	if icons == nil {
		return nil
	}
	names, _ := fs.Glob(icons, "*.txt")
	var out []credit
	for _, n := range names {
		b, err := fs.ReadFile(icons, n)
		if err != nil {
			continue
		}
		c := credit{What: strings.TrimSuffix(n, ".txt") + " icon", License: "Flaticon license (attribution required)"}
		if m := reText.FindSubmatch(b); m != nil {
			c.Who = string(m[1])
		} else {
			c.Who = strings.TrimSpace(string(b))
		}
		if m := reHref.FindSubmatch(b); m != nil {
			c.URL = string(m[1])
		}
		out = append(out, c)
	}
	return out
}

// credits lists every attribution, with the linked modules taken from the
// binary's build info so the list follows the actual dependencies.
func credits(icons fs.FS) []credit {
	out := iconCredits(icons)
	out = append(out,
		credit{What: "Noto Sans typeface (embedded by Fyne)", Who: "Google", License: "SIL Open Font License 1.1", URL: "https://fonts.google.com/noto", Text: "font_noto.txt"},
		credit{What: "Inter Symbols typeface (embedded by Fyne)", Who: "The Inter Project Authors", License: "SIL Open Font License 1.1", URL: "https://github.com/rsms/inter", Text: "font_inter.txt"},
		credit{What: "DejaVu Sans Mono Powerline typeface (embedded by Fyne)", Who: "Bitstream, Tavmjong Bah, DejaVu authors", License: "Bitstream Vera / Arev font license", URL: "https://dejavu-fonts.github.io", Text: "font_dejavu.txt"},
		credit{What: "EmojiOne Color font (embedded by Fyne)", Who: "Adobe Systems Incorporated, artwork by EmojiOne", License: "MIT (font), CC BY 4.0 (artwork)", URL: "https://github.com/adobe-fonts/emojione-color", Text: "font_emojione.txt"},
		credit{What: "Material Design icons (embedded by Fyne)", Who: "Google", License: "Apache-2.0", URL: "https://github.com/google/material-design-icons"},
		credit{What: "restim (funscript axis model, formulas used by the mapping)", Who: "diglet48", License: "MIT", URL: "https://github.com/diglet48/restim", Text: "restim.txt"},
		credit{What: "Go standard library", Who: "The Go Authors", License: "BSD-3-Clause", URL: "https://go.dev", Text: "go.txt"},
	)
	versions := map[string]string{}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			versions[d.Path] = d.Version
		}
	}
	var mods []credit
	for p, c := range moduleCredits {
		v, linked := versions[p]
		if len(versions) > 0 && !linked {
			continue // no longer a dependency
		}
		c.What = strings.TrimSpace(p + " " + v)
		c.URL = "https://" + p
		c.Text = strings.ReplaceAll(p, "/", "_") + ".txt"
		mods = append(mods, c)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].What < mods[j].What })
	out = append(out, mods...)
	out = append(out, credit{What: "FFmpeg (separate program in libs/ffmpeg, used to decode MP3 and other formats; see its NOTICE.txt)", Who: "the FFmpeg developers, build by BtbN/FFmpeg-Builds", License: "LGPL v3+ (bundled build)", URL: "https://ffmpeg.org"})
	return out
}

// aboutText is the plain-text About content: summary, then full licenses.
func aboutText(icons fs.FS) string {
	var b strings.Builder
	b.WriteString("stimconv — converts audio stim tracks into restim funscripts for FOC-Stim.\n\n")
	b.WriteString("Third-party components and attributions:\n\n")
	seen := map[string]bool{}
	var texts []string
	for _, c := range credits(icons) {
		fmt.Fprintf(&b, "• %s\n   %s — %s", c.What, c.Who, c.License)
		if c.URL != "" {
			fmt.Fprintf(&b, "\n   %s", c.URL)
		}
		b.WriteString("\n\n")
		if c.Text != "" && !seen[c.Text] {
			seen[c.Text] = true
			texts = append(texts, c.Text)
		}
	}
	// identical texts (e.g. Apache-2.0 for several modules) are shown once
	if n := ffmpegNotice(); n != "" {
		fmt.Fprintf(&b, "——— Bundled FFmpeg (libs/ffmpeg/NOTICE.txt) ———\n\n%s\n", strings.TrimSpace(n))
	}
	b.WriteString("Full license texts follow.\n")
	var order []string
	users := map[string][]string{}
	for _, t := range texts {
		body, err := licenseFS.ReadFile(path.Join("licenses", t))
		if err != nil {
			continue
		}
		k := strings.TrimSpace(string(body))
		if _, ok := users[k]; !ok {
			order = append(order, k)
		}
		users[k] = append(users[k], strings.TrimSuffix(t, ".txt"))
	}
	for _, k := range order {
		fmt.Fprintf(&b, "\n——— %s ———\n\n%s\n", strings.Join(users[k], ", "), k)
	}
	return b.String()
}

// showAbout swaps the window to the About page.
func (u *ui) showAbout() {
	u.about = true
	u.rebuild()
}

func (u *ui) aboutPage() fyne.CanvasObject {
	txt := aboutText(u.icons)
	body := widget.NewLabel(txt)
	body.Wrapping = fyne.TextWrapWord
	back := widget.NewButton("Back", func() { u.post(func() { u.about = false; u.rebuild() }) })
	back.Importance = widget.HighImportance
	return container.NewBorder(
		heading("About stimconv"),
		container.NewHBox(layout.NewSpacer(), widget.NewButton("Copy", func() { u.copyText(txt, "attributions") }), back),
		nil, nil,
		vscroll(body),
	)
}

// icon returns the UI icon name from u.icons, or fallback when it is missing.
func (u *ui) icon(name string, fallback fyne.Resource) fyne.Resource {
	if u.icons != nil {
		if b, err := fs.ReadFile(u.icons, name); err == nil {
			return fyne.NewStaticResource(name, b)
		}
	}
	return fallback
}

// ffmpegNotice returns libs/ffmpeg/NOTICE.txt next to the executable (or one
// level up, matching decode's ffmpeg search), if a bundled build is present.
func ffmpegNotice() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	for _, d := range []string{filepath.Dir(exe), filepath.Dir(filepath.Dir(exe))} {
		if b, err := os.ReadFile(filepath.Join(d, "libs", "ffmpeg", "NOTICE.txt")); err == nil {
			return string(b)
		}
	}
	return ""
}
