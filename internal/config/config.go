// Package config holds every tunable of the conversion so experiment runs
// can be reproduced from a JSON file (--config / --emit-config).
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Range maps a physical axis value onto funscript positions 0..100.
// It must match the range configured for that axis in restim's funscript kit.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Circuit describes the original amplifier -> resistor -> transformer -> skin chain.
type Circuit struct {
	Enabled  bool    `json:"enabled"`
	AmpVPeak float64 `json:"amp_v_peak"` // amplifier output voltage at full-scale audio (V)
	SeriesR  float64 `json:"series_r"`   // series resistor (ohm)
	WindingR float64 `json:"winding_r"`  // primary winding resistance (ohm)
	Turns    float64 `json:"turns"`      // secondary:primary voltage ratio
	MagL     float64 `json:"mag_l"`      // primary magnetising inductance (H)
	SkinR    float64 `json:"skin_r"`     // series skin/electrode resistance (ohm)
	SkinParR float64 `json:"skin_par_r"` // parallel skin resistance (ohm)
	SkinParC float64 `json:"skin_par_c"` // parallel skin capacitance (F)
}

// Config is the complete conversion configuration.
type Config struct {
	// topology: "dual" (A=e1/e2, B=e3/e4, restim FOC-Stim 4-phase) or
	// "joined" (commons tied, alpha/beta, restim FOC-Stim 3-phase).
	Topology string `json:"topology"`
	// overlap (dual): "auto", "blend", "dominant" or "multiplex".
	// auto multiplexes only while A and B carry independent content
	// (different carrier, rhythm, or L/R correlation below AutoCorr),
	// decided by majority over AutoWindowS, and blends otherwise.
	Overlap     string  `json:"overlap"`
	MuxHz       float64 `json:"mux_hz"`
	AutoCorr    float64 `json:"auto_corr"`
	AutoWindowS float64 `json:"auto_window_s"`
	// params: how conflicting A/B carrier/rhythm are merged:
	// "weighted", "dominant" or "constant" (pulse axes not written).
	Params string `json:"params"`
	// rhythm: "pulses", "volume" or "auto".
	Rhythm string `json:"rhythm"`
	// FusionHz: in auto mode, rhythms below this rate are rendered as
	// volume modulation (individually felt twitches), above it as FOC pulses.
	FusionHz float64 `json:"fusion_hz"`
	// ContinuousPulseHz is the FOC pulse rate used when the rhythm is carried
	// by the volume axis (or no rhythm is present).
	ContinuousPulseHz float64 `json:"continuous_pulse_hz"`
	// IFC (joined): "beat" renders |fA-fB| as the rhythm, "off" ignores it.
	IFC string `json:"ifc"`

	// Intensity: "current" (peak electrode current) or "effective"
	// (strength-duration weighted charge; restim's TauCalibration then
	// handles the FOC carrier).
	Intensity string  `json:"intensity"`
	TauUS     float64 `json:"tau_us"` // nerve chronaxie, must match restim's tau setting
	// RefCarrierHz is the carrier at which restim's TauCalibration applies no
	// derating (the FOC maximum carrier, clipped by restim's safety limits).
	RefCarrierHz float64 `json:"ref_carrier_hz"`
	// Normalize: "peak", "p99" or "abs" (divide by RefLevel).
	Normalize string  `json:"normalize"`
	RefLevel  float64 `json:"ref_level"`
	Gamma     float64 `json:"gamma"`
	SilenceDB float64 `json:"silence_db"` // below track max -> silence

	ModSplitHz float64 `json:"mod_split_hz"` // slow/fast envelope split
	StepMS     float64 `json:"step_ms"`      // funscript sampling step
	Epsilon    float64 `json:"epsilon"`      // RDP tolerance in funscript position units

	Circuit Circuit `json:"circuit"`

	Ranges map[string]Range `json:"ranges"`
}

// FOC-Stim v4 axis limits (src/main_focstim_v4.cpp SimpleAxis declarations).
var FOCLimits = map[string]Range{
	"frequency":             {300, 2000},
	"pulse_frequency":       {1, 100},
	"pulse_width":           {3, 20},
	"pulse_rise_time":       {2, 10},
	"pulse_interval_random": {0, 1},
}

// Default returns the default configuration. Ranges default to restim's
// funscript kit defaults (qt_ui/models/funscript_kit.py).
func Default() Config {
	return Config{
		Topology:          "dual",
		Overlap:           "auto",
		MuxHz:             4,
		AutoCorr:          0.7,
		AutoWindowS:       1,
		Params:            "weighted",
		Rhythm:            "auto",
		FusionHz:          12,
		ContinuousPulseHz: 100,
		IFC:               "beat",
		Intensity:         "effective",
		TauUS:             355,
		RefCarrierHz:      2000,
		Normalize:         "p99",
		RefLevel:          1,
		Gamma:             0.85,
		SilenceDB:         -40,
		ModSplitHz:        3,
		StepMS:            10,
		Epsilon:           0.5,
		Circuit: Circuit{
			Enabled:  true,
			AmpVPeak: 12,
			SeriesR:  3.9,
			WindingR: 1.0,
			Turns:    35, // 70 V line, 0.5 W tap: sqrt(70^2/0.5/8)
			MagL:     0.02,
			SkinR:    500,
			SkinParR: 5000,
			SkinParC: 50e-9,
		},
		Ranges: map[string]Range{
			"volume":                {0, 1},
			"e1":                    {0, 1},
			"e2":                    {0, 1},
			"e3":                    {0, 1},
			"e4":                    {0, 1},
			"alpha":                 {-1, 1},
			"beta":                  {-1, 1},
			"frequency":             {500, 1000},
			"pulse_frequency":       {0, 100},
			"pulse_width":           {4, 10},
			"pulse_rise_time":       {2, 20},
			"pulse_interval_random": {0, 1},
		},
	}
}

// Load merges a JSON file over the defaults.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, c.Validate()
}

// Save writes the configuration as indented JSON.
func (c Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func oneOf(name, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("invalid %s %q (allowed: %v)", name, v, allowed)
}

// Validate checks enumerated options.
func (c Config) Validate() error {
	for _, e := range []error{
		oneOf("topology", c.Topology, "dual", "joined"),
		oneOf("overlap", c.Overlap, "auto", "blend", "dominant", "multiplex"),
		oneOf("params", c.Params, "weighted", "dominant", "constant"),
		oneOf("rhythm", c.Rhythm, "pulses", "volume", "auto"),
		oneOf("ifc", c.IFC, "beat", "off"),
		oneOf("intensity", c.Intensity, "current", "effective"),
		oneOf("normalize", c.Normalize, "peak", "p99", "abs"),
	} {
		if e != nil {
			return e
		}
	}
	for _, k := range []string{"frequency", "pulse_frequency", "pulse_width", "pulse_rise_time"} {
		if r, ok := c.Ranges[k]; !ok || r.Max <= r.Min {
			return fmt.Errorf("invalid range for %s", k)
		}
	}
	if c.StepMS <= 0 || c.ModSplitHz <= 0 || c.Gamma <= 0 {
		return fmt.Errorf("step_ms, mod_split_hz and gamma must be > 0")
	}
	return nil
}
