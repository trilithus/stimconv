// Package pipeline runs one conversion (decode → measure → derive → map →
// write) so the CLI and the GUI share exactly the same code path.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/trilithus/stimconv/internal/analysis"
	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/funscript"
	"github.com/trilithus/stimconv/internal/mapping"
)

// Options are the per-run settings that are not part of config.Config.
type Options struct {
	Input  string
	OutDir string // default: DefaultOutDir(Input, OutName, preset, Subfolder)
	// OutName replaces the input name (without extension) in the output
	// names, e.g. "PEP11" for PEP11.fr.mp3 -> PEP11.alpha.funscript.
	// "" uses the input name; see ValidateOutName.
	OutName string
	Preset  string // added to the sub-folder with PresetSuffix; "" = "default"
	// Subfolder writes into a dedicated folder next to the input, named
	// after the output name, instead of next to the input itself.
	Subfolder bool
	// PresetSuffix adds the preset to that sub-folder's name:
	// "track.tri-original" instead of "track".
	PresetSuffix bool
	DumpCSV      string // per-hop features CSV, empty = off
	DryRun       bool   // analyse only, write no funscripts
	Stats        bool   // print per-axis statistics
	NativeMP3    bool   // decode Layer III with go-mp3 instead of ffmpeg
	// OverwriteFiles allows replacing existing output files and deleting
	// stale funscripts (see Conflicts). Without it Convert asks Confirm, or
	// fails with an *ExistingFilesError when Confirm is nil.
	OverwriteFiles bool
	// Confirm is asked, before anything is written, whether the listed
	// existing files may be overwritten or deleted. false skips the input
	// with ErrDeclined.
	Confirm func([]Conflict) bool
	// AudioTrack picks an audio stream in video/container files, numbered
	// from 1 (decode.Track.Index+1); 0 uses the default track.
	AudioTrack int
}

// DefaultOutDir is the output folder used when none is given: the input's
// own folder, or with subfolder a folder next to the input named after the
// output name (see OutBase) and the preset, e.g. "track.tri-original", or
// just "track" when preset is "". preset must already be a safe folder name.
func DefaultOutDir(input, outName, preset string, subfolder bool) string {
	if !subfolder {
		return filepath.Dir(input)
	}
	name := OutBase(input, outName)
	if preset != "" {
		name += "." + preset
	}
	return filepath.Join(filepath.Dir(input), name)
}

// FolderPreset is the preset part of the default output folder: "" without
// the suffix, otherwise preset ("default" when empty).
func FolderPreset(preset string, suffix bool) string {
	switch {
	case !suffix:
		return ""
	case preset == "":
		return "default"
	}
	return preset
}

// OutBase is the base of the output names: outName when set, otherwise the
// input file name without its extension.
func OutBase(input, outName string) string {
	if n := strings.TrimSpace(outName); n != "" {
		return n
	}
	return strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
}

// ValidateOutName checks an output name override: one file name component
// without characters Windows refuses. "" (no override) is valid.
func ValidateOutName(name string) error {
	n := strings.TrimSpace(name)
	switch {
	case n == "":
		return nil
	case n == "." || n == "..":
		return fmt.Errorf("output name %q is not a file name", name)
	case strings.ContainsAny(n, `<>:"/\|?*`):
		return fmt.Errorf(`output name %q contains one of < > : " / \ | ? *`, name)
	case strings.HasSuffix(n, "."):
		return fmt.Errorf("output name %q ends with a dot", name)
	}
	for _, r := range n {
		if r < 0x20 {
			return fmt.Errorf("output name %q contains a control character", name)
		}
	}
	return nil
}

// CheckSeconds is how much of the input the content check looks at.
const CheckSeconds = 90

// Result summarises a finished run.
type Result struct {
	OutDir string
	Files  []string // funscripts written (empty on a dry run)
	Hints  string   // restim settings the output depends on
}

// Convert runs the full conversion and writes progress to log. ctx is
// checked between stages.
func Convert(ctx context.Context, cfg config.Config, o Options, log io.Writer) (Result, error) {
	var out Result
	if err := cfg.Validate(); err != nil {
		return out, err
	}
	if err := ValidateOutName(o.OutName); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	in := o.Input
	start := time.Now()
	// quick content check of the first 90 s before the full pass
	if c, err := analysis.CheckFile(in, decode.Options{NativeMP3: o.NativeMP3, AudioTrack: o.AudioTrack}, CheckSeconds); err == nil && c.Kind != analysis.KindStim {
		fmt.Fprintln(log, "warning:", c)
	}
	src, format, err := decode.Open(in, decode.Options{NativeMP3: o.NativeMP3, AudioTrack: o.AudioTrack})
	if err != nil {
		return out, err
	}
	what := string(format)
	if format == decode.FormatUnknown { // container decoded by ffmpeg
		if info, err := decode.Probe(in); err == nil && len(info.Tracks) > 0 {
			i := o.AudioTrack - 1
			if i < 0 {
				i = decode.DefaultTrack(info.Tracks)
			}
			if i < len(info.Tracks) {
				t := info.Tracks[i]
				what = fmt.Sprintf("%s, audio track %d of %d (%s)", info.Container, i+1, len(info.Tracks), t)
				if t.Channels > 2 {
					fmt.Fprintf(log, "warning: track %d has %d channels; using the first two as L (A) and R (B)\n", i+1, t.Channels)
				}
			}
		}
	}
	raw, err := analysis.Measure(src, cfg.Circuit)
	if cerr := src.Close(); err == nil {
		err = cerr // e.g. ffmpeg exited with an error
	}
	if err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	feat := analysis.Derive(raw, cfg.ModSplitHz, cfg.SilenceDB)
	if o.DumpCSV != "" {
		if err := feat.WriteCSV(o.DumpCSV); err != nil {
			return out, err
		}
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	res, err := mapping.Map(feat, cfg)
	if err != nil {
		return out, err
	}
	if n := raw.Overload[0] + raw.Overload[1]; n > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%d samples far beyond full scale (first at %.2f s), likely a corrupt frame in the file; clipped to ±1 as the amplifier would",
			n, raw.OverloadAt))
	}
	wiring := analysis.AnalyseWiring(raw)
	if t := wiring.Verdict.Topology(); t != "" && t != cfg.Topology {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"the A/B content suggests topology %s, but %s is selected (%.0f%% phase-coded, %.0f%% separate A/B content)",
			t, cfg.Topology, 100*wiring.Phased, 100*(wiring.EnvDiff+wiring.OneSided)))
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	fmt.Fprintf(log, "%s: %s, %d Hz, %.1f s, analysed in %.1fs (topology %s)\n",
		filepath.Base(in), what, raw.SampleRate, raw.Duration, time.Since(start).Seconds(), cfg.Topology)
	if o.Stats || o.DryRun {
		fmt.Fprintln(log, wiring)
	}

	dir := o.OutDir
	if dir == "" {
		dir = DefaultOutDir(in, o.OutName, FolderPreset(o.Preset, o.PresetSuffix), o.Subfolder)
	}
	out.OutDir = dir
	if !o.DryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return out, err
		}
	}
	base := OutBase(in, o.OutName)
	readme := filepath.Join(dir, ReadmeName(in, o.OutName, o.OutDir == "" && o.Subfolder))
	wroteAxis := map[string]bool{}
	for _, a := range res.Axes {
		wroteAxis[a.Name] = true
	}
	if !o.DryRun && !o.OverwriteFiles {
		if c := conflicts(dir, base, readme, res.Axes, wroteAxis); len(c) > 0 {
			if o.Confirm == nil {
				return out, &ExistingFilesError{Conflicts: c}
			}
			if !o.Confirm(c) {
				return out, ErrDeclined
			}
		}
	}
	var written []writtenAxis
	for _, a := range res.Axes {
		acts := a.Actions(cfg.Epsilon)
		path := filepath.Join(dir, base+"."+a.Name+".funscript")
		if !o.DryRun {
			if err := funscript.Write(path, acts); err != nil {
				return out, err
			}
			out.Files = append(out.Files, path)
			// validate what is on disk, not what we meant to write
			s, err := funscript.Read(path)
			if err != nil {
				return out, err
			}
			acts = s.Actions
		}
		written = append(written, writtenAxis{Name: a.Name, Actions: acts})
		if o.Stats {
			printStats(log, a, len(acts))
		}
	}
	if o.Stats {
		var modes []string
		for k, v := range res.ModeTime {
			if v > 0 {
				modes = append(modes, fmt.Sprintf("%s %.0f%%", k, 100*v))
			}
		}
		sort.Strings(modes)
		fmt.Fprintf(log, "rendering modes: %s\n", strings.Join(modes, ", "))
		if cfg.Topology == "dual" {
			fmt.Fprintf(log, "A/B multiplexed: %.0f%% of the time (overlap %s)\n", 100*res.Multiplexed, cfg.Overlap)
		}
	}
	issues := validate(raw, cfg.SilenceDB, written)
	for _, v := range issues {
		res.Warnings = append(res.Warnings, "validation failed: "+v)
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(log, "warning:", w)
	}
	if len(issues) == 0 {
		fmt.Fprintln(log, "validation: output matches the input's activity, files are well-formed")
	}
	out.Hints = RestimHints(cfg, res.Axes)
	if !o.DryRun {
		removed, err := removeStale(dir, base, wroteAxis)
		for _, p := range removed {
			fmt.Fprintf(log, "removed %s (not part of this output; a previous run with other options wrote it)\n", filepath.Base(p))
		}
		if err != nil {
			fmt.Fprintln(log, "warning:", err)
		}
		preset := o.Preset
		if preset == "" {
			preset = "default"
		}
		if err := writeReadme(readme, cfg, readmeInfo{
			Input: in, Source: what, Preset: preset, Duration: raw.Duration, Files: out.Files, Warnings: res.Warnings, Restim: RestimSettings(cfg, res.Axes),
		}); err != nil {
			return out, err
		}
		fmt.Fprintf(log, "wrote %d funscripts and %s to %s\n", len(res.Axes), filepath.Base(readme), dir)
	}
	fmt.Fprintln(log, out.Hints)
	return out, nil
}

// staleFiles lists the existing <base>.<axis>.funscript files in dir for
// axes stimconv can write (config ranges) that are not in wrote.
func staleFiles(dir, base string, wrote map[string]bool) []string {
	var axes []string
	for k := range config.Default().Ranges {
		axes = append(axes, k)
	}
	sort.Strings(axes)
	var stale []string
	for _, ax := range axes {
		if wrote[ax] {
			continue
		}
		p := filepath.Join(dir, base+"."+ax+".funscript")
		if st, err := os.Lstat(p); err == nil && st.Mode().IsRegular() {
			stale = append(stale, p)
		}
	}
	return stale
}

// Conflict is an existing file a conversion would overwrite or delete.
type Conflict struct {
	Path   string
	Delete bool // a stale funscript that would be removed, else overwritten
}

func (c Conflict) String() string {
	if c.Delete {
		return filepath.Base(c.Path) + " (delete)"
	}
	return filepath.Base(c.Path) + " (overwrite)"
}

// ErrDeclined is returned when Options.Confirm refused to touch existing files.
var ErrDeclined = errors.New("existing files kept; input skipped")

// ExistingFilesError reports existing files a conversion would overwrite or
// delete without Options.OverwriteFiles.
type ExistingFilesError struct{ Conflicts []Conflict }

func (e *ExistingFilesError) Error() string {
	n := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		n[i] = c.String()
	}
	return fmt.Sprintf("%d existing file(s) would be overwritten or deleted in %s: %s",
		len(e.Conflicts), filepath.Dir(e.Conflicts[0].Path), strings.Join(n, ", "))
}

// conflicts lists the existing files a conversion into dir would overwrite
// (its funscripts and report) or delete (stale funscripts).
func conflicts(dir, base, readme string, axes []*funscript.Axis, wrote map[string]bool) []Conflict {
	var c []Conflict
	for _, a := range axes {
		p := filepath.Join(dir, base+"."+a.Name+".funscript")
		if _, err := os.Lstat(p); err == nil {
			c = append(c, Conflict{Path: p})
		}
	}
	if _, err := os.Lstat(readme); err == nil {
		c = append(c, Conflict{Path: readme})
	}
	for _, p := range staleFiles(dir, base, wrote) {
		c = append(c, Conflict{Path: p, Delete: true})
	}
	return c
}

// removeStale deletes <base>.<axis>.funscript in dir for every axis stimconv
// can write (config ranges) that this run did not write, e.g. e1-e4 left
// from a quad-phase run when this one is tri-phase. restim would otherwise
// load them alongside the new files. Other funscripts are left alone.
func removeStale(dir, base string, wrote map[string]bool) ([]string, error) {
	var removed []string
	var errs []error
	for _, p := range staleFiles(dir, base, wrote) {
		if err := os.Remove(p); err != nil {
			errs = append(errs, fmt.Errorf("could not remove %s: %w", filepath.Base(p), err))
			continue
		}
		removed = append(removed, p)
	}
	return removed, errors.Join(errs...)
}

func printStats(log io.Writer, a *funscript.Axis, points int) {
	lo, hi, sum := math.Inf(1), math.Inf(-1), 0.0
	for _, v := range a.Values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		sum += v
	}
	fmt.Fprintf(log, "  %-22s min %9.3f  mean %9.3f  max %9.3f  (%d points, range %g..%g)\n",
		a.Name, lo, sum/float64(len(a.Values)), hi, points, a.Min, a.Max)
}
