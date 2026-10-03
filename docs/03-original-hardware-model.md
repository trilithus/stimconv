# 3. Original hardware model

Option group: `circuit.*` (see [8](08-options-reference.md#original-circuit-model-advanced)).
Code: `internal/circuit`.

## 3.1 Why model the circuit at all

The audio is a voltage command to the amplifier, but nerves respond to
**current** (more precisely, charge per phase). FOC-Stim is current-controlled:
restim's `volume` sets a current amplitude. To compare like with like, the
analysis should see the current that actually flowed through the electrodes,
not the audio voltage. The difference matters because the chain between them
is frequency-dependent:

- the transformer's magnetising inductance attenuates low frequencies and
  makes square-wave tops droop;
- skin and electrode capacitance pass edges more easily than flat tops, so
  square-wave current becomes spiky;
- the skin load is small next to the series resistance once referred through
  the transformer, so the box behaves **close to a current source**. That
  makes the conversion fairly robust to the exact skin values.

## 3.2 Topology

```
audio x ∈ [-1,1]
  └─► amplifier: v_s = AmpVPeak · x
        └─► R1 = SeriesR + WindingR ──┬──────────────┐
                                      │              │
                               L_m (magnetising)   Z_load' (skin, referred to primary)
                                      │              │
                                      └──────────────┘
Z_skin = SkinR + (SkinParR ‖ SkinParC),  referred: R' = R/n², C' = C·n²
electrode current I_skin = I_load(primary) / n
```

Everything is referred to the primary side and integrated with the
**trapezoidal rule** on two states: magnetising current i_m and skin-capacitor
voltage v_c. The update is a precomputed 2×2 matrix step per sample. It is
unconditionally stable, so it works at any audio sample rate.

## 3.3 Parameter derivation

| Parameter | Default | Basis |
|---|---|---|
| `amp_v_peak` | 12 V | A bridge-tied TPA3116 on a 12 V supply swings roughly ±12 V across the load. It is only a scale factor, since normalisation removes absolute scale unless `normalize = abs`. |
| `series_r` | 3.9 Ω | Parts list and schematic. |
| `winding_r` | 1 Ω | Estimate for a small 10 W line transformer's 8 Ω primary. **Assumption.** |
| `turns` | 35 | 70 V line, 0.5 W tap: Z_sec = 70²/0.5 = 9.8 kΩ. With an 8 Ω primary, n = √(9800/8) ≈ 35. |
| `mag_l` | 20 mH | Chosen so that the 8 Ω primary has a low-frequency corner near 60 Hz (L = 8/(2π·60)), typical of small 70 V transformers. **Assumption, not measured.** |
| `skin_r` | 500 Ω | Series electrode/skin resistance, in line with the restim wiki's skin-resistance table (roughly 100–1000 Ω). **Rough.** |
| `skin_par_r` | 5 kΩ | Parallel (stratum corneum) resistance; literature models use kΩ values (Dorgan & Reilly 1999; Keller & Kuhn 2008). **Rough.** |
| `skin_par_c` | 50 nF | Parallel capacitance; literature models use tens of nF for surface electrodes. **Rough.** |

Sanity checks enforced by tests:

- Full-scale current at 800 Hz lies between 30 and 100 mA. The hand estimate is
  12 V / ~5 Ω / 35 ≈ 70 mA.
- Current at 50 Hz is lower than at 800 Hz, because of the magnetising
  inductance roll-off.
- The simulated sinusoidal steady state matches the analytic frequency
  response within 2 % at 50, 200, 650, 900 and 2400 Hz.

## 3.4 Effect on the analysis

With the default skin values, a 660 Hz square wave becomes a current waveform
with a spike at each edge followed by a lower plateau. The time constant of the
referred skin RC (about 0.1–0.25 ms) is a sizeable fraction of the 0.76 ms half
period. Two consequences follow:

- the **peak** current (used for the envelope) is set by the edge spike;
- the measured **form factor** drops from about 1 (ideal square) to about 0.5.

Under `intensity = effective` the product peak × form factor (mean current per
phase) is what counts, so these two effects partly cancel. Under
`intensity = current` they don't. This is the most parameter-sensitive part of
the model: different skin values change how square-wave sections compare with
sine sections. Compare runs with `circuit.enabled = false` to see the
model-free result.

## 3.5 What the model ignores

- Amplifier clipping, output filter and supply sag. Tracks normalised to full
  scale may have clipped in the real box.
- Transformer core saturation and leakage inductance.
- Non-linear and time-varying skin impedance (electroporation, sweat, drying).
  Vargas Luna et al. (2015) show strong current dependence.
- **Joined wiring.** The model is applied per channel. With commons tied, the
  real currents interact through the shared electrode, so for `topology =
  joined` the per-channel model is only an approximation.
