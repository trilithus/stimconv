# 9. Validation

This document separates what has been **verified** (numerically, against
reference implementations or analytic results) from what has only been
**checked for plausibility**, and from what has **not been validated**.

## 9.1 Verified against reference implementations

| Claim | Method | Result |
|---|---|---|
| Layer II decoder is correct | Sample-by-sample comparison with libavcodec (PyAV) on the 53-minute track, from 100 s | Lag 0, 83.5 dB SNR (float rounding) |
| go-mp3 is unreliable on some files | Same comparison on stayh (MPEG-2, 99 % short blocks), plus LAME-encoded MPEG-1 and MPEG-2 test files | ≈ 0 dB SNR on stayh; 82–83 dB on the LAME files (after compensating for encoder delay). This is why Layer III goes to ffmpeg by default. |
| Joined forward model equals restim | Covariances from restim's `ThreePhaseSignalGenerator.generate` for nine positions | Within 10⁻⁶ |
| Joined inversion | Recover those nine positions from covariances at scales 0.01, 1 and 7 | Within 10⁻³ |
| Effective intensity is consistent with restim's TauCalibration | For carriers 300–1500 Hz, the effective intensity of restim's derated sine output is constant | Exact (10⁻⁹) |
| Circuit simulation | Sinusoidal steady state compared with the analytic frequency response | Within 2 % at 50–2400 Hz |

## 9.2 Verified on synthetic signals (unit tests)

The signals are modelled on the observed segments ([2](02-source-material.md)).

| Signal | Expected | Checked |
|---|---|---|
| 650 Hz square, 20 Hz decaying bursts | carrier ≈ 650; form factor ≈ 1 (no circuit model); rhythm 20 ± 1 Hz; attack < 10 ms; pulse mode; pulse width clamped and warned | ✓ |
| 2400 Hz sine, 14 Hz sinusoidal AM | carrier ≈ 2400; form factor ≈ 0.637; rhythm 14 ± 1 Hz; smooth attack; output carrier clamped to 2000 with warnings | ✓ |
| L 800 Hz / R 900 Hz, 0.5–0.7 Hz anti-phase crossfade | correct per-channel carriers; no rhythm; anti-phase slow envelopes; correlation ≈ 0; blend pans e1/e3 | ✓ |
| Joined, 800/900 Hz, constant | IFC mode, pulse rate ≈ 100 Hz | ✓ |
| Joined, L ≡ R and L ≡ −R | α ≈ +1 and α ≈ −1 | ✓ |
| Square versus sine at equal peak, `effective` | volume ratio π/2 | ✓ |
| Auto overlap: 800/900 Hz | multiplexed most of the time; each slot fully on its own pair with its own carrier; no rhythm-conflict warning | ✓ |
| Auto overlap: mono at different levels | ~0 % multiplexed; blend position r = 0.7 | ✓ |
| Majority vote, funscript round trip, RDP endpoints | — | ✓ |

## 9.3 Checked for plausibility on the reference tracks

Feature dumps of `sample_long` match the manual analysis in each segment:

| Segment | Measured |
|---|---|
| 0–12 min | carriers 800 / 900 Hz, no rhythm; e1/e3 anti-phase panning (blend) or 4 Hz alternation (auto) |
| 13–24 min | A rhythm 40 Hz → FOC pulses; B's ~1.9 Hz ramps visible in e3/e4 (blend) or in B's slots (auto) |
| 25–39 min | carrier 660 Hz, rhythm 19.9 Hz |
| 39–53 min | carrier 2400 Hz (clamped to 2000), rhythm stepping through 8–20 Hz, ramps in volume |

`sample_stayh`: carrier 660 Hz, rhythm 19.9 Hz, 100 % pulse mode, 0 %
multiplexed, equal pairs.

Processing time is about 16 s for the 53-minute track.

## 9.4 Not validated

- **Perceptual or physiological equivalence.** No study has compared the
  original box with FOC-Stim playing the converted files, whether by subjective
  rating, EMG or force. This is the central open question.
- **Circuit parameters.** Magnetising inductance, winding resistance and skin
  values are estimates. The transformer and the skin load were not measured.
- **Rhythm thresholds** on material other than the two tracks.
- **Behaviour on the device under load.** One playback session of a converted
  track tripped FOC-Stim's over-current e-stop three times
  ([10.3](10-open-issues.md#103-over-current-e-stops-during-playback)).
