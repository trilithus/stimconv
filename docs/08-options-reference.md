# 8. Options reference

Every conversion option, with its configuration key (as in `--config` /
`--emit-config` JSON), its command-line flag, its default, and what it changes
technically. Options marked *advanced* are only shown in the GUI's expert mode.

## Built-in presets

The defaults below are the `[tri-original]` preset. The GUI's preset list and
the CLI's `--preset` flag also offer the others; `--config` and individual
flags override a preset.

| Preset | Topology | Volume |
|---|---|---|
| `[quad-original]` | dual | **original**: `normalize abs` (auto reference), `gamma 1`, no circuit model, `intensity current`. The volume is the recorded peak level; quiet tracks stay quiet. |
| `[quad-normalized]` | dual | **normalized**: `normalize p99`, `gamma 0.85`, circuit model, `intensity effective`. Each track scaled to the full range, quiet passages lifted. |
| `[tri-original]` (default) | joined | original, with the beat between different carriers imitated (`ifc beat`) |
| `[tri-original-smooth]` | joined | original, without the beat imitation (`ifc off`) |
| `[tri-normalized]` | joined | normalized |

The original settings became the default after listening tests: the circuit
model and effective intensity rate square waves as much stronger than sines,
which made tracks that mix both feel unbalanced, while the plain recorded peak
level matched the original box. With `intensity current`, set restim's nerve
time constant (τ) to 0 so it does not derate low carriers; the settings report
says so.

## Output type

### `topology` — `--topology` — default `joined`

| Value | Meaning |
|---|---|
| `dual` (Quad-phase) | Two isolated pairs: A → e1/e2, B → e3/e4. Writes `e1`–`e4`. Use with restim's **FOC-Stim 4-phase** mode. |
| `joined` (Tri-phase) | The two common leads tied together, giving three electrodes. Writes `alpha`, `beta`, obtained by inverting restim's 3-phase model ([6.6](06-mapping.md#66-joined-topology)). Use with **FOC-Stim 3-phase**. |

Choose the topology that matches how the original electrodes were wired. It
determines whether A/B interaction exists physically (joined: shared electrode,
interference) or not (dual).

## Basic

### `overlap` — `--overlap` — default `auto` — dual only

What to do when both channels are active. See [6.5](06-mapping.md#65-ab-strategies-dual).

| Value | Meaning |
|---|---|
| `auto` | Alternate the pairs only while A and B carry independent content; blend otherwise. |
| `blend` | Louder pair at 1, quieter pair at the level ratio. One shared carrier and rhythm; some current crosses the pairs. |
| `dominant` | Only the louder pair. The quieter channel is dropped. |
| `multiplex` | Always alternate the pairs at `mux_hz` while both are active. Each slot plays its own carrier and rhythm. |

### `rhythm` — `--rhythm` — default `auto`

How envelope rhythm is rendered ([6.2](06-mapping.md#62-rendering-modes)).

| Value | Meaning |
|---|---|
| `auto` | Rate ≥ `fusion_hz` gives FOC pulses (rate, width, rise from the bursts); slower rhythms go into the volume axis. |
| `pulses` | Every detected rhythm becomes FOC pulse parameters. Width and rise are clamped to FOC-Stim limits. |
| `volume` | Every rhythm goes into the volume axis over a high-rate pulse train. Keeps full burst length, but rhythms faster than about 12–15 Hz are smeared by restim's ~30 ms update ramps. |

### `intensity` — `--intensity` — default `current`

What the volume axis measures ([6.3](06-mapping.md#63-intensity)).

| Value | Meaning |
|---|---|
| `effective` | Strength–duration weighted charge per phase, relative to a sine at `ref_carrier_hz`. Accounts for waveform shape and carrier. Requires restim's τ = `tau_us` and maximum carrier = `ref_carrier_hz`. |
| `current` | Peak electrode current (or peak audio amplitude without the circuit model). Ignores carrier and waveform shape. |

### `normalize` — `--normalize` — default `abs`

How the volume series is scaled ([6.8](06-mapping.md#68-volume-normalisation)).

| Value | Meaning |
|---|---|
| `p99` | The 99th percentile of non-silent values maps to 1; the top 1 % clips. |
| `peak` | The maximum maps to 1. |
| `abs` | Divide by `ref_level`, so volume is comparable across tracks. |

### `gamma` — `--gamma` — default `1`

Exponent applied after normalisation: volume = x^gamma. 1 is linear in the
measured quantity. Above 1, low levels become quieter and the dynamics more
pronounced. Below 1, low levels are raised. It must be > 0.

## Rhythm and A/B mixing (advanced)

### `mux_hz` — `--mux-hz` — default `4` — dual only

Alternation rate for `multiplex` and `auto`. A full cycle is A then B, each for
half the period. restim ramps every position change over about 30–45 ms, so
above about 4 Hz an increasing share of each slot is transition
([6.5](06-mapping.md#65-ab-strategies-dual)). Lower values separate the pairs
more cleanly, but the alternation becomes a slow rhythm of its own.

### `auto_corr` — `--auto-corr` — default `0.7` — dual only

For `overlap = auto`: a normalised L/R waveform correlation below this counts
as independent content. 1 would treat nearly everything as independent; 0 would
leave only the carrier and rhythm tests.

### `auto_window_s` — `--auto-window-s` — default `1` — dual only

Length of the centred majority vote that smooths the independence decision.
Longer windows give fewer, longer multiplexed stretches; shorter windows react
faster but can flicker.

### `params` — `--params` — default `weighted`

How differing A/B carrier and pulse parameters are merged ([6.4](06-mapping.md#64-shared-carrier-and-pulse-parameters)).

| Value | Meaning |
|---|---|
| `weighted` | Louder channel's mode and rate. Carrier, width, rise and randomisation are level-weighted when both channels are in the same mode. |
| `dominant` | All parameters from the louder channel. |
| `constant` | No carrier or pulse axes are written; restim's settings apply. |

### `fusion_hz` — `--fusion-hz` — default `12`

For `rhythm = auto`: the boundary between rendering a rhythm as FOC pulses (at
or above) and as volume modulation (below). It is based on muscle twitch fusion
([7.4](07-research-basis.md#74-muscle-fusion-and-fatigue)) and on restim's
update resolution.

### `continuous_pulse_hz` — `--continuous-pulse-hz` — default `100`

FOC pulse rate used in continuous and volume modes. Width is set to 0.9 × the
pulse period, so the train approximates a continuous carrier. 100 Hz is FOC-Stim's
maximum. Lower values add a perceptible pulse rhythm of their own.

### `ifc` — `--ifc` — default `beat` — joined only

| Value | Meaning |
|---|---|
| `beat` | When A and B have different carriers and are both active, the beat \|f_A − f_B\| (3–100 Hz) becomes the pulse rate, with half-period width and smooth rise ([6.7](06-mapping.md#67-interferential-beat-joined)). |
| `off` | Ignore interference; parameters are merged per `params`. |

## Intensity and envelope (advanced)

### `tau_us` — `--tau-us` — default `355` — only with `intensity = effective`

Nerve chronaxie τ in µs, used in the strength–duration weight pw/(pw + τ).
**It must equal restim's tau setting**, otherwise restim's carrier compensation
and stimconv's weighting disagree.

### `ref_carrier_hz` — `--ref-carrier-hz` — default `2000` — only with `intensity = effective`

The carrier at which restim's TauCalibration applies no derating: FOC-Stim's
maximum carrier, clipped by restim's safety limits. If restim's maximum carrier
is set lower, set this to the same value.

### `ref_level` — `--ref-level` — default `0` (auto) — only with `normalize = abs`

The level that maps to volume 1, in the analysis unit: effective-intensity
units (`effective`), amperes (`current` with the circuit model), or full scale
(no circuit model). `0` picks the level of full-scale audio: a full-scale
square wave at FOC-Stim's lowest carrier (300 Hz), sent through the circuit
model and measured like the analysis does (without the circuit model, the
audio itself: 1 as `current`, about 3.1 as `effective`). A
square is the strongest signal the audio can carry, and the lowest carrier
counts as strongest in effective intensity, so no playable full-scale track
exceeds it. With the circuit model's default parameters that is about 0.13 (`effective`) or 92 mA
(`current`). A track that peaks at half scale then reaches about half volume
(before `gamma`).

### `silence_db` — `--silence-db` — default `-40`

Envelope values more than this many dB below the louder channel's overall
maximum count as silence. Silence gates position updates, holds parameters and
gives volume 0. Raise it (e.g. −30) to suppress noise and decoding residue;
lower it to keep very quiet passages.

### `mod_split_hz` — `--mod-split-hz` — default `3`

Frequency that divides the envelope into slow (level and position) and fast
(rhythm) parts ([4.5](04-analysis.md#45-slow--fast-split)). The window is
w = 1000 / mod_split_hz ms, and the rhythm search covers periods from 10 ms up
to w. Lower values detect slower rhythms but move slow panning into the rhythm
band; higher values do the opposite.

## Output sampling (advanced)

### `step_ms` — `--step-ms` — default `10`

Time step at which axes are computed before simplification. Below restim's
~16 ms update interval, so it doesn't limit fidelity. Larger steps give
coarser output.

### `epsilon` — `--epsilon` — default `0.5`

Ramer–Douglas–Peucker tolerance in funscript position units (0–100). Larger
values give smaller files but drop small changes. 0 disables simplification.

## Original circuit model (advanced)

See [3](03-original-hardware-model.md). All circuit parameters except
`enabled` only apply while the model is enabled.

| Key | Flag | Default | Meaning |
|---|---|---|---|
| `circuit.enabled` | `--circuit` | `false` | Analyse estimated electrode current instead of the audio waveform. Without it, a 5 Hz DC blocker is applied instead. |
| `circuit.amp_v_peak` | `--amp-v-peak` | 12 V | Amplifier output voltage at full-scale audio. Pure scale. |
| `circuit.series_r` | `--series-r` | 3.9 Ω | Series resistor between amplifier and transformer. |
| `circuit.winding_r` | `--winding-r` | 1 Ω | Primary winding resistance. |
| `circuit.turns` | `--turns` | 35 | Secondary:primary voltage ratio (70 V line, 0.5 W tap). |
| `circuit.mag_l` | `--mag-l` | 0.02 H | Primary magnetising inductance; sets the low-frequency roll-off. |
| `circuit.skin_r` | `--skin-r` | 500 Ω | Series skin/electrode resistance. |
| `circuit.skin_par_r` | `--skin-par-r` | 5000 Ω | Parallel skin resistance. |
| `circuit.skin_par_c` | `--skin-par-c` | 50 nF | Parallel skin capacitance. Strongly affects how spiky square-wave current becomes. |

## Funscript ranges (advanced)

Key `ranges.<axis>.min` / `.max`. Flag `--range axis=min:max` (repeatable).

Defines the physical value written as pos 0 and pos 100 for each axis. **The
same range must be set in restim's funscript kit**, otherwise restim misreads
the file ([5.1](05-target-restim-focstim.md#51-funscripts-and-restims-funscript-kit)).
Defaults equal restim's kit defaults:

| Axis | Default | Note |
|---|---|---|
| `volume` | 0–1 | |
| `e1`–`e4` | 0–1 | dual only |
| `alpha`, `beta` | −1–1 | joined only |
| `frequency` | 500–1000 Hz | widen to 500–2000 (or 300–2000) for high-carrier tracks |
| `pulse_frequency` | 0–100 Hz | |
| `pulse_width` | 4–10 cycles | widen to 3–20 to keep long bursts |
| `pulse_rise_time` | 2–20 cycles | FOC-Stim caps at 10 |
| `pulse_interval_random` | 0–1 | |

Values outside the range are clipped and reported with a suggested range.
Ranges don't override FOC-Stim's own limits, which are always enforced on
carrier, pulse rate, width and rise.

## Output

| Flag | Meaning |
|---|---|
| `-o dir` | Output folder. Default: `<output name>` next to the input. |
| `--name name` | Output name instead of the input name without its extension, e.g. `--name PEP11` turns `PEP11.fr.mp3` into `PEP11.alpha.funscript`. It also names the default folder and, with `-o`, the `.md` report. In the GUI, tick **Output name** below the output folder. |
| `--preset-suffix` | Name the default folder `<output name>.<preset>` instead of `<output name>`; the preset is the `--config` file name or `default`. In the GUI, tick **Add preset to folder name**. |

## Decoding

| Flag | Meaning |
|---|---|
| `--native-mp3` | Decode MPEG Layer III with the pure-Go go-mp3 decoder instead of ffmpeg. Experimental: it corrupted a short-block-heavy MPEG-2 file during validation. |
| `--ffmpeg path` | Use a specific ffmpeg binary. |

## Diagnostics

| Flag | Meaning |
|---|---|
| `--stats` | Per-axis min/mean/max and point counts, time per rendering mode, multiplexed share, and the estimated intended wiring (3-phase or 4-phase, also shown on `--dry-run`). |
| `--dump-features file.csv` | Per-hop (100 Hz) features for both channels: env, slow, fast, carrier, form factor, rhythm, duty, attack, jitter, depth, and covariance (including `cov_lq`, the quadrature part used for the A/B phase). |
| `--emit-config file.json` | Save the effective configuration, so the run can be reproduced with `--config`. |
