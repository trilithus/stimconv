// Command stimconv converts audio-driven e-stim tracks into restim funscripts
// for FOC-Stim.
//
//	stimconv [--console]           (graphical interface)
//	stimconv cli [flags] <audio file>
//	stimconv tools teleplot [flags]   (terminal link monitor for restim)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/gogpu/gg/gpu" // GPU acceleration for the GUI

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/decode"
	"github.com/trilithus/stimconv/internal/gui"
	"github.com/trilithus/stimconv/internal/pipeline"
	"github.com/trilithus/stimconv/internal/presets"
	"github.com/trilithus/stimconv/internal/teleplot"
)

type rangeFlags map[string]config.Range

func (r rangeFlags) String() string { return "" }

func (r rangeFlags) Set(s string) error {
	name, spec, ok := strings.Cut(s, "=")
	lo, hi, ok2 := strings.Cut(spec, ":")
	if !ok || !ok2 {
		return fmt.Errorf("want axis=min:max, got %q", s)
	}
	a, err1 := strconv.ParseFloat(lo, 64)
	b, err2 := strconv.ParseFloat(hi, 64)
	if err1 != nil || err2 != nil || b <= a {
		return fmt.Errorf("bad range %q", s)
	}
	r[name] = config.Range{Min: a, Max: b}
	return nil
}

func main() {
	args := os.Args[1:]
	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == "--console"):
		if len(args) == 0 {
			hideOwnConsole()
		}
		if err := gui.Run(iconFS(), appIcon()); err != nil {
			fmt.Fprintln(os.Stderr, "stimconv:", err)
			os.Exit(1)
		}
		return
	case args[0] == "tools":
		if err := runTools(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "stimconv:", err)
			os.Exit(1)
		}
		return
	case args[0] != "cli":
		fmt.Fprintln(os.Stderr, "Usage:\n  stimconv                          open the graphical interface\n  stimconv --console                GUI, keeping the console window for log output (Windows)\n  stimconv cli [flags] <audio file>  command-line conversion (stimconv cli -h for flags)\n  stimconv tools teleplot [flags]    terminal monitor for restim's FOC-Stim link metrics")
		os.Exit(2)
	}
	if err := run(args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "stimconv:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// the config file provides the defaults the flags then override
	cfg := config.Default()
	for i, a := range args {
		if (a == "--config" || a == "-config") && i+1 < len(args) {
			c, err := config.Load(args[i+1])
			if err != nil {
				return err
			}
			cfg = c
		} else if v, ok := strings.CutPrefix(a, "--config="); ok {
			c, err := config.Load(v)
			if err != nil {
				return err
			}
			cfg = c
		}
	}

	fs := flag.NewFlagSet("stimconv", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: stimconv cli [flags] <audio file>\n\nConverts an audio stim track into restim funscripts for FOC-Stim.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	fs.String("config", "", "JSON config file (flags override it)")
	emit := fs.String("emit-config", "", "write the effective config as JSON to this path")
	outDir := fs.String("o", "", "output directory (default: <input name without extension>.<preset> next to the input; preset = --config file name or \"default\")")
	stats := fs.Bool("stats", false, "print per-axis statistics")
	dump := fs.String("dump-features", "", "write per-hop analysis features to this CSV path")
	dry := fs.Bool("dry-run", false, "analyse only, do not write funscripts")
	fs.StringVar(&decode.FFmpegPath, "ffmpeg", "", "path to ffmpeg (default: $STIMCONV_FFMPEG, libs/ffmpeg/bin, PATH)")
	audioTrack := fs.Int("audio-track", 0, "audio track to use in video/container files, numbered from 1 (0 = the default track); see --list-tracks")
	listTracks := fs.Bool("list-tracks", false, "list the audio tracks of the input file and exit")
	nativeMP3 := fs.Bool("native-mp3", false, "decode MPEG Layer III with go-mp3 instead of ffmpeg (experimental)")

	fs.StringVar(&cfg.Topology, "topology", cfg.Topology, "electrode wiring: dual (A=e1/e2, B=e3/e4; FOC-Stim 4-phase) | joined (commons tied; alpha/beta, FOC-Stim 3-phase)")
	fs.StringVar(&cfg.Overlap, "overlap", cfg.Overlap, "dual: when both channels are active: auto (multiplex only independent A/B content) | blend | dominant | multiplex")
	fs.Float64Var(&cfg.MuxHz, "mux-hz", cfg.MuxHz, "multiplex/auto: A/B alternation rate (restim ramps positions over ~30-45 ms; keep <= 4)")
	fs.Float64Var(&cfg.AutoCorr, "auto-corr", cfg.AutoCorr, "auto: L/R correlation below which A and B count as independent")
	fs.Float64Var(&cfg.AutoWindowS, "auto-window-s", cfg.AutoWindowS, "auto: majority window for the multiplex decision (s)")
	fs.StringVar(&cfg.Params, "params", cfg.Params, "merge of differing A/B carrier/rhythm: weighted | dominant | constant (don't write pulse axes)")
	fs.StringVar(&cfg.Rhythm, "rhythm", cfg.Rhythm, "render envelope rhythm as: pulses | volume | auto (by fusion frequency)")
	fs.Float64Var(&cfg.FusionHz, "fusion-hz", cfg.FusionHz, "auto: rhythms at or above this rate become FOC pulses")
	fs.Float64Var(&cfg.ContinuousPulseHz, "continuous-pulse-hz", cfg.ContinuousPulseHz, "FOC pulse rate for steady / volume-rendered segments")
	fs.StringVar(&cfg.IFC, "ifc", cfg.IFC, "joined: interferential beat of different carriers: beat | off")
	fs.StringVar(&cfg.Intensity, "intensity", cfg.Intensity, "volume measure: effective (strength-duration) | current")
	fs.Float64Var(&cfg.TauUS, "tau-us", cfg.TauUS, "nerve chronaxie in microseconds (match restim's tau setting)")
	fs.Float64Var(&cfg.RefCarrierHz, "ref-carrier-hz", cfg.RefCarrierHz, "carrier at which restim applies no tau derating (FOC max carrier)")
	fs.StringVar(&cfg.Normalize, "normalize", cfg.Normalize, "volume normalisation: p99 | peak | abs")
	fs.Float64Var(&cfg.RefLevel, "ref-level", cfg.RefLevel, "abs normalisation: level mapped to volume 1")
	fs.Float64Var(&cfg.Gamma, "gamma", cfg.Gamma, "volume exponent after normalisation")
	fs.Float64Var(&cfg.SilenceDB, "silence-db", cfg.SilenceDB, "envelope below track max by this many dB is silence")
	fs.Float64Var(&cfg.ModSplitHz, "mod-split-hz", cfg.ModSplitHz, "split between slow (volume/position) and fast (rhythm) envelope")
	fs.Float64Var(&cfg.StepMS, "step-ms", cfg.StepMS, "funscript sampling step before simplification")
	fs.Float64Var(&cfg.Epsilon, "epsilon", cfg.Epsilon, "simplification tolerance in funscript position units (0 = off)")
	fs.BoolVar(&cfg.Circuit.Enabled, "circuit", cfg.Circuit.Enabled, "model the original amp/transformer/skin circuit")
	fs.Float64Var(&cfg.Circuit.AmpVPeak, "amp-v-peak", cfg.Circuit.AmpVPeak, "circuit: amplifier volts at full-scale audio")
	fs.Float64Var(&cfg.Circuit.SeriesR, "series-r", cfg.Circuit.SeriesR, "circuit: series resistor (ohm)")
	fs.Float64Var(&cfg.Circuit.WindingR, "winding-r", cfg.Circuit.WindingR, "circuit: primary winding resistance (ohm)")
	fs.Float64Var(&cfg.Circuit.Turns, "turns", cfg.Circuit.Turns, "circuit: transformer voltage ratio")
	fs.Float64Var(&cfg.Circuit.MagL, "mag-l", cfg.Circuit.MagL, "circuit: magnetising inductance (H)")
	fs.Float64Var(&cfg.Circuit.SkinR, "skin-r", cfg.Circuit.SkinR, "circuit: series skin resistance (ohm)")
	fs.Float64Var(&cfg.Circuit.SkinParR, "skin-par-r", cfg.Circuit.SkinParR, "circuit: parallel skin resistance (ohm)")
	fs.Float64Var(&cfg.Circuit.SkinParC, "skin-par-c", cfg.Circuit.SkinParC, "circuit: parallel skin capacitance (F)")
	ranges := rangeFlags{}
	fs.Var(ranges, "range", "funscript range of an axis, axis=min:max (repeatable); must match restim's funscript kit")

	if err := fs.Parse(args); err != nil {
		return err
	}
	for k, v := range ranges {
		cfg.Ranges[k] = v
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if *emit != "" {
		if err := cfg.Save(*emit); err != nil {
			return err
		}
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one input file")
	}
	in := fs.Arg(0)
	if *listTracks {
		info, err := decode.Probe(in)
		if err != nil {
			return err
		}
		for _, t := range info.Tracks {
			fmt.Printf("%d: %s\n", t.Index+1, t)
		}
		return nil
	}

	_, err := pipeline.Convert(context.Background(), cfg, pipeline.Options{
		Input: in, OutDir: *outDir, Preset: presetName(args), DumpCSV: *dump, DryRun: *dry, Stats: *stats, NativeMP3: *nativeMP3,
		AudioTrack: *audioTrack,
	}, os.Stdout)
	return err
}

// presetName names the default output folder after the --config file
// (its name without extension, if that is a valid preset name).
func presetName(args []string) string {
	for i, a := range args {
		v, ok := strings.CutPrefix(a, "--config=")
		if !ok && (a == "--config" || a == "-config") && i+1 < len(args) {
			v, ok = args[i+1], true
		}
		if ok {
			return presets.FolderName(strings.TrimSuffix(filepath.Base(v), filepath.Ext(v)))
		}
	}
	return presets.DefaultName
}

func runTools(args []string) error {
	if len(args) == 0 || args[0] != "teleplot" {
		return fmt.Errorf("usage: stimconv tools teleplot [flags]")
	}
	fs := flag.NewFlagSet("stimconv tools teleplot", flag.ContinueOnError)
	o := teleplot.Options{}
	fs.StringVar(&o.Addr, "listen", teleplot.DefaultAddr, "UDP address to listen on (restim sends to 127.0.0.1:47269)")
	fs.DurationVar(&o.Window, "window", time.Minute, "history kept and shown")
	fs.DurationVar(&o.Refresh, "refresh", 250*time.Millisecond, "redraw interval")
	fs.IntVar(&o.Width, "width", 60, "history columns")
	fs.StringVar(&o.Prefix, "prefix", "", "restim's FOC-Stim teleplot prefix setting, if set")
	fs.StringVar(&o.Filter, "filter", "", "show only metrics whose name contains this")
	fs.StringVar(&o.Forward, "forward", "", "also re-send every datagram to this UDP address (e.g. a Teleplot app on another port)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if o.Window <= 0 || o.Refresh <= 0 || o.Width < 1 {
		return fmt.Errorf("--window, --refresh and --width must be positive")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return teleplot.Run(ctx, o, os.Stdout)
}
