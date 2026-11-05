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
	// Firmware is the FOC-Stim firmware the pulse timing is written for.
	// 2.x leaves at least 5 ms between pulses and accepts rates up to 200 Hz
	// (pairs with restim v1.67); 1.x has no minimum pause and accepts up to
	// 100 Hz (restim v1.66). Zero = not recorded (an older file; see
	// UpgradeFirmware), treated as CurrentFirmware.
	Firmware FirmwareVersion `json:"firmware"`
	// ContinuousPulse is the pulse train used when the rhythm is carried by
	// the volume axis (or no rhythm is present), on firmware 2.x:
	// "longest" plays the longest pulse FOC-Stim allows (20 cycles, at most
	// 35 ms) followed by the minimum pause, so the rate follows the carrier
	// (28 Hz at 650 Hz, 40 Hz at 1 kHz) and the duty stays at 73-86%;
	// "rate" plays ContinuousPulseHz with the pulse filling the period except
	// for the minimum pause (70 Hz: 9.3 ms on, 5 ms off, 65% duty).
	// Firmware 1.x always uses "rate" with the pulse filling 90% of the
	// period, as before firmware 2.0.
	ContinuousPulse string `json:"continuous_pulse"`
	// ContinuousPulseHz is the continuous pulse rate of the "rate" train.
	ContinuousPulseHz float64 `json:"continuous_pulse_hz"`
	// IFC (joined): "beat" renders |fA-fB| as the rhythm, "off" ignores it.
	IFC string `json:"ifc"`
	// Position (joined): "track" derives alpha/beta from the A/B relation;
	// "ab" holds the A-B edge (alpha 0.5, beta √3/2, FOC-Stim currents
	// 1, 1, 0), so current flows only between outputs A and B and C can stay
	// unconnected: how the original box played a mono track on one channel.
	Position string `json:"position"`

	// Intensity: "current" (peak electrode current) or "effective"
	// (strength-duration weighted charge; restim's TauCalibration then
	// handles the FOC carrier).
	Intensity string  `json:"intensity"`
	TauUS     float64 `json:"tau_us"` // nerve chronaxie, must match restim's tau setting
	// RefCarrierHz is the carrier at which restim's TauCalibration applies no
	// derating (the FOC maximum carrier, clipped by restim's safety limits).
	RefCarrierHz float64 `json:"ref_carrier_hz"`
	// Normalize: "peak", "p99" or "abs" (divide by RefLevel).
	Normalize string `json:"normalize"`
	// RefLevel is the abs level mapped to volume 1; 0 = auto: the level of
	// full-scale audio (see mapping.RefLevel).
	RefLevel  float64 `json:"ref_level"`
	Gamma     float64 `json:"gamma"`
	SilenceDB float64 `json:"silence_db"` // below track max -> silence

	ModSplitHz float64 `json:"mod_split_hz"` // slow/fast envelope split
	StepMS     float64 `json:"step_ms"`      // funscript sampling step
	Epsilon    float64 `json:"epsilon"`      // RDP tolerance in funscript position units

	Circuit Circuit `json:"circuit"`

	Ranges map[string]Range `json:"ranges"`
}

// FOC-Stim v4 axis limits (src/main_focstim_v4.cpp SimpleAxis declarations),
// for firmware 2.x. Firmware 1.x accepted pulse rates up to 100 Hz only; see
// Config.MaxPulseHz.
var FOCLimits = map[string]Range{
	"frequency":             {300, 2000},
	"pulse_frequency":       {1, 200}, // in practice limited by FOCMinPauseS
	"pulse_width":           {3, 20},
	"pulse_rise_time":       {2, 10},
	"pulse_interval_random": {0, 1},
}

// FOC-Stim v4 pulse timing limits (src/bsp/config_g473re_focstim_v4.h).
// Pulse rate, width and carrier together have to fit them: the firmware plays
// a pulse of width/carrier seconds, then pauses for at least FOCMinPauseS, so
// a pulse that leaves less room lowers the actual rate.
const (
	FOCMaxPulseS = 0.035 // longest pulse (s)
	FOCMinPauseS = 0.005 // shortest pause between pulses (s), firmware 2.x
)

// MinPauseS is the shortest pause between pulses on c's firmware: 5 ms on
// 2.x, none on 1.x.
func (c Config) MinPauseS() float64 {
	if c.fw1() {
		return 0
	}
	return FOCMinPauseS
}

// MaxPulseHz is the highest pulse rate c's firmware accepts.
func (c Config) MaxPulseHz() float64 {
	if c.fw1() {
		return 100
	}
	return FOCLimits["pulse_frequency"].Max
}

// LongestPulse reports whether continuous segments use the longest-pulse
// train (firmware 2.x with continuous_pulse "longest").
func (c Config) LongestPulse() bool { return !c.fw1() && c.ContinuousPulse == "longest" }

// fw1 reports whether c targets firmware 1.x (no minimum pause).
func (c Config) fw1() bool { return !c.Firmware.IsZero() && c.Firmware.Major < 2 }

// FW1 reports whether c targets firmware 1.x, whose fixed-rate train fills
// 90% of the period.
func (c Config) FW1() bool { return c.fw1() }

// Default returns the default configuration, the "original" settings: the
// volume follows the audio's peak level as recorded (abs against full scale,
// gamma 1, no circuit model, plain peak current). Listening tests found that
// the circuit model and effective intensity over-weight square waves, which
// unbalanced tracks mixing waveforms. presets.Builtins derives the other
// built-in presets from it. Ranges default to restim's funscript kit
// defaults (qt_ui/models/funscript_kit.py), except pulse_width, which goes
// up to FOC-Stim's 20 cycles for the longest-pulse train (the report asks for
// the same range in restim).
func Default() Config {
	return Config{
		Topology:          "joined",
		Overlap:           "auto",
		MuxHz:             4,
		AutoCorr:          0.7,
		AutoWindowS:       1,
		Params:            "weighted",
		Rhythm:            "auto",
		FusionHz:          12,
		Firmware:          CurrentFirmware,
		ContinuousPulse:   "longest",
		ContinuousPulseHz: 70,
		IFC:               "beat",
		Position:          "track",
		Intensity:         "current",
		TauUS:             355,
		RefCarrierHz:      2000,
		Normalize:         "abs",
		RefLevel:          0,
		Gamma:             1,
		SilenceDB:         -40,
		ModSplitHz:        3,
		StepMS:            10,
		Epsilon:           0.5,
		Circuit: Circuit{
			Enabled:  false,
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
			"pulse_width":           {4, 20}, // restim: 4-10; 20 lets the longest-pulse train through
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
	if HasPEM(b) { // a settings report
		e, err := DecodePEM(b)
		if err != nil {
			return c, fmt.Errorf("%s: %w", path, err)
		}
		return e.Config, nil
	}
	if err := Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, c.Validate()
}

// Rate70 turns c into the continuous train of stimconv before the
// longest-pulse train: 70 Hz fixed rate on firmware 2.x, with restim's
// default 4-10 cycle pulse_width range.
func Rate70(c Config) Config {
	c.Firmware, c.ContinuousPulse, c.ContinuousPulseHz = CurrentFirmware, "rate", 70
	r := make(map[string]Range, len(c.Ranges))
	for k, v := range c.Ranges {
		r[k] = v
	}
	r["pulse_width"] = Range{4, 10}
	c.Ranges = r
	return c
}

// Unmarshal merges JSON over c. A file without continuous_pulse predates
// the longest-pulse train, so it keeps the "rate" train it was made with
// rather than taking today's default. A file without firmware keeps it zero
// (unrecorded), so UpgradeFirmware can flag it.
func Unmarshal(b []byte, c *Config) error {
	c.ContinuousPulse, c.Firmware = "", FirmwareVersion{}
	if err := json.Unmarshal(b, c); err != nil {
		return err
	}
	if c.ContinuousPulse == "" {
		c.ContinuousPulse = "rate"
	}
	return nil
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
		oneOf("position", c.Position, "track", "ab"),
		oneOf("intensity", c.Intensity, "current", "effective"),
		oneOf("normalize", c.Normalize, "peak", "p99", "abs"),
		oneOf("continuous_pulse", c.ContinuousPulse, "longest", "rate"),
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
	if !c.Firmware.IsZero() && c.Firmware.Major < 1 {
		return fmt.Errorf("invalid firmware version %s", c.Firmware)
	}
	if c.RefLevel < 0 {
		return fmt.Errorf("ref_level must be >= 0 (0 = auto)")
	}
	if c.StepMS <= 0 || c.ModSplitHz <= 0 || c.Gamma <= 0 {
		return fmt.Errorf("step_ms, mod_split_hz and gamma must be > 0")
	}
	return nil
}
