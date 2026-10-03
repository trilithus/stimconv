# 6. Mapping

Code: `internal/mapping`, `internal/physio`, `internal/threephase`.

Mapping turns the features from [4](04-analysis.md) into one time series per
restim axis. Output is computed every `step_ms` (default 10 ms), then
simplified ([6.9](#69-sampling-simplification-and-output)).

It runs in two passes:

1. **Per-channel state** at each step: rendering mode, level, carrier and pulse
   parameters for A and B separately, plus the **A/B independence** flag.
2. **Rendering**: merge A and B into the shared axes according to the topology
   and strategies.

## 6.1 Per-channel state

For each channel at time t:

| Quantity | Source |
|---|---|
| original carrier f | smoothed carrier ([4.3](04-analysis.md#43-carrier-frequency-and-form-factor-half-cycle-tracker)) |
| form factor | half-cycle tracker. If unknown, a sine (2/π) is assumed. |
| rhythm rate | rhythm descriptors ([4.6](04-analysis.md#46-rhythm-descriptors)), 0 = none |
| FOC carrier f_c | f clamped to FOC-Stim's 300–2000 Hz |

## 6.2 Rendering modes

Each channel is in one of these modes:

| Mode | When | Level taken from | Pulse parameters |
|---|---|---|---|
| **pulses** | rhythm present, and `rhythm = pulses`, or `rhythm = auto` with rate ≥ `fusion_hz` | slow envelope **S** at t | rate = rhythm (1–100 Hz); width = duty / rate × f_c cycles (3–20, ≤ 0.035·f_c); rise = attack × f_c cycles (2–10); randomisation = jitter if > 0.05 |
| **volume** | rhythm present, and `rhythm = volume`, or `auto` with rate < `fusion_hz` | **max of E** within ±step/2 | rate = `continuous_pulse_hz`; width = 0.9 · f_c / rate cycles (3–20); rise = 2 |
| **continuous** | no rhythm detected | max of E within ±step/2 | as for volume |
| **ifc** | joined only; see [6.7](#67-interferential-beat-joined) | as continuous | rate = beat |
| **silent** | gated envelope | — | parameters held from the last active step |

**Rationale.**

- *Fast rhythms → pulses.* restim/FOC-Stim can't reproduce a 20–40 Hz
  rhythm through `volume` ([5.4](05-target-restim-focstim.md#54-update-timing-restim--foc-stim)),
  but the FOC pulse train *is* a rhythm generator. A burst of the original
  maps naturally onto one FOC pulse: burst rate → pulse rate, on-time → width,
  attack → rise. Volume then carries only the slow level S, so the burst shape
  is not applied twice.
- *Slow rhythms → volume.* Below the fusion frequency each burst is felt as a
  separate twitch. Its timing and shape matter, and they are slow enough for
  the ~30 ms update path. Meanwhile the FOC pulse train runs at a high rate
  (`continuous_pulse_hz`, default 100 Hz). With width ≈ 0.9 × the pulse
  period, it approximates the original's continuous carrier.
- *No rhythm → continuous*, with volume following the full envelope E. That is
  why slow patterns that fall outside the rhythm band, such as 1.9 Hz ramps,
  still come through.

**Duty definition.** Duty is the fraction of time F > 0.5, so for decaying
bursts width covers the strong part of the burst. In stayh, 44 ms bursts
decaying to about 30 % give a width of about 15 cycles (about 23 ms), not 28.
This is deliberate: the weak tail contributes little excitation. It is open to
challenge.

**Clamping.** Widths above 20 cycles or 35 ms, and rises above 10 cycles, are
clamped and counted. The tool reports the percentage of time affected, and
suggests `rhythm = volume` to keep full burst length at the cost of rhythm
fidelity.

## 6.3 Intensity

Option `intensity`. The level of a channel is either:

- **current**: the raw envelope value (peak electrode current, or peak
  full-scale amplitude without the circuit model);
- **effective** (default): a strength–duration weighted charge per phase,
  relative to a full-amplitude sine at the reference carrier:

```
pw        = 1 / (2 f)                         phase width of the ORIGINAL carrier
efficacy  = pw / (pw + τ)                     fraction of charge that counts (Q_t = Q₀(1 + pw/τ))
effective = peak · formFactor · efficacy(pw) / ( (2/π) · efficacy(1/(2 f_ref)) )
```

with τ = `tau_us`, f_ref = `ref_carrier_hz`.

**Why this form.**

- Excitation depends on charge per phase relative to the threshold charge,
  Q_t = Q₀(1 + pw/τ) (Lapicque/Weiss; restim wiki `nerve-activation.md`).
  peak × form factor is the mean current over a phase, so it captures waveform
  shape: a square wave carries π/2 times the charge of a sine with the same
  peak.
- restim's TauCalibration already rescales volume so that the felt intensity
  does not depend on the FOC carrier, relative to f_max. Expressing the
  original's intensity relative to a sine at f_ref = f_max makes the two
  compensations meet without double counting. **Requirement:** restim's τ
  must equal `tau_us`, and its maximum carrier must equal `ref_carrier_hz`.
- Consistency is enforced by a test. For any FOC carrier f, the effective
  intensity of restim's derated output, Effective(derate(f), sine, f), is
  constant.

**Caveats.** τ = 355 µs is a single value measured for one nerve and electrode
configuration (restim wiki). Chronaxie varies with fibre type (sensory versus
motor, muscle), electrode and placement. Since the formula affects
*relative* levels between carriers and waveform shapes, a wrong τ mainly
distorts the balance between sections with different carriers.

## 6.4 Shared carrier and pulse parameters

Option `params`. FOC-Stim has one carrier and one pulse train, so A and B must
be merged at each step (outside multiplexed stretches):

| Value | Behaviour |
|---|---|
| **weighted** (default) | Start from the louder channel. If both channels are in the same mode, carrier, width, rise and randomisation are averaged, weighted by level. Pulse **rate** always comes from the louder channel, since an average of two rhythms is neither. |
| **dominant** | Everything from the louder channel. |
| **constant** | The carrier and pulse axes aren't written at all, so restim's GUI settings apply. Only volume and position are written. |

A **rhythm conflict** is counted when both channels are active (the quieter one
at ≥ 20 % of the louder), they are not multiplexed, and either their modes
differ or both pulse at rates more than 20 % apart. The tool reports it as a
percentage of time.

## 6.5 A/B strategies (dual)

Topology `dual`: A → e1/e2 and B → e3/e4, played with restim's FOC-Stim
4-phase mode. Option `overlap` decides what happens when both channels are
active:

| Value | Position | Volume | Parameters |
|---|---|---|---|
| **blend** | louder pair 1, quieter pair r = l_min / l_max: (1,1,r,r) or (r,r,1,1) | max(l_A, l_B) | merged per `params` |
| **dominant** | louder pair only: (1,1,0,0) or (0,0,1,1) | max | merged |
| **multiplex** | alternate (1,1,0,0) and (0,0,1,1) at `mux_hz` (first half-period A), whenever the quieter channel is ≥ 5 % of the louder | the active slot's level | the active slot's own carrier and pulse parameters |
| **auto** (default) | multiplex while A and B are *independent* (below); blend otherwise | as multiplex / blend | as multiplex / blend |

**Independence test** (per step, before smoothing): both channels active, the
quieter ≥ 20 % of the louder, and at least one of:

- carriers differ by more than 3 Hz;
- rendering modes differ;
- both in pulse mode with rates more than 20 % apart;
- normalised L/R correlation below `auto_corr` (default 0.7).

The flag is smoothed by a centred **majority vote** over `auto_window_s`
(default 1 s), so the strategy doesn't flicker. A step is multiplexed only if
the smoothed flag is set *and* the quieter channel is still ≥ 20 % of the
louder. Without that second check, the vote would keep multiplexing through a
fade-out, producing silent slots and stray carrier estimates from the nearly
silent channel.

**Rationale.**

- *blend* follows the A/B balance smoothly and never adds rhythm, but both
  pairs share one carrier and rhythm, and some current crosses the pairs
  ([5.3](05-target-restim-focstim.md#53-position-semantics)).
- *multiplex* keeps each pair's own carrier and rhythm and isolates the pairs,
  but halves each pair's duty and adds an artificial alternation at `mux_hz`.
  Each switch is also ramped over about 30–45 ms by restim.
- Most material is mono. Multiplexing identical channels only adds an
  artificial rhythm, which is why *auto* multiplexes only where it buys
  something.

**Multiplex timing.** With restim's 30–45 ms effective ramp, at 4 Hz
(125 ms slots) about 30 % of each slot is transition. At 8 Hz it is about 60 %,
and at ≥ 12 Hz the pairs are never cleanly separated. During transitions,
pulses play at intermediate positions. The default of 4 Hz is a compromise
between clean separation and how audible the alternation itself is.

## 6.6 Joined topology

Topology `joined`: commons tied, written as alpha/beta for restim's FOC-Stim
3-phase mode.

At each step the smoothed L/R covariance (C_LL, C_LR, C_RR) is inverted into
restim's position ([5.3](05-target-restim-focstim.md#3-phase-alphabeta)):

1. M = T⁻¹ C T⁻ᵀ, which is proportional to Q² because Q is symmetric.
2. Q' = √M (closed-form principal square root of a symmetric positive
   semi-definite 2×2 matrix).
3. With tr = Q'₁₁ + Q'₂₂: u = (Q'₁₁ − Q'₂₂)/tr, v = −2Q'₁₂/tr,
   ρ = √(u² + v²), r = 2ρ/(1+ρ), α = u(2 − r), β = v(2 − r).

The overall scale of the covariance cancels, so the position is independent of
loudness. Volume is max(l_A, l_B). When silent, the last position is held.

*Verification:* restim's own `ThreePhaseSignalGenerator.generate` produced
covariances for nine positions. The forward model matches them within 10⁻⁶, and
the inverse recovers them within 10⁻³ at three different scales.

*Caveats:* per-hop covariance of two different carriers oscillates at the beat
frequency and averages towards zero correlation. The position then reflects
the average power ratio, while the beat itself goes to the IFC rule. Also, the
volume choice max(l_A, l_B) ignores that the common electrode carries the sum
of both currents.

## 6.7 Interferential beat (joined)

Option `ifc`. With commons tied and different carriers on A and B, the currents
add in the tissue near the shared electrode and produce an amplitude beat at
|f_A − f_B| (interferential current therapy). With `ifc = beat`, when both
channels are active (quieter ≥ 20 %) and the beat lies between 3 and 100 Hz:

- pulse rate = beat;
- width = 0.5 · f_c / beat cycles (half the beat period);
- rise = 0.25 · f_c / beat cycles (smooth, like the beat's sinusoidal envelope);
- randomisation 0; mode `ifc`.

Example: 800/900 Hz → 100 Hz pulse rate. The beat is not applied in `dual`,
because isolated pairs don't interfere.

## 6.8 Volume normalisation

After rendering, the volume series is normalised:

| `normalize` | Reference |
|---|---|
| **p99** (default) | 99th percentile of the non-silent volume values |
| **peak** | maximum value |
| **abs** | `ref_level`, a fixed level in the analysis unit (A, or full scale) |

volume = min(v / ref, 1)^`gamma`.

- *p99* rather than peak keeps a few transient spikes (edge spikes, decoding
  artefacts) from pushing the rest of the track down. The top 1 % is clipped
  to 1.
- Normalisation is **per track**: volume 1 means "the loud parts of this
  track", not an absolute current. Absolute intensity is set by restim's master
  volume and safety limit. *abs* allows comparisons across tracks.
- *gamma* defaults to 1 (electrically linear). Perceived intensity of electric
  current grows steeply with current, much steeper than for sound, so a gamma
  is an experimental knob, not a correction.

## 6.9 Sampling, simplification and output

- Axes are sampled every `step_ms` (default 10 ms). That is finer than
  restim's ~16 ms update, so step-ms isn't the bottleneck.
- Carrier and pulse parameters are **held through silence**, and backfilled
  before the first sound, so they don't jump around while nothing plays.
- Each value is mapped onto pos 0–100 through the axis range and clipped to it.
  It is then simplified with **Ramer–Douglas–Peucker** using vertical distance
  in position units (`epsilon`, default 0.5). First and last points are always
  kept, and points that round to the same millisecond are merged.
- Axes written: `volume`; `e1`–`e4` (dual) or `alpha`, `beta` (joined); and
  unless `params = constant`: `frequency`, `pulse_frequency`, `pulse_width`,
  `pulse_rise_time`, `pulse_interval_random`.

## 6.10 Warnings

| Warning | Meaning |
|---|---|
| carrier exceeds/below FOC-Stim limit | % of steps where the shared carrier was clamped to 300–2000 Hz |
| bursts longer than the pulse width limit | % of steps where width was clamped (20 cycles / 35 ms) |
| burst attacks exceed the rise time limit | % of steps where rise was clamped (10 cycles) |
| A and B have different rhythms | % of steps with a rhythm conflict ([6.4](#64-shared-carrier-and-pulse-parameters)) |
| `<axis>`: values outside the funscript range | % of samples clipped by the funscript range, with the suggested `--range` |

Each run also reports how much time was spent in each rendering mode, and
(dual) the share of time multiplexed.
