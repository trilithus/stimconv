# 5. Target: restim and FOC-Stim

Everything in this document was taken from the reference **source code** in
`reference/restim` and `reference/FOC-Stim`, at the commits in the submodules:
restim v1.67 and FOC-Stim firmware v2.0.3. `reference/restim` is a fork that adds
Jellyfin commits on top of the v1.67 release; they don't change anything
described here, so this also describes stock v1.67. restim v1.67 only accepts
firmware 2.x, and v1.66 only 1.x, so the two have to be updated together.
FOC-Stim has had four hardware versions and its `docs/` folder is partly stale,
so the v4 code paths are treated as authoritative: `src/main_focstim_v4.cpp`,
`src/bsp/config_g473re_focstim_v4.h` and `src/signals/fourphase_*.cpp`.

## 5.1 Funscripts and restim's funscript kit

restim matches files named `<media>.<axis>.funscript` to axes
(`restim/funscript/collect_funscripts.py`). Each axis maps funscript position
0–100 **linearly** onto a range configured in its *funscript kit*
(`restim/qt_ui/models/funscript_kit.py`).

| Axis | Kit default range | FOC-Stim v4 axis limit |
|---|---|---|
| `volume` | 0–1 | (scales 0–0.2 A × safety limit) |
| `e1`–`e4` | 0–1 | 0–1 |
| `alpha`, `beta` | −1…1 | −1…1 |
| `frequency` (carrier) | 500–1000 Hz | 300–2000 Hz |
| `pulse_frequency` | 0–100 Hz | 1–200 Hz, in practice limited by the 5 ms pause ([5.2](#52-how-foc-stim-plays-a-pulse)) |
| `pulse_width` | 4–10 cycles | 3–20 cycles (and ≤ 35 ms) |
| `pulse_rise_time` | 2–20 cycles | 2–10 cycles |
| `pulse_interval_random` | 0–1 | 0–1 |

**The range used when writing a funscript must equal the range configured in
restim**, or every value is misread. stimconv writes against the ranges in its
configuration (default: the kit defaults) and warns when data fall outside
them. Real tracks routinely exceed the kit defaults for `frequency` and
`pulse_width`. Values outside the funscript range are clipped to pos 0 or 100.

## 5.2 How FOC-Stim plays a pulse

From the v4 main loop:

- The device plays **one pulse at a time**. At the start of each pulse it reads
  every axis once: carrier, pulse rate, width, rise, randomisation, position,
  amplitude.
- Pulse active time = width / carrier. Width is capped at 0.035 × carrier,
  i.e. 35 ms.
- Pause = max(5 ms, 1/pulse_rate − active), multiplied by a random factor in
  [1 − r, 1 + r] where r = `pulse_interval_random`. The 5 ms floor applies
  before the random factor, and it arrived with firmware 2.0 (the floor was 0
  before). So the highest rate is 1/(active + 5 ms), which depends on the
  carrier: at the 3-cycle minimum width it is 91 Hz at a 500 Hz carrier, 125 Hz
  at 1 kHz and 154 Hz at 2 kHz. A pulse train can't be fully continuous any
  more: 20 cycles at 1 kHz is 20 ms on and 5 ms off, 80 % duty at 40 Hz.
- restim applies the same rule first: it clips the pause, recomputes
  `pulse_frequency` from rate, width and carrier, and sends the result. The
  firmware then arrives at the same pause.
- The next pulse also waits until the boost converter is ready, so at high
  power the actual pulse rate can fall below the requested one.
- Each pulse has a randomised start phase. Polarity flipping is present in the
  code but commented out.
- If no axis command arrives for 4 s, playback stops. That is restim's job to
  prevent.

A FOC pulse is therefore a **wavelet**: `width` carrier cycles with a
raised-cosine-like ramp of `rise` cycles at both ends. The rise is capped at
half the width.

## 5.3 Position semantics

### 4-phase (e1–e4)

`fourphase_math.cpp`:

1. The position vector is **constrained** so that its maximum is exactly 1 and
   its three smallest components sum to at least 1.
2. It is converted into electrode currents, taking calibration into account.
3. The result is **intensity-normalised** by a p-norm of the two largest
   components (`reduction_in_center`).
4. It is projected onto complex points: electrodes 1–2 split around a point
   *p*, and electrodes 3–4 around −*p*. The currents always sum to zero.

Consequences for stimconv:

- **e1–e4 express distribution, not loudness.** All loudness belongs in
  `volume`.
- **(1,1,0,0)** gives *p* = 0, so current flows only between electrodes 1 and
  2 (pair A). **(0,0,1,1)** does the same for pair B.
- **(1,1,1,1)** gives *p* = 1. Electrodes 1 and 2 sit at ±60° around *p*, so
  part of the current flows between the pairs (1/2 ↔ 3/4). Driving both
  pairs equally at once is therefore **not** two isolated channels. Some
  current crosses the pairs, which the original box never did.

### 3-phase (alpha/beta)

restim's stereostim model (`stim_math/threephase.py`):

```
[L, R] = T · Q · [cos ωt, sin ωt]
T = (potential_to_channel @ ab_transform)[:2,:2] / √3 = [[√3/2, −1/2], [√3/2, 1/2]]
Q = ½ · [[2 − r + α, −β], [−β, 2 − r − α]],   r = |(α, β)|
```

Here L and R are the potentials of the two outer electrodes relative to the
*neutral* electrode. That is exactly the joined wiring, with the tied commons
as neutral: L ≡ R gives α = +1 (current into the commons), and L ≡ −R gives
α = −1 (current between the two hot electrodes, none in the commons).

## 5.4 Update timing (restim → FOC-Stim)

`restim/device/focstim/proto_device.py`:

- every **~16 ms** (60 Hz Qt timer), the algorithm's parameters are evaluated
  and changed values are sent;
- each value is sent as `move_to(value, interval = 30 ms)`, which FOC-Stim's
  `SimpleAxis` interpolates linearly over 30 ms;
- funscripts are read with linear interpolation.

A step in a funscript therefore reaches the device as a ramp of about 30–45 ms.
Changes faster than that, such as a 40 Hz rhythm written into `volume` or
fast A/B alternation, are smeared. This is why fast rhythms are rendered as
pulse parameters, and why `mux_hz` should stay ≤ 4.

Over WiFi the ESP32 sits in this path (`reference/FOC-Stim-esp32`). Since v1.1.0
it sends the PC at most 100 TCP packets per second, which delays replies and
device statistics by up to 10 ms. Commands to the device aren't delayed, and the
UART behind it stays at 115200 baud, so the limit on axis updates per second is
unchanged. The main firmware doesn't request the ESP32's version, so neither
restim nor stimconv can see it.

## 5.5 Intensity handling in restim

`device/focstim/fourphase_algorithm.py` and `threephase_algorithm.py`:

- amplitude = volume × the user's safety limit (`waveform_amplitude_amps`, at
  most 0.2 A);
- **TauCalibration** (`stim_math/tau_calibration.py`): volume is multiplied by
  (f·τ + 0.5) / (f_max·τ + 0.5), where f_max is FOC-Stim's maximum carrier
  clipped by the user's limits and τ is a restim setting (default 355 µs). It
  follows from the strength–duration relation Q_t = Q₀(1 + pw/τ). In effect,
  restim's `volume` means "as strong as a sine of that amplitude at f_max";
- optional pulse-frequency calibration (`PulseFrequencyCalibration`) adds a
  small empirical correction of a few percent between 10 and 100 Hz.

stimconv's `intensity = effective` is built to be exactly consistent with
TauCalibration. A test proves it: restim's output at any carrier, weighted by
stimconv's effective-intensity formula, is constant ([6.3](06-mapping.md#63-intensity)).

## 5.6 Safety mechanisms relevant to the signal

`fourphase_model.cpp`, `output_limits.cpp`:

- **Open-loop voltages per pulse.** Each output's voltage is precomputed from
  the commanded currents and a **learned impedance model** z1–z4:
  v = p·z + N, with N chosen so that the voltages sum to zero. The magnitude of
  z is corrected by gradient descent *between* pulses, scaled by |p|, so an
  output that isn't driven doesn't learn. The phase angle is only updated
  above 20 mA.
- **Volt-seconds limit**: 1100 µV·s (transformer saturation). The pulse
  amplitude is scaled down if |z|·|p| / (2πf) exceeds it. Since firmware 2.0 the
  4-phase path checks all four electrodes; before that it skipped electrode 4.
- **Drive-voltage limit** from the boost converter.
- **E-stop** (`OUTPUT_OVER_CURRENT`): trips if any output's measured
  driver-side current exceeds the commanded driving current
  (body current × transformer current ratio, 6.66 for the 42TL004) plus a
  0.12 A margin, capped at the measurable range of about 1.9 A.

These mechanisms explain why abrupt pulse starts, low carriers and switching
pairs on and off are risky. See [10.3](10-open-issues.md#103-over-current-e-stops-during-playback).

## 5.7 What can't be expressed

- Two simultaneous pulse trains with different carriers or rates.
- Per-pulse position changes. Funscripts are sampled every ~16 ms and ramped.
- Arbitrary waveform shapes: only sine-based wavelets.
- Bursts longer than 20 cycles or 35 ms, or carriers outside 300–2000 Hz.
- A truly continuous carrier, or less than 5 ms between pulses.
