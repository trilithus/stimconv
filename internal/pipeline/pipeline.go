// Package pipeline runs one conversion (decode → measure → derive → map →
// write) so the CLI and the GUI share exactly the same code path.
package pipeline

import (
	"context"
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
	OutDir string // default: DefaultOutDir(Input, OutName, Preset)
	// OutName replaces the input name (without extension) in the output
	// names, e.g. "PEP11" for PEP11.fr.mp3 -> PEP11.alpha.funscript.
	// "" uses the input name; see ValidateOutName.
	OutName   string
	Preset    string // names the default output folder; "" = "default"
	DumpCSV   string // per-hop features CSV, empty = off
	DryRun    bool   // analyse only, write no funscripts
	Stats     bool   // print per-axis statistics
	NativeMP3 bool   // decode Layer III with go-mp3 instead of ffmpeg
	// AudioTrack picks an audio stream in video/container files, numbered
	// from 1 (decode.Track.Index+1); 0 uses the default track.
	AudioTrack int
}

// DefaultOutDir is the output folder used when none is given: a folder next
// to the input named after the output name (see OutBase) and the preset,
// e.g. "track.default". preset must already be a safe folder name.
func DefaultOutDir(input, outName, preset string) string {
	if preset == "" {
		preset = "default"
	}
	return filepath.Join(filepath.Dir(input), OutBase(input, outName)+"."+preset)
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
		dir = DefaultOutDir(in, o.OutName, o.Preset)
	}
	out.OutDir = dir
	if !o.DryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return out, err
		}
	}
	base := OutBase(in, o.OutName)
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
		readme := filepath.Join(dir, ReadmeName(in, o.OutName, o.OutDir != ""))
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

func printStats(log io.Writer, a *funscript.Axis, points int) {
	lo, hi, sum := math.Inf(1), math.Inf(-1), 0.0
	for _, v := range a.Values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		sum += v
	}
	fmt.Fprintf(log, "  %-22s min %9.3f  mean %9.3f  max %9.3f  (%d points, range %g..%g)\n",
		a.Name, lo, sum/float64(len(a.Values)), hi, points, a.Min, a.Max)
}
