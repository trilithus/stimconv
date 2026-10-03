package config

import "sort"

// Kind is the value type of an Option.
type Kind int

const (
	Enum  Kind = iota // string, one of Choices
	Float             // float64
	Bool              // bool
)

// Choice is one allowed value of an Enum option.
type Choice struct {
	Value, Label string
}

// Option describes one Config field for a user interface: label, help text,
// grouping and accessors. The CLI flags are defined separately in main.go.
type Option struct {
	Key      string // JSON path, e.g. "gamma" or "circuit.turns"
	Label    string
	Help     string
	Group    string
	Kind     Kind
	Choices  []Choice
	Advanced bool              // only shown in expert mode
	OnlyIf   func(Config) bool // nil = always relevant
	Get      func(*Config) any
	Set      func(*Config, any)
}

// Option groups, in display order.
const (
	GroupOutput   = "Output type"
	GroupBasic    = "Basic"
	GroupMixing   = "Rhythm and A/B mixing"
	GroupLevel    = "Intensity and envelope"
	GroupSampling = "Output sampling"
	GroupCircuit  = "Original circuit model"
	GroupRanges   = "Funscript ranges (must match restim's funscript kit)"
)

func isDual(c Config) bool   { return c.Topology == "dual" }
func isJoined(c Config) bool { return c.Topology == "joined" }

func str(name, label, help, group string, adv bool, only func(Config) bool, p func(*Config) *string, ch ...Choice) Option {
	return Option{Key: name, Label: label, Help: help, Group: group, Kind: Enum, Choices: ch, Advanced: adv, OnlyIf: only,
		Get: func(c *Config) any { return *p(c) }, Set: func(c *Config, v any) { *p(c) = v.(string) }}
}

func num(name, label, help, group string, adv bool, only func(Config) bool, p func(*Config) *float64) Option {
	return Option{Key: name, Label: label, Help: help, Group: group, Kind: Float, Advanced: adv, OnlyIf: only,
		Get: func(c *Config) any { return *p(c) }, Set: func(c *Config, v any) { *p(c) = v.(float64) }}
}

func circ(name, label, help string, p func(*Circuit) *float64) Option {
	return num("circuit."+name, label, help, GroupCircuit, true,
		func(c Config) bool { return c.Circuit.Enabled }, func(c *Config) *float64 { return p(&c.Circuit) })
}

// Options lists every Config field in display order. The first entry is the
// output type (topology), the most commonly changed setting.
var Options = buildOptions()

func buildOptions() []Option {
	o := []Option{
		str("topology", "Output", "Quad-phase: channel A on e1/e2 and B on e3/e4 (restim FOC-Stim 4-phase). Tri-phase: commons tied, written as alpha/beta (restim FOC-Stim 3-phase).",
			GroupOutput, false, nil, func(c *Config) *string { return &c.Topology },
			Choice{"dual", "Quad-phase (4 electrodes, FOC-Stim 4-phase)"}, Choice{"joined", "Tri-phase (commons joined, FOC-Stim 3-phase)"}),

		str("overlap", "When A and B overlap", "auto multiplexes only independent A/B content; blend mixes positions; dominant follows the louder channel; multiplex always alternates.",
			GroupBasic, false, isDual, func(c *Config) *string { return &c.Overlap },
			Choice{"auto", "auto"}, Choice{"blend", "blend"}, Choice{"dominant", "dominant"}, Choice{"multiplex", "multiplex"}),
		str("rhythm", "Render rhythm as", "pulses uses FOC pulse axes; volume modulates the volume axis; auto chooses by the fusion frequency.",
			GroupBasic, false, nil, func(c *Config) *string { return &c.Rhythm },
			Choice{"auto", "auto"}, Choice{"pulses", "pulses"}, Choice{"volume", "volume"}),
		str("intensity", "Volume measure", "effective: strength-duration matched (restim's tau then compensates carrier changes); current: peak electrode current.",
			GroupBasic, false, nil, func(c *Config) *string { return &c.Intensity },
			Choice{"effective", "effective (strength-duration)"}, Choice{"current", "current"}),
		str("normalize", "Volume normalisation", "p99: 99th percentile maps to volume 1; peak: the maximum does; abs: divide by the reference level.",
			GroupBasic, false, nil, func(c *Config) *string { return &c.Normalize },
			Choice{"p99", "p99"}, Choice{"peak", "peak"}, Choice{"abs", "abs"}),
		num("gamma", "Volume gamma", "Volume exponent after normalisation (1 = linear, >1 = quieter low levels).",
			GroupBasic, false, nil, func(c *Config) *float64 { return &c.Gamma }),

		num("mux_hz", "Multiplex rate (Hz)", "A/B alternation rate. restim ramps positions over ~30-45 ms, so keep <= 4.",
			GroupMixing, true, isDual, func(c *Config) *float64 { return &c.MuxHz }),
		num("auto_corr", "Independence correlation", "auto overlap: L/R correlation below which A and B count as independent.",
			GroupMixing, true, isDual, func(c *Config) *float64 { return &c.AutoCorr }),
		num("auto_window_s", "Independence window (s)", "auto overlap: majority window for the multiplex decision.",
			GroupMixing, true, isDual, func(c *Config) *float64 { return &c.AutoWindowS }),
		str("params", "Carrier/rhythm merge", "How differing A/B carrier and rhythm are merged. constant writes no pulse axes.",
			GroupMixing, true, nil, func(c *Config) *string { return &c.Params },
			Choice{"weighted", "weighted"}, Choice{"dominant", "dominant"}, Choice{"constant", "constant"}),
		num("fusion_hz", "Fusion frequency (Hz)", "auto rhythm: rhythms at or above this rate become FOC pulses.",
			GroupMixing, true, nil, func(c *Config) *float64 { return &c.FusionHz }),
		num("continuous_pulse_hz", "Continuous pulse rate (Hz)", "FOC pulse rate for steady or volume-rendered segments.",
			GroupMixing, true, nil, func(c *Config) *float64 { return &c.ContinuousPulseHz }),
		str("ifc", "Interferential beat", "Tri-phase: render the beat |fA-fB| of different carriers as rhythm, or ignore it.",
			GroupMixing, true, isJoined, func(c *Config) *string { return &c.IFC },
			Choice{"beat", "beat"}, Choice{"off", "off"}),

		num("tau_us", "Chronaxie tau (µs)", "Nerve chronaxie; must match restim's tau setting.",
			GroupLevel, true, func(c Config) bool { return c.Intensity == "effective" }, func(c *Config) *float64 { return &c.TauUS }),
		num("ref_carrier_hz", "Reference carrier (Hz)", "Carrier at which restim applies no tau derating (FOC maximum carrier).",
			GroupLevel, true, func(c Config) bool { return c.Intensity == "effective" }, func(c *Config) *float64 { return &c.RefCarrierHz }),
		num("ref_level", "Reference level", "abs normalisation: level mapped to volume 1.",
			GroupLevel, true, func(c Config) bool { return c.Normalize == "abs" }, func(c *Config) *float64 { return &c.RefLevel }),
		num("silence_db", "Silence threshold (dB)", "Envelope below the track maximum by this many dB is silence.",
			GroupLevel, true, nil, func(c *Config) *float64 { return &c.SilenceDB }),
		num("mod_split_hz", "Envelope split (Hz)", "Split between slow (volume/position) and fast (rhythm) envelope.",
			GroupLevel, true, nil, func(c *Config) *float64 { return &c.ModSplitHz }),

		num("step_ms", "Sampling step (ms)", "Funscript sampling step before simplification.",
			GroupSampling, true, nil, func(c *Config) *float64 { return &c.StepMS }),
		num("epsilon", "Simplification tolerance", "Tolerance in funscript position units (0 = off).",
			GroupSampling, true, nil, func(c *Config) *float64 { return &c.Epsilon }),

		{Key: "circuit.enabled", Label: "Model the circuit", Help: "Model the original amplifier, transformer and skin, so analysis runs on electrode current.",
			Group: GroupCircuit, Kind: Bool, Advanced: true,
			Get: func(c *Config) any { return c.Circuit.Enabled }, Set: func(c *Config, v any) { c.Circuit.Enabled = v.(bool) }},
		circ("amp_v_peak", "Amplifier peak (V)", "Amplifier volts at full-scale audio.", func(c *Circuit) *float64 { return &c.AmpVPeak }),
		circ("series_r", "Series resistor (Ω)", "Series resistor.", func(c *Circuit) *float64 { return &c.SeriesR }),
		circ("winding_r", "Winding resistance (Ω)", "Primary winding resistance.", func(c *Circuit) *float64 { return &c.WindingR }),
		circ("turns", "Transformer ratio", "Secondary:primary voltage ratio.", func(c *Circuit) *float64 { return &c.Turns }),
		circ("mag_l", "Magnetising inductance (H)", "Primary magnetising inductance.", func(c *Circuit) *float64 { return &c.MagL }),
		circ("skin_r", "Skin resistance (Ω)", "Series skin/electrode resistance.", func(c *Circuit) *float64 { return &c.SkinR }),
		circ("skin_par_r", "Skin parallel R (Ω)", "Parallel skin resistance.", func(c *Circuit) *float64 { return &c.SkinParR }),
		circ("skin_par_c", "Skin parallel C (F)", "Parallel skin capacitance.", func(c *Circuit) *float64 { return &c.SkinParC }),
	}
	axes := make([]string, 0, len(Default().Ranges))
	for k := range Default().Ranges {
		axes = append(axes, k)
	}
	sort.Strings(axes)
	for _, ax := range axes {
		for _, end := range []string{"min", "max"} {
			o = append(o, rangeOpt(ax, end))
		}
	}
	return o
}

func rangeOpt(axis, end string) Option {
	only := func(c Config) bool {
		switch axis {
		case "e1", "e2", "e3", "e4":
			return isDual(c)
		case "alpha", "beta":
			return isJoined(c)
		}
		return true
	}
	return Option{Key: "ranges." + axis + "." + end, Label: axis + " " + end,
		Help:  "Funscript position 0..100 maps linearly onto this range; set the same range in restim.",
		Group: GroupRanges, Kind: Float, Advanced: true, OnlyIf: only,
		Get: func(c *Config) any {
			r := c.Ranges[axis]
			if end == "min" {
				return r.Min
			}
			return r.Max
		},
		Set: func(c *Config, v any) {
			// copy-on-write so configs sharing the map stay independent
			m := make(map[string]Range, len(c.Ranges))
			for k, r := range c.Ranges {
				m[k] = r
			}
			r := m[axis]
			if end == "min" {
				r.Min = v.(float64)
			} else {
				r.Max = v.(float64)
			}
			m[axis] = r
			c.Ranges = m
		}}
}
