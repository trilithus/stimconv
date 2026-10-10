package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/trilithus/stimconv/internal/config"
)

// ReadmeName is the settings report's file name: README.md inside a
// dedicated sub-folder (one input per folder), otherwise <output name>.md,
// since several inputs may share the folder.
func ReadmeName(input, outName string, ownFolder bool) string {
	if ownFolder {
		return "README.md"
	}
	return OutBase(input, outName) + ".md"
}

// readmeInfo is what writeReadme reports besides the settings.
type readmeInfo struct {
	Input    string
	Source   string // format / container / track description
	Preset   string
	Duration float64 // seconds
	Files    []string
	Warnings []string
	Restim   []RestimSetting
	TwoLead  bool // tri-phase position ab: output C stays unconnected
}

// writeReadme writes the report next to the funscripts. It is written for
// whoever receives the funscripts (setup and restim settings first) and
// leaves no local paths in it; how they were made comes last.
func writeReadme(path string, cfg config.Config, info readmeInfo) error {
	var b strings.Builder
	name := filepath.Base(info.Input)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	fmt.Fprintf(&b, "# %s\n\n", base)
	src := fmt.Sprintf("`%s`", mdInline(name))
	if st, err := os.Stat(info.Input); err == nil {
		src += fmt.Sprintf(" (%s, %.1f MB)", fmtDuration(info.Duration), float64(st.Size())/1e6)
	}
	fmt.Fprintf(&b, "restim funscripts for a FOC-Stim, converted from the audio drive signal in %s.\n\n", src)
	fmt.Fprintf(&b, "Made with stimconv %s on %s. The restim settings below were checked against restim %s and FOC-Stim firmware %s; other versions may use different defaults.\n",
		version(), time.Now().Format("2006-01-02"), RestimVersion, FOCStimVersion)

	fmt.Fprintf(&b, "\n## Setup\n\n")
	fmt.Fprintf(&b, "1. Put the funscripts next to the video. restim links them by name: `<video name>.<axis>.funscript`. These are named after `%s`; if the video has another name, rename the part before the axis (for example `%s.volume.funscript` becomes `<video name>.volume.funscript`).\n", mdInline(base), mdInline(base))
	changes := 0
	for _, s := range info.Restim {
		if s.Change {
			changes++
		}
	}
	fmt.Fprintf(&b, "2. Set up restim as in the table below. ")
	if changes > 0 {
		fmt.Fprintf(&b, "**%d settings differ from restim's defaults** and are marked **change**; the rest can stay as they are.\n", changes)
	} else {
		fmt.Fprintf(&b, "Everything matches restim's defaults.\n")
	}
	fmt.Fprintf(&b, "\n| Where | Setting | Expected | restim default | |\n|---|---|---|---|---|\n")
	for _, s := range info.Restim {
		mark := ""
		if s.Change {
			mark = "**change**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", mdCell(s.Where), mdCell(s.Name), mdCell(s.Need), mdCell(s.Default), mark)
	}
	if info.TwoLead {
		fmt.Fprintf(&b, "3. Connect two electrodes, to FOC-Stim outputs A and B, as the original box used one channel for a mono track. Output C gets no current and can stay unconnected. Keep restim's 3-phase position transform off, or it moves current onto C.\n")
	}
	fmt.Fprintf(&b, "\nFunscript kit ranges are read linearly: a script written for 300 – 2000 Hz plays wrong under restim's 500 – 1000 Hz, so set each one exactly.\n")

	if len(info.Files) > 0 {
		fmt.Fprintf(&b, "\n## Files\n\n")
		for _, f := range info.Files {
			fmt.Fprintf(&b, "- `%s`\n", mdInline(filepath.Base(f)))
		}
	}

	fmt.Fprintf(&b, "\n## How these were made\n\n")
	fmt.Fprintf(&b, "For whoever converts the track again; not needed for playback.\n\n")
	fmt.Fprintf(&b, "| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Source | %s |\n", mdCell(info.Source))
	fmt.Fprintf(&b, "| Preset | %s |\n", mdCell(info.Preset))
	if len(info.Warnings) > 0 {
		fmt.Fprintf(&b, "\n### Conversion notes\n\nWhere the source asked for more than FOC-Stim or the chosen settings allow:\n\n")
		for _, w := range info.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	fmt.Fprintf(&b, "\n### stimconv settings\n\n\"(not used)\" marks settings that have no effect with the other choices, for example tri-phase options in a quad-phase conversion.\n\n")
	fmt.Fprintf(&b, "| Setting | Key | Value | Default |\n|---|---|---|---|\n")
	def := config.Default()
	for i := range config.Options {
		o := &config.Options[i]
		v, d := fmtValue(o.Get(&cfg)), fmtValue(o.Get(&def))
		isDef := "yes"
		if v != d {
			isDef = "no (default " + d + ")"
		}
		label := o.Label
		if o.Group == config.GroupRanges {
			label = "Range " + o.Label
		}
		shown := v
		if o.OnlyIf != nil && !o.OnlyIf(cfg) {
			shown += " (not used)"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", mdCell(label), o.Key, mdCell(shown), mdCell(isDef))
	}
	fmt.Fprintf(&b, "\nReproduce with the CLI: save these settings as a JSON config (GUI: Export config…) and run `stimconv cli --config <file> \"%s\"`.\n", mdInline(name))
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func fmtDuration(sec float64) string {
	d := time.Duration(sec * float64(time.Second)).Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// mdInline keeps a name intact inside `code`.
func mdInline(s string) string { return strings.ReplaceAll(s, "`", "'") }

func fmtValue(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprint(x)
	}
}

// mdCell escapes text for a Markdown table cell.
func mdCell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}

// version is the module version or VCS revision stimconv was built from.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown version)"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev, dirty := "", ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if rev == "" {
		return "(development build)"
	}
	return "rev " + rev + dirty
}
