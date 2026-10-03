// Package physio holds the nerve/muscle rules the mapping relies on.
//
//   - Strength-duration (Lapicque/Weiss; restim wiki nerve-activation.md):
//     threshold charge Qt = Q0 (1 + pw/tau), so a phase of mean current I and
//     width pw has effective intensity I*pw / (pw + tau) (in units of Q0/tau).
//   - restim's TauCalibration derates FOC volume by (f*tau + 0.5)/(fmax*tau + 0.5),
//     i.e. restim's volume already means "intensity equivalent to a sine at
//     fmax". Effective intensities are therefore expressed relative to a
//     full-amplitude sine at RefCarrierHz, and restim handles the FOC carrier.
//   - Interferential current: two carriers fA != fB overlapping in tissue give
//     an amplitude-modulation beat at |fA - fB| (Goats 1990; Ozcan et al. 2004).
//   - Fusion: rhythms below ~12-20 Hz are felt as individual twitches, above it
//     they fuse (tetanic) - used to pick how a rhythm is rendered.
package physio

import "math"

// SineFormFactor is mean(|sin|)/peak.
const SineFormFactor = 2 / math.Pi

// PhaseEfficacy returns pw/(pw+tau): the fraction of charge that counts
// towards excitation for a phase of width pw seconds.
func PhaseEfficacy(pw, tau float64) float64 {
	if pw <= 0 {
		return 0
	}
	return pw / (pw + tau)
}

// Effective returns the strength-duration effective intensity of a waveform
// with the given peak current, form factor (mean|x|/peak over a phase) and
// carrier frequency, relative to a full-amplitude sine of the same peak at
// refCarrier. A result of 1 means "as strong as a sine at refCarrier".
func Effective(peak, formFactor, carrierHz, refCarrierHz, tau float64) float64 {
	if carrierHz <= 0 || peak <= 0 {
		return 0
	}
	pw := 1 / (2 * carrierHz)
	ref := SineFormFactor * PhaseEfficacy(1/(2*refCarrierHz), tau)
	return peak * formFactor * PhaseEfficacy(pw, tau) / ref
}

// RestimDerating reproduces restim's stim_math/tau_calibration.py.
func RestimDerating(maxHz, hz, tau float64) float64 {
	return (hz*tau + 0.5) / (maxHz*tau + 0.5)
}

// Beat returns the interferential beat frequency of two carriers.
func Beat(fA, fB float64) float64 { return math.Abs(fA - fB) }

// Fused reports whether a rhythm at rateHz is perceived as a fused contraction.
func Fused(rateHz, fusionHz float64) bool { return rateHz >= fusionHz }
