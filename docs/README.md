# stimconv technical documentation

These documents describe **how stimconv converts a stimulation audio track
into restim funscripts, and why**. They cover the signal conversion: the
source signals, the hardware models, the analysis, the mapping onto restim and
FOC-Stim, and the research behind each decision. They do not describe the
application's architecture (GUI, presets, packaging). For the bundled decoder
binary see [FFMPEG.md](FFMPEG.md).

The aim is for a reader to be able to:

- follow every processing step from audio sample to funscript point;
- see which observation, measurement, piece of reference code or published
  finding each decision rests on;
- find the decisions that rest on assumptions, and challenge them.

## Reading order

| # | Document | Contents |
|---|---|---|
| 1 | [Problem and signal chain](01-problem-and-signal-chain.md) | What is being converted into what, what "similar" means, and the mismatches that can't be avoided |
| 2 | [Source material](02-source-material.md) | What the sample tracks actually contain, as measured, and what that implies |
| 3 | [Original hardware model](03-original-hardware-model.md) | The amplifier, transformer and skin circuit, and why it is modelled |
| 4 | [Analysis](04-analysis.md) | Decoding, carrier, form factor, envelopes, rhythm detection, A/B covariance |
| 5 | [Target: restim and FOC-Stim](05-target-restim-focstim.md) | What the output can and can't express, taken from the reference source code |
| 6 | [Mapping](06-mapping.md) | How analysed features become funscript axes: rendering modes, A/B strategies, intensity, normalisation |
| 7 | [Research basis](07-research-basis.md) | The literature each rule relies on, how it is applied, and its limits |
| 8 | [Options reference](08-options-reference.md) | Every option: technical meaning, default, interactions |
| 9 | [Validation](09-validation.md) | What has been verified, how, and what has not |
| 10 | [Open issues and decisions to challenge](10-open-issues.md) | Known limitations, weak assumptions, the over-current trips, future work |
| 11 | [User FAQ](11-user-faq.md) | Practical recommendations: too wide dynamic range, choppy output, device stops, loading problems |

## Conventions

- **A / B**: the two channels of the original box. A is the left audio channel and
  B the right. Each drove one isolated electrode pair.
- **Carrier**: the oscillation frequency of the drive waveform (hundreds of Hz
  to a few kHz).
- **Envelope**: the slowly varying peak amplitude of the carrier.
- **Rhythm**: periodic structure in the envelope (bursts, gating, amplitude
  modulation) in roughly the 3–100 Hz range.
- **FOC pulse**: one FOC-Stim burst of a few carrier cycles with a ramped
  envelope. restim calls it a "pulse"; physically it is a wavelet.
- **Funscript position** (`pos`): integer 0–100 in a `.funscript` file, mapped
  linearly by restim onto a physical range.
- File references such as `restim/stim_math/tau_calibration.py` point to the
  reference submodules in `reference/`. File references such as
  `internal/mapping` point to this repository.
