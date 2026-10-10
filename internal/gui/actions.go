package gui

import (
	"context"
	"fmt"
	"github.com/trilithus/stimconv/internal/analysis"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"

	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/pipeline"
	"github.com/trilithus/stimconv/internal/presets"
)

const (
	maxLogLines = 2000
	logFont     = 12
	logLine     = 1.3 // line height multiplier
	logView     = 134 // visible console height in px (box height minus padding)
	logRows     = 300 // console rows kept after wrapping
)

// logf appends to the console. A "warning:" or "error:" prefix (as in
// pipeline output) selects that channel; anything else is info. UI thread only.
func (u *ui) logf(format string, a ...any) {
	for _, l := range strings.Split(strings.TrimRight(fmt.Sprintf(format, a...), "\n"), "\n") {
		u.logLines = append(u.logLines, classify(l))
	}
	if n := len(u.logLines); n > maxLogLines {
		u.logLines = u.logLines[n-maxLogLines:]
	}
	u.relayout()
}

// copyText puts s on the system clipboard and reports it in the console.
func (u *ui) copyText(s, what string) {
	if u.app == nil {
		return
	}
	pp := u.app.PlatformProvider()
	if pp == nil {
		u.logf("error: clipboard unavailable")
		return
	}
	if err := pp.ClipboardWrite(s); err != nil {
		u.logf("error: copy %s: %v", what, err)
		return
	}
	u.status.Set("Copied " + what + " to the clipboard")
	u.relayout()
}

func (u *ui) copyLog() { u.copyText(u.logText(tagSet), "log") }

// logText renders the console with the given channel markers.
func (u *ui) logText(set symbolSet) string {
	lines := make([]string, len(u.logLines))
	for i, e := range u.logLines {
		lines[i] = set.sym[e.lvl] + " " + e.text
	}
	return strings.Join(lines, "\n")
}

func (u *ui) clearLog() {
	u.logLines = nil
	u.relayout()
}

// logWriter forwards pipeline output to the console from the worker goroutine.
type logWriter struct{ u *ui }

func (w logWriter) Write(p []byte) (int, error) {
	s := string(p)
	w.u.post(func() { w.u.logf("%s", s) })
	return len(p), nil
}

// dialog runs a blocking native file dialog off the UI thread and posts the
// chosen path back. Cancelling does nothing.
func (u *ui) dialog(open func() (string, error), done func(string)) {
	go func() {
		p, err := open()
		if err == zenity.ErrCanceled {
			return
		}
		u.post(func() {
			if err != nil {
				u.logf("warning: file dialog unavailable (%v); type or paste the path instead", err)
				return
			}
			done(p)
		})
	}()
}

// File dialog filters. MP1/2/3, WAV and FLAC are decoded natively;
// everything else goes through ffmpeg, which reads these formats.
var (
	audioPatterns = []string{"*.mp3", "*.mp2", "*.mp1", "*.mpa", "*.wav", "*.flac", "*.ogg", "*.oga", "*.opus",
		"*.m4a", "*.aac", "*.wma", "*.aif", "*.aiff", "*.ac3", "*.eac3", "*.dts", "*.mka", "*.wv", "*.ape", "*.caf", "*.au"}
	videoPatterns = []string{"*.mkv", "*.mp4", "*.m4v", "*.mov", "*.avi", "*.webm", "*.wmv", "*.flv", "*.mpg",
		"*.mpeg", "*.ts", "*.m2ts", "*.mts", "*.vob", "*.3gp", "*.ogv"}
)

func (u *ui) browseInput() {
	start := u.input.Get()
	if len(u.inputs) > 0 {
		start = u.inputs[0]
	}
	u.dialog(func() (string, error) {
		paths, err := zenity.SelectFileMultiple(zenity.Title("Select stim audio or video (several files = batch)"), zenity.Filename(start),
			zenity.FileFilters{
				{Name: "Audio & Video", Patterns: append(append([]string{}, audioPatterns...), videoPatterns...), CaseFold: true},
				{Name: "Audio", Patterns: audioPatterns, CaseFold: true},
				{Name: "Video", Patterns: videoPatterns, CaseFold: true},
				{Name: "All files", Patterns: []string{"*"}},
			})
		return strings.Join(paths, "\x00"), err
	}, func(joined string) { u.setInputs(strings.Split(joined, "\x00")) })
}

func (u *ui) browseOutDir() {
	start := u.outDir.Get()
	if start == "" && u.input.Get() != "" {
		start = filepath.Dir(u.input.Get()) + string(filepath.Separator)
	}
	u.dialog(func() (string, error) {
		return zenity.SelectFile(zenity.Title("Output folder"), zenity.Directory(), zenity.Filename(start))
	}, u.outDir.Set)
}

func (u *ui) exportConfig() {
	cfg := u.cfg
	u.dialog(func() (string, error) {
		return zenity.SelectFileSave(zenity.Title("Export config (use with: stimconv cli --config)"),
			zenity.Filename("stimconv-config.json"), zenity.ConfirmOverwrite(),
			zenity.FileFilters{{Name: "JSON", Patterns: []string{"*.json"}}})
	}, func(p string) {
		if err := cfg.Save(p); err != nil {
			u.logf("error: export failed: %v", err)
			return
		}
		u.logf("config written to %s  (stimconv cli --config %q <file>)", p, p)
	})
}

// setInput selects the input file and sniffs its format in the background.
func (u *ui) setInput(p string) {
	p = strings.TrimSpace(p)
	u.selGen++
	if len(u.inputs) > 0 {
		u.inputs = nil
		defer u.rebuild() // leave batch mode
	}
	u.input.Set(p)
	defer u.relayout()
	if len(u.tracks) > 0 {
		u.tracks, u.track = nil, 0
		defer u.rebuild() // drop the track selector of the previous file
	}
	if p == "" {
		return
	}
	u.info.Set("Checking " + filepath.Base(p) + " …")
	u.contentWarn.Set("")
	gen := u.selGen
	go func() {
		msg, tracks := probeInput(p)
		warn := contentWarning(p, 0, tracks)
		u.post(func() {
			if u.selGen != gen {
				return
			}
			u.info.Set(msg)
			u.contentWarn.Set(warn)
			if len(tracks) > 1 {
				u.tracks, u.track = tracks, 0
				u.rebuild()
			}
		})
	}()
}

// setInputs selects one file, or several for batch processing. In a batch
// every file uses its default audio track and gets its own default output
// folder (or they all share the chosen output folder).
func (u *ui) setInputs(paths []string) {
	var list []string
	seen := map[string]bool{}
	for _, p := range paths {
		if p = strings.TrimSpace(p); p != "" && !seen[p] {
			seen[p] = true
			list = append(list, p)
		}
	}
	if len(list) <= 1 {
		u.setInput(strings.Join(list, ""))
		return
	}
	u.selGen++
	gen := u.selGen
	u.inputs = list
	u.tracks, u.track = nil, 0
	u.input.Set(fmt.Sprintf("%d files selected (batch)", len(list)))
	lines := make([]string, len(list))
	for i, p := range list {
		lines[i] = filepath.Base(p) + " — checking …"
	}
	u.info.Set(capLines(lines, 8))
	u.contentWarn.Set("")
	u.rebuild()
	go func() {
		var warns []string
		for i, p := range list {
			msg, tracks := probeInput(p)
			if w := contentWarning(p, 0, tracks); w != "" {
				warns = append(warns, filepath.Base(p)+": "+strings.TrimPrefix(w, "⚠ "))
			}
			line, ws := filepath.Base(p)+" — "+msg, append([]string(nil), warns...)
			u.post(func() {
				if u.selGen != gen {
					return
				}
				lines[i] = line
				u.info.Set(capLines(lines, 8))
				if len(ws) > 0 {
					u.contentWarn.Set(capLines(prefixAll("⚠ ", ws), 6))
				}
			})
		}
	}()
}

// capLines joins at most max lines and summarises the rest.
func capLines(lines []string, max int) string {
	if len(lines) <= max {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n… and %d more", len(lines)-max)
}

func prefixAll(p string, ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = p + s
	}
	return out
}

// selectTrack switches the audio track and re-runs the content check.
func (u *ui) selectTrack(track int) {
	u.track = track
	p, tracks := u.input.Get(), u.tracks
	u.contentWarn.Set("")
	go func() {
		warn := contentWarning(p, track, tracks)
		u.post(func() {
			if u.input.Get() == p && u.track == track {
				u.contentWarn.Set(warn)
			}
		})
	}()
}

// contentWarning runs the stim/music check on the given track when the
// file decodes; it returns "" for drive signals and undecodable files.
func contentWarning(p string, track int, tracks []decode.Track) string {
	if len(tracks) == 0 {
		return ""
	}
	c, err := analysis.CheckFile(p, decode.Options{AudioTrack: track}, pipeline.CheckSeconds)
	if err != nil || c.Kind == analysis.KindStim {
		return ""
	}
	return "⚠ " + strings.ToUpper(c.String()[:1]) + c.String()[1:] + ". The conversion assumes an amplifier drive signal; music or speech will give a meaningless result."
}

// outNameValue is the output name override, "" when it is off.
func (u *ui) outNameValue() string {
	if !u.outNameOn.Get() {
		return ""
	}
	return strings.TrimSpace(u.outName.Get())
}

// outNameErr explains why the output name override can't be used, "" if it can.
func (u *ui) outNameErr() string {
	n := u.outNameValue()
	if n == "" {
		return ""
	}
	if len(u.inputs) > 0 {
		return "an output name applies to a single file; turn it off for a batch"
	}
	if err := pipeline.ValidateOutName(n); err != nil {
		return err.Error()
	}
	return ""
}

// folderPreset is the preset part of the default output folder, "" when the
// suffix is turned off.
func (u *ui) folderPreset() string {
	return pipeline.FolderPreset(presets.FolderName(u.preset), u.presetSuffix.Get())
}

// defaultOutText describes where output goes when no folder is set and how
// the files are named.
func (u *ui) defaultOutText() string {
	if e := u.outNameErr(); e != "" {
		return "⚠ " + strings.ToUpper(e[:1]) + e[1:] + "."
	}
	name := u.folderPreset()
	suffix := ""
	if name != "" {
		suffix = "." + name
	}
	in := strings.TrimSpace(u.input.Get())
	files := ""
	if in != "" && len(u.inputs) == 0 {
		files = " Files: " + pipeline.OutBase(in, u.outNameValue()) + ".<axis>.funscript."
	}
	if strings.TrimSpace(u.outDir.Get()) != "" {
		return strings.TrimSpace(files)
	}
	if len(u.inputs) > 0 {
		return "Default: a folder next to each input file, named <file name without extension>" + suffix
	}
	if in == "" {
		return "Default: a folder next to the input file, named <output name>" + suffix
	}
	return "Default: " + pipeline.DefaultOutDir(in, u.outNameValue(), name) + "." + files
}

// probeInput checks that p can be decoded and lists its audio tracks.
func probeInput(p string) (string, []decode.Track) {
	st, err := os.Stat(p)
	if err != nil {
		return "⚠ " + err.Error(), nil
	}
	info, err := decode.Probe(p)
	if err != nil {
		return "⚠ cannot decode: " + err.Error(), nil
	}
	what := string(info.Format)
	if info.Format == decode.FormatUnknown {
		what = fmt.Sprintf("%s file · %d audio track(s)", info.Container, len(info.Tracks))
	} else if src, _, err := decode.Open(p, decode.Options{}); err != nil {
		return "⚠ cannot decode: " + err.Error(), nil
	} else {
		src.Close()
	}
	return fmt.Sprintf("%s · %.1f MB", what, float64(st.Size())/1e6), info.Tracks
}

// ---- presets ----

func (u *ui) loadPreset(name string) {
	c, err := presets.Load(name)
	if err != nil {
		u.logf("error: preset %s: %v", name, err)
		return
	}
	u.cfg, u.preset = c, name
	if name == presets.Default {
		u.presetName.Set("")
	} else {
		u.presetName.Set(name)
	}
	u.validate()
	u.rebuild()
	u.logf("loaded preset %s", name)
}

func (u *ui) savePreset() {
	name := strings.TrimSpace(u.presetName.Get())
	if name == "" && u.preset != presets.Default {
		name = u.preset
	}
	if err := presets.ValidateName(name); err != nil {
		u.status.Set("Preset not saved")
		u.logf("error: preset name %q: %v", name, err)
		return
	}
	if err := presets.Save(name, u.cfg); err != nil {
		u.logf("error: save preset %q: %v", name, err)
		return
	}
	u.preset = name
	u.rebuild()
	u.logf("saved preset %s", name)
}

func (u *ui) deletePreset() {
	name := u.preset
	if err := presets.Delete(name); err != nil {
		u.logf("error: delete preset %q: %v", name, err)
		return
	}
	u.preset = presets.Default
	u.rebuild()
	u.logf("deleted preset %s", name)
}

// ---- run ----

func (u *ui) startOrCancel() {
	if u.running.Get() {
		if u.cancel != nil {
			u.cancel()
			u.status.Set("Cancelling after the current stage…")
		}
		return
	}
	if e := u.outNameErr(); e != "" {
		u.logf("error: %s", e)
		return
	}
	if !u.logOpen {
		u.logOpen = true
		u.rebuild()
	}
	inputs := u.inputs
	track := 0 // batch files use their default track
	if len(inputs) == 0 {
		inputs, track = []string{u.input.Get()}, u.track
	}
	base := pipeline.Options{
		OutDir:       strings.TrimSpace(u.outDir.Get()),
		OutName:      u.outNameValue(),
		Preset:       presets.FolderName(u.preset),
		PresetSuffix: u.presetSuffix.Get(),
		DryRun:       u.dryRun.Get(),
		Stats:        u.stats.Get(),
		AudioTrack:   track,
	}
	if dup := duplicateTargets(inputs, base.OutDir, u.folderPreset()); len(dup) > 0 {
		u.logf("warning: these files write to the same funscript names, so later ones overwrite earlier ones: %s", strings.Join(dup, ", "))
	}
	cfg := u.cfg
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.running.Set(true)
	n := len(inputs)
	go func() {
		var failed []string
		done := 0
		for i, in := range inputs {
			if ctx.Err() != nil {
				break
			}
			label := filepath.Base(in)
			if n > 1 {
				label = fmt.Sprintf("%d/%d · %s", i+1, n, label)
			}
			u.post(func() {
				u.status.Set("Converting " + label + " …")
				u.logf("— %s", label)
			})
			opts := base
			opts.Input = in
			res, err := pipeline.Convert(ctx, cfg, opts, logWriter{u})
			switch {
			case err == context.Canceled:
			case err != nil:
				failed = append(failed, filepath.Base(in))
				u.post(func() { u.logf("error: %v", err) })
			default:
				done++
				if n == 1 && !opts.DryRun {
					u.post(func() { u.status.Set(fmt.Sprintf("Done — %d files in %s", len(res.Files), res.OutDir)) })
				}
			}
		}
		cancel()
		cancelled := ctx.Err() == context.Canceled && done+len(failed) < n
		u.post(func() {
			u.running.Set(false)
			u.cancel = nil
			switch {
			case cancelled:
				u.status.Set(fmt.Sprintf("Cancelled — %d of %d converted", done, n))
				u.logf("warning: cancelled")
			case len(failed) > 0 && n == 1:
				u.status.Set("Failed")
			case len(failed) > 0:
				u.status.Set(fmt.Sprintf("Done — %d of %d converted, %d failed", done, n, len(failed)))
				u.logf("error: failed: %s", strings.Join(failed, ", "))
			case base.DryRun:
				u.status.Set("Analysis done")
			case n > 1:
				u.status.Set(fmt.Sprintf("Done — all %d files converted", n))
			}
			if n > 1 {
				u.logf("— batch finished: %d converted, %d failed, %d skipped", done, len(failed), n-done-len(failed))
			}
		})
	}()
}

// duplicateTargets lists inputs whose funscripts would land on the same
// paths (same output folder and same name without extension).
func duplicateTargets(inputs []string, outDir, preset string) []string {
	seen := map[string]string{}
	var dup []string
	for _, in := range inputs {
		dir := outDir
		if dir == "" {
			dir = pipeline.DefaultOutDir(in, "", preset)
		}
		key := filepath.Join(dir, strings.TrimSuffix(filepath.Base(in), filepath.Ext(in)))
		if first, ok := seen[key]; ok {
			dup = append(dup, filepath.Base(first)+" & "+filepath.Base(in))
		} else {
			seen[key] = in
		}
	}
	return dup
}
