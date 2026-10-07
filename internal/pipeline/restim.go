package pipeline

import (
	"fmt"
	"math"
	"strings"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/funscript"
)

// RestimSetting is one restim setting the output depends on.
type RestimSetting struct {
	Where   string // restim screen
	Name    string // label as restim shows it
	Need    string // value the output expects
	Default string // restim's out-of-the-box value
	Change  bool   // the default does not fit this output
}

// The reference versions the defaults below were read from (the
// reference/restim and reference/FOC-Stim submodules; a test keeps them in
// sync).
const (
	RestimVersion  = "v1.66"
	FOCStimVersion = "v1.3.2"
)

// restim's defaults, from reference/restim: qt_ui/settings.py (device
// configuration and volume tab) and qt_ui/models/funscript_kit.py.
const (
	restimMinCarrier   = 500.0
	restimMaxCarrier   = 1000.0
	restimAmplitudeMA  = 120.0
	restimTauUS        = 355.0
	restimPulseNorm    = true
	restimBurstGapMode = true
	restimCenterReduct = 0.07
)

var restimKitRanges = map[string]config.Range{
	"alpha": {Min: -1, Max: 1}, "beta": {Min: -1, Max: 1},
	"volume": {Min: 0, Max: 1}, "frequency": {Min: 500, Max: 1000},
	"pulse_frequency": {Min: 0, Max: 100}, "pulse_width": {Min: 4, Max: 10},
	"pulse_interval_random": {Min: 0, Max: 1}, "pulse_rise_time": {Min: 2, Max: 20},
	"e1": {Min: 0, Max: 1}, "e2": {Min: 0, Max: 1}, "e3": {Min: 0, Max: 1}, "e4": {Min: 0, Max: 1},
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// RestimSettings lists the restim settings the written axes depend on, with
// restim's default and whether it has to change.
func RestimSettings(cfg config.Config, axes []*funscript.Axis) []RestimSetting {
	byName := map[string]*funscript.Axis{}
	for _, a := range axes {
		byName[a.Name] = a
	}
	var out []RestimSetting
	add := func(where, name, need, def string, change bool) {
		out = append(out, RestimSetting{where, name, need, def, change})
	}
	const wizard, vol, kit, four = "Device wizard", "Volume tab", "Funscript kit", "4-phase calibration"

	mode := "FOC-Stim 4-phase"
	if cfg.Topology == "joined" {
		mode = "FOC-Stim 3-phase"
	}
	add(wizard, "Device type", mode, "-", false)

	// restim clamps the carrier to its safety limits, and its tau derating
	// is relative to the maximum, so the maximum is fixed for "effective"
	if a := byName["frequency"]; a != nil && len(a.Values) > 0 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range a.Values {
			v = math.Max(a.Min, math.Min(a.Max, v)) // as written to the funscript
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		lo, hi = math.Floor(lo), math.Ceil(hi)
		add(wizard, "Minimum frequency [Hz]", fmt.Sprintf("%g or lower", lo), fmt.Sprintf("%g", restimMinCarrier), lo < restimMinCarrier)
		if cfg.Intensity == "effective" {
			add(wizard, "Maximum frequency [Hz]", fmt.Sprintf("exactly %g (volume is tau-matched to it)", cfg.RefCarrierHz),
				fmt.Sprintf("%g", restimMaxCarrier), cfg.RefCarrierHz != restimMaxCarrier)
		} else {
			add(wizard, "Maximum frequency [Hz]", fmt.Sprintf("%g or higher", hi), fmt.Sprintf("%g", restimMaxCarrier), hi > restimMaxCarrier)
		}
	}
	add(wizard, "Waveform amplitude [mA]", "your choice: volume 100% drives this current", fmt.Sprintf("%g", restimAmplitudeMA), false)

	if cfg.Intensity == "effective" {
		add(vol, "Nerve time constant [µs]", fmt.Sprintf("%g", cfg.TauUS), fmt.Sprintf("%g", restimTauUS), cfg.TauUS != restimTauUS)
	}
	if byName["pulse_frequency"] != nil {
		// the original's pulse rate is reproduced, so its effect on loudness
		// already is; restim's normalisation would apply it a second time
		add(vol, "Pulse frequency normalization", "off", onOff(restimPulseNorm), restimPulseNorm)
		// pulse_frequency holds the pulse rate, not 1 / burst gap
		add(vol, "Burst gap instead of pulse frequency", "off", onOff(restimBurstGapMode), restimBurstGapMode)
	}

	if cfg.Topology != "joined" {
		// the original box drove A and B equally; placement differences are
		// the listener's to calibrate
		add(four, "A/B/C/D power [dB]", "0, or adjust for your electrode placement", "0", false)
		// stimconv's levels don't compensate for it, so the default is assumed
		add(four, "Center reduction", fmt.Sprintf("%g", restimCenterReduct), fmt.Sprintf("%g", restimCenterReduct), false)
	}
	for _, a := range axes {
		need := fmt.Sprintf("%g – %g", a.Min, a.Max)
		def, change := "-", true
		if d, ok := restimKitRanges[a.Name]; ok {
			def = fmt.Sprintf("%g – %g", d.Min, d.Max)
			change = d.Min != a.Min || d.Max != a.Max
		}
		add(kit, a.Name+" limit min – max", need, def, change)
	}
	return out
}

// RestimHints is the one-paragraph console summary of RestimSettings: the
// device and every setting that differs from restim's default.
func RestimHints(cfg config.Config, axes []*funscript.Axis) string {
	var mode string
	var changes []string
	for _, s := range RestimSettings(cfg, axes) {
		if s.Name == "Device type" {
			mode = s.Need
		} else if s.Change {
			changes = append(changes, fmt.Sprintf("%s: %s (default %s)", s.Name, s.Need, s.Default))
		}
	}
	msg := "restim: use device " + mode
	if len(changes) == 0 {
		return msg + "; restim defaults otherwise"
	}
	return msg + "; change from restim's defaults: " + strings.Join(changes, "; ")
}
