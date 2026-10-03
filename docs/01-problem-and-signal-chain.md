# 1. Problem and signal chain

## 1.1 Source

The input tracks were authored for a home-built, two-channel, audio-driven
muscle-stimulation box (schematic in `reference/original_device/`). The audio
is not meant to be heard. Each sample is effectively an **amplifier output
voltage**. The box:

1. amplifies the stereo audio with a class-D stereo amplifier (TPA3116, 12 V
   supply, bridge-tied load);
2. passes each channel through a 3.9 Ω series resistor;
3. steps it up with a 70 V line ("speaker distribution") transformer, using the
   electrodes on the 0.5 W tap;
4. delivers it to **two isolated electrode pairs**: channel A (left) and
   channel B (right).

Users sometimes tied the two "common" leads together, which turns three
electrodes into one shared circuit. That is the **joined** wiring, as opposed
to **dual** (two independent pairs).

## 1.2 Target

```
audio file ─► stimconv ─► funscripts ─► restim ─► FOC-Stim v4 ─► electrodes
```

- **restim** is a real-time signal generator. It reads one funscript per
  parameter ("axis") and sends parameter updates to the device.
- **FOC-Stim v4** is a current-controlled, transformer-isolated stimulator with
  four outputs in an *any-to-any* topology. It supports a 4-phase mode
  (per-electrode powers e1–e4) and a 3-phase mode (position alpha/beta).

stimconv therefore never produces a waveform. It produces a **description**
of the waveform, in the vocabulary that restim and FOC-Stim understand: carrier
frequency, pulse rate, pulse width, rise time, timing jitter, volume, and
electrode position.

## 1.3 What "similar" means here

The goal is a **similar electrical stimulus**, not a copy of the audio. In
practice this means preserving, in decreasing order of priority:

1. **When** stimulation happens, and how strong it is over time (the envelope).
2. **Rhythm**: burst and gating rates, which determine whether a muscle twitches
   or contracts continuously.
3. **Where** it happens: which electrode pair, and the A/B balance.
4. **Excitation strength per pulse**: charge per phase, which depends on
   carrier frequency and waveform shape, not just amplitude.
5. **Carrier frequency** itself, as far as the target allows.

This ordering is a judgement call: perceptually, rhythm and level dominate
carrier frequency in this range. It drives several defaults (for example,
clamping a 2.4 kHz carrier is accepted, while losing a 20 Hz rhythm is not). See
[10](10-open-issues.md) for why it might be wrong.

## 1.4 Mismatches between source and target

These limits come from the reference code and hardware, not from stimconv.

| Original box | FOC-Stim via restim | Consequence |
|---|---|---|
| Two fully independent channels: own carrier, own envelope, any waveform | **One** carrier and **one** pulse train shared by all electrodes; per-electrode *distribution*, not per-electrode signals | Independent A/B content must be merged, alternated or approximated ([6.5](06-mapping.md#65-ab-strategies-dual)) |
| Continuous waveforms, arbitrary burst lengths | Wavelets of 3–20 carrier cycles, at most 35 ms, at 1–100 pulses/s | Long bursts are shortened, or moved into the volume axis |
| Any carrier the audio can carry (up to 2.4 kHz observed) | Carrier 300–2000 Hz | Higher carriers are clamped |
| Square, sine, anything else | Sine-based wavelets | Waveform shape is converted into an equivalent intensity ([6.3](06-mapping.md#63-intensity)) |
| Voltage drive through a resistor and transformer | Closed-loop current target | The original current is estimated with a circuit model ([3](03-original-hardware-model.md)) |
| Sample-accurate | Parameters updated about every 16 ms and ramped over 30 ms | Nothing faster than about 30–45 ms can be expressed through funscripts |

## 1.5 Processing outline

```
decode ─► circuit model ─► streaming measurements ─► offline features ─► mapping ─► simplify ─► .funscript files
          (optional)       carrier, form factor,     slow/fast envelope,  rendering modes,
                           1 ms peaks, covariance    rhythm descriptors   A/B strategy, intensity,
                                                                          normalisation, ranges
```

Each stage has its own document: [4](04-analysis.md) covers decoding through
features, and [6](06-mapping.md) covers mapping through output.
