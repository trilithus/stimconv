package gui

import (
	"context"
	"errors"
	"fmt"
	"github.com/trilithus/stimconv/internal/analysis"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"

	"github.com/trilithus/stimconv/internal/config"
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

// importReport loads the settings block of a settings report (.md) as the
// current options. They become a preset only when saved.
func (u *ui) importReport() {
	u.dialog(func() (string, error) {
		return zenity.SelectFile(zenity.Title("Import settings from a stimconv report"),
			zenity.FileFilters{{Name: "stimconv report", Patterns: []string{"*.md"}}, {Name: "All files", Patterns: []string{"*"}}})
	}, func(p string) {
		data, err := os.ReadFile(p)
		if err == nil {
			var e config.Embedded
			if e, err = config.DecodePEM(data); err == nil {
				u.applyImported(e, filepath.Base(p))
				return
			}
		}
		u.logf("error: import %s: %v", filepath.Base(p), err)
	})
}

// applyImported makes imported settings the current options. The preset
// name field suggests the original preset's name unless that is taken by a
// built-in, so Save stores them under it.
func (u *ui) applyImported(e config.Embedded, from string) {
	u.cfg = e.Config
	name := ""
	if presets.ValidateName(e.Preset) == nil {
		name = e.Preset
	}
	u.presetName.Set(name)
	u.rebuild()
	u.logf("imported settings from %s (preset %s, made with stimconv %s); Save stores them as a preset", from, e.Preset, e.Version)
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
	u.clearGuidance()
	gen := u.selGen
	go func() {
		msg, tracks := probeInput(p)
		u.post(func() {
			if u.selGen != gen {
				return
			}
			u.info.Set(msg)
			if len(tracks) > 1 {
				u.tracks, u.track = tracks, 0
				u.rebuild()
			}
			u.checkContent(p, 0, tracks)
		})
	}()
}

// clearGuidance drops the content and wiring hints and stops a running
// wiring analysis.
func (u *ui) clearGuidance() {
	u.contentWarn.Set("")
	u.guidance.Set("")
	if u.wiringCancel != nil {
		u.wiringCancel()
		u.wiringCancel = nil
	}
}

// checkContent runs the quick stim/music check on a track and, for a drive
// signal, the whole-file wiring analysis in the background, showing both
// under the file name. A newer selection discards (and stops) them.
func (u *ui) checkContent(p string, track int, tracks []decode.Track) {
	u.clearGuidance()
	if len(tracks) == 0 {
		return // not decodable; probeInput already says so
	}
	u.selGen++
	gen := u.selGen
	ctx, cancel := context.WithCancel(context.Background())
	u.wiringCancel = cancel
	u.guidance.Set("Checking the content …")
	go func() {
		stim, warn := contentCheck(p, track)
		u.post(func() {
			if u.selGen != gen {
				return
			}
			u.contentWarn.Set(warn)
			if !stim {
				u.guidance.Set("")
				return
			}
			u.guidance.Set("Looks like a stim drive signal. Checking whether it was made for tri-phase or quad-phase (whole file) …")
		})
		if !stim {
			return
		}
		w, err := wiringCheck(ctx, p, track)
		u.post(func() {
			if u.selGen != gen || ctx.Err() != nil {
				return
			}
			if err != nil {
				u.guidance.Set("Looks like a stim drive signal. (Wiring check failed: " + err.Error() + ")")
				return
			}
			u.guidance.Set("Looks like a stim drive signal. " + wiringGuidance(w))
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
	u.clearGuidance()
	u.rebuild()
	go func() {
		var warns []string
		for i, p := range list {
			msg, tracks := probeInput(p)
			if _, w := contentCheck(p, 0); len(tracks) > 0 && w != "" {
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

// selectTrack switches the audio track and re-runs the content checks.
func (u *ui) selectTrack(track int) {
	u.track = track
	u.checkContent(u.input.Get(), track, u.tracks)
}

// contentCheck runs the quick stim/music check on a track. stim is true for
// a drive signal; warn explains otherwise. An undecodable file gives
// neither (probeInput reports it).
func contentCheck(p string, track int) (stim bool, warn string) {
	c, err := analysis.CheckFile(p, decode.Options{AudioTrack: track}, pipeline.CheckSeconds)
	if err != nil {
		return false, ""
	}
	if c.Kind == analysis.KindStim {
		return true, ""
	}
	return false, "⚠ " + strings.ToUpper(c.String()[:1]) + c.String()[1:] + ". The conversion assumes an amplifier drive signal; music or speech will give a meaningless result."
}

// wiringCheck runs the A/B wiring analysis over the whole track (audio as
// is, no circuit model). ctx stops it early.
func wiringCheck(ctx context.Context, p string, track int) (analysis.Wiring, error) {
	src, _, err := decode.Open(p, decode.Options{AudioTrack: track})
	if err != nil {
		return analysis.Wiring{}, err
	}
	defer src.Close()
	raw, err := analysis.Measure(ctxSource{src, ctx}, config.Circuit{})
	if err != nil {
		return analysis.Wiring{}, err
	}
	return analysis.AnalyseWiring(raw), nil
}

// ctxSource stops a decode.Source when ctx is done.
type ctxSource struct {
	decode.Source
	ctx context.Context
}

func (s ctxSource) Read(dst []float32) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	return s.Source.Read(dst)
}

// wiringGuidance turns a wiring verdict into a hint about which built-in
// presets fit.
func wiringGuidance(w analysis.Wiring) string {
	pct := func(f float64) string { return fmt.Sprintf("%.0f%%", 100*f) }
	switch w.Verdict {
	case analysis.WiringThree:
		return "Likely made for tri-phase: the phase between A and B carries position (" + pct(w.Phased) + " of the track), which only a shared common electrode turns into a sensation. Use a tri-phase preset, e.g. [tri-original]."
	case analysis.WiringFour:
		return "Likely made for quad-phase (two separate pairs): A and B carry separate content (" + pct(w.EnvDiff+w.OneSided) + " of the track). Use a quad-phase preset, e.g. [quad-original]."
	case analysis.WiringMono:
		if w.SingleChannel() {
			return "A mono track: A and B carry the same signal (" + pct(w.Mono) + " of the track). The original box played these on one channel with two electrodes. Use [mono-original] and connect only FOC-Stim outputs A and B."
		}
		return "Tri-phase and quad-phase both fit: A and B carry the same signal (" + pct(w.Mono) + " of the track)."
	case analysis.WiringDetuned:
		return "Tri-phase and quad-phase both fit: A and B use different carriers (" + pct(w.Detuned) + " of the track); tri-phase plays them as a beat, quad-phase as two separate sensations."
	case analysis.WiringMixed:
		return "Wiring unclear: no dominant A/B pattern; either tri-phase or quad-phase may fit."
	}
	return "Too little signal to tell whether it was made for tri-phase or quad-phase."
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
	stale := " Funscripts from earlier runs with other options (e.g. e1-e4 after switching to tri-phase) are removed."
	if strings.TrimSpace(u.outDir.Get()) != "" {
		return strings.TrimSpace(files + stale)
	}
	sub := u.subfolder.Get()
	switch {
	case !sub && (len(u.inputs) > 0 || in == ""):
		return "Default: next to each input file." + stale
	case len(u.inputs) > 0:
		return "Default: a folder next to each input file, named <file name without extension>" + suffix + "." + stale
	case in == "":
		return "Default: a folder next to the input file, named <output name>" + suffix + "." + stale
	}
	return "Default: " + pipeline.DefaultOutDir(in, u.outNameValue(), name, sub) + "." + files + stale
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
	if _, builtin := presets.Lookup(name); builtin {
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
	if _, builtin := presets.Lookup(u.preset); name == "" && !builtin {
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
		Subfolder:    u.subfolder.Get(),
		PresetSuffix: u.presetSuffix.Get(),
		DryRun:       u.dryRun.Get(),
		Stats:        u.stats.Get(),
		AudioTrack:   track,
	}
	if dup := duplicateTargets(inputs, base.OutDir, u.folderPreset(), base.Subfolder); len(dup) > 0 {
		u.logf("warning: these files write to the same funscript names, so later ones overwrite earlier ones: %s", strings.Join(dup, ", "))
	}
	cfg := u.cfg
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.running.Set(true)
	n := len(inputs)
	ask := u.askOverwrite
	go func() {
		var failed []string
		done, skipped := 0, 0
		overwriteAll := false
		base.Confirm = func(c []pipeline.Conflict) bool {
			if overwriteAll {
				return true
			}
			ok, all := ask(c, n > 1)
			overwriteAll = all
			return ok || all
		}
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
			case errors.Is(err, pipeline.ErrDeclined):
				skipped++
				u.post(func() { u.logf("warning: skipped %s: existing files kept", filepath.Base(in)) })
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
		cancelled := ctx.Err() == context.Canceled && done+len(failed)+skipped < n
		u.post(func() {
			u.running.Set(false)
			u.cancel = nil
			switch {
			case cancelled:
				u.status.Set(fmt.Sprintf("Cancelled — %d of %d converted", done, n))
				u.logf("warning: cancelled")
			case len(failed) > 0 && n == 1:
				u.status.Set("Failed")
			case skipped > 0 && n == 1:
				u.status.Set("Skipped — existing files kept")
			case len(failed) > 0:
				u.status.Set(fmt.Sprintf("Done — %d of %d converted, %d failed", done, n, len(failed)))
				u.logf("error: failed: %s", strings.Join(failed, ", "))
			case base.DryRun:
				u.status.Set("Analysis done")
			case n > 1 && skipped > 0:
				u.status.Set(fmt.Sprintf("Done — %d of %d converted, %d skipped", done, n, skipped))
			case n > 1:
				u.status.Set(fmt.Sprintf("Done — all %d files converted", n))
			}
			if n > 1 {
				u.logf("— batch finished: %d converted, %d failed, %d skipped", done, len(failed), n-done-len(failed))
			}
		})
	}()
}

// askOverwrite shows a native confirmation dialog listing the existing files
// a conversion would overwrite or delete. In a batch it also offers
// "Overwrite all". A dialog that cannot be shown counts as "skip".
func askOverwrite(c []pipeline.Conflict, batch bool) (ok, all bool) {
	const maxList = 12
	lines := make([]string, 0, maxList+1)
	for i, f := range c {
		if i == maxList {
			lines = append(lines, fmt.Sprintf("… and %d more", len(c)-maxList))
			break
		}
		lines = append(lines, "  "+f.String())
	}
	text := fmt.Sprintf("These existing files in %s would be overwritten or deleted:\n\n%s\n\nDeleted files are funscripts a previous run with other options wrote.",
		filepath.Dir(c[0].Path), strings.Join(lines, "\n"))
	opts := []zenity.Option{zenity.Title("Overwrite existing files?"), zenity.Icon(zenity.WarningIcon),
		zenity.OKLabel("Overwrite"), zenity.CancelLabel("Skip"), zenity.DefaultCancel()}
	if batch {
		opts = append(opts, zenity.ExtraButton("Overwrite all"))
	}
	switch err := zenity.Question(text, opts...); err {
	case nil:
		return true, false
	case zenity.ErrExtraButton:
		return true, true
	}
	return false, false
}

// duplicateTargets lists inputs whose funscripts would land on the same
// paths (same output folder and same name without extension).
func duplicateTargets(inputs []string, outDir, preset string, subfolder bool) []string {
	seen := map[string]string{}
	var dup []string
	for _, in := range inputs {
		dir := outDir
		if dir == "" {
			dir = pipeline.DefaultOutDir(in, "", preset, subfolder)
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
