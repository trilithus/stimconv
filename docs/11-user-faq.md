# 11. User FAQ and recommendations

Practical answers for common problems with converted files. Each answer names
the options to change. Their technical meaning is in
[08 Options reference](08-options-reference.md).

**General approach**

- Change one option at a time and write each variant to its own folder (`-o`).
- Save the settings that work with `--emit-config`, and reuse them with
  `--config`.
- Run with `--stats` and read the warnings. Most problems show up there first:
  clamped carriers, shortened bursts, values outside the funscript range, A/B
  conflicts.
- `--dump-features file.csv` shows what the analysis saw (envelope, carrier,
  rhythm) every 10 ms. Open it in a spreadsheet when the output doesn't match
  what you expect from the track.
- Always start with restim's master volume low.

---

## Setup and loading

### restim doesn't pick up the files

restim matches funscripts by name: `<media name>.<axis>.funscript` in the same
folder as the media (or in one of restim's script folders). If you play
`PEP1.mp4` but converted `sample_long.mp3`, rename the files to
`PEP1.volume.funscript` and so on, or convert a copy of the audio that has the
media's name.

### The output feels nothing like what the stats say (wrong carrier, odd pulse lengths)

Almost always this is a **funscript range mismatch**. stimconv writes each axis
against a range (`--range`), and restim reads it against the range in its
funscript kit. If they differ, every value is misread. Check that restim's
funscript kit uses exactly the ranges stimconv printed at the end of the run.
The usual culprits are `frequency` (e.g. 500–2000) and `pulse_width` (e.g.
3–20). See [5.1](05-target-restim-focstim.md#51-funscripts-and-restims-funscript-kit).

### Which restim device mode do I pick?

- `--topology dual` (the default): **FOC-Stim 4-phase**, with A on electrodes
  1/2 and B on 3/4.
- `--topology joined`: **FOC-Stim 3-phase**, for tracks that were used with the
  two common leads tied together.

### What else must match in restim?

With `--intensity effective` (the default), restim's **tau** must equal
`--tau-us` (355 µs), and its **maximum carrier** must equal `--ref-carrier-hz`
(2000 Hz). Otherwise sections with different carriers come out at the wrong
relative strength.

---

## Intensity

### The quiet parts are too weak while the loud parts are too strong

The track's dynamic range is too wide for you. In order of preference:

1. **Lower `--gamma`** below 1 (try 0.7, then 0.5). Volume becomes x^gamma, so
   low levels are raised much more than high ones, while 1 stays 1. This is the
   main "compression" control.
2. **Check `--intensity`.** With `effective`, sections at low carriers or with
   square waves are weighted up, because they carry more charge per phase.
   If the strong parts are the low-carrier or square sections, compare with
   `--intensity current`.
3. **Check the circuit model.** Its skin values make square-wave current spiky,
   which affects levels ([3.4](03-original-hardware-model.md#34-effect-on-the-analysis)).
   Compare with `--circuit=false`, or try a smaller `--skin-par-c` (e.g. 20e-9)
   for fewer spikes.
4. Keep `--normalize p99` (the default) rather than `peak`, so a few very loud
   moments don't push everything else down.

There is no separate compressor or limiter. Gamma is the only dynamics control.

### The quiet parts are fine but the peaks are much too strong

- `--normalize p99` is the default. It already clips the top 1 % to full
  volume, so with `peak` switch back to `p99`.
- Lower restim's master volume, and use `gamma` below 1 so the quiet parts
  don't drop with it.
- Isolated spikes can come from decoding artefacts or the circuit model's edge
  spikes. Try `--circuit=false` to check.

### Everything is too weak or too strong overall

Volume is normalised **per track**: 1.0 means "the loud parts of this track".
Absolute strength is set by restim's master volume and its current limit, not
by stimconv. To make several tracks comparable, use `--normalize abs` with the
same `--ref-level` for all of them.

### The low end is too strong (lows not low enough)

Use `--gamma` above 1 (try 1.5). Low levels then drop more than high ones.

### Faint background noise or residue is played as stimulation

Raise `--silence-db` (e.g. −30 or −25). Anything that far below the track's
maximum becomes silence.

---

## Smoothness and timing

### The signal is spotty, choppy or jittery

Find which kind of chop it is:

| Symptom | Likely cause | What to try |
|---|---|---|
| Regular on/off alternation at about 4 per second, sensation hopping between electrode pairs | A/B **multiplexing** (`--overlap auto` alternates pairs when A and B differ) | `--overlap blend`; or a lower `--mux-hz` (2) for slower, cleaner alternation; or a longer `--auto-window-s` (3) for fewer switches |
| Multiplexing switches on and off irregularly | The independence decision flickers | Raise `--auto-window-s` (2–3); lower `--auto-corr` (0.5) so fewer passages count as independent |
| Short dropouts in quiet passages | The silence gate cuts quiet signal | Lower `--silence-db` (−50) |
| Irregular pulse timing | Measured burst jitter is passed on as `pulse_interval_random` | `--params constant` (no pulse axes), or edit/remove the `pulse_interval_random` funscript; then set randomisation to 0 in restim |
| Carrier or sensation quality jumps around | Carriers of A and B being averaged or alternated | `--params dominant` |
| Slow, wobbly level changes that weren't in the original | A coarse funscript | Lower `--epsilon` (0.2) |
| Stepwise, coarse level changes | Simplification too strong | Lower `--epsilon`; check that `--step-ms` is 10 or less |

### The rhythm feels mushy or blurred

- For rhythms faster than about 12–15 Hz, make sure they are rendered as pulses:
  `--rhythm auto` (default) or `--rhythm pulses`. With `--rhythm volume`, fast
  rhythms are smeared by restim's roughly 30 ms update ramps.
- With `--overlap auto` or `multiplex`, each pair only plays half the time.
  Try `--overlap blend`.

### Distinct pulses feel like a continuous buzz (or the other way round)

That's the **fusion** boundary.

- Raise `--fusion-hz` (e.g. 18) to render more rhythms through volume, as
  separate bumps.
- Lower it (e.g. 8) to render more rhythms as FOC pulse rate.

### A slow pattern in the track isn't recognised as rhythm

Rhythms slower than `--mod-split-hz` (default 3 Hz, i.e. periods over 333 ms)
are treated as level changes. They still come through in volume. To have them
detected as rhythm, lower `--mod-split-hz` (e.g. 1.5). Very slow panning
between A and B may then be read as rhythm.

### Steady sections have a slight pulsing "texture"

Steady and volume-rendered sections are played as a 100 Hz FOC pulse train
(`--continuous-pulse-hz`). Keep it at 100 for the smoothest result. Lower values
make the pulsing perceptible.

---

## Bursts, carriers and sensation quality

### "Bursts are longer than FOC-Stim's pulse width limit"

FOC-Stim pulses can't be longer than 20 carrier cycles or 35 ms, so long bursts
are shortened.

- Fine if the rhythm is what matters.
- `--rhythm volume` keeps the full burst length instead, but loses fast rhythms
  (see above).
- Make sure the `pulse_width` range is wide enough (e.g.
  `--range pulse_width=3:20`, the same in restim). Otherwise widths get clipped
  a second time at 10 cycles.

### "Original carrier exceeds FOC-Stim's maximum"

FOC-Stim can't go above 2000 Hz, so these sections play at 2000 Hz. With
`--intensity effective` their strength is still matched. There's no setting
that restores the original carrier.

### It feels "sharper", "deeper" or "buzzier" than the original

The carrier may differ: clamped, averaged between A and B, or misread through a
range mismatch.

1. Check the ranges first.
2. `--params dominant` stops A/B averaging.
3. To shift the overall feel, you can floor the carrier through the range:
   `--range frequency=1000:2000` (and the same in restim) plays everything
   below 1000 Hz at 1000 Hz.

---

## Electrodes and position

### The sensation is on the wrong electrodes, or spreads between pairs

- Check the wiring: A = electrodes 1/2, B = 3/4 (dual).
- With mono tracks both pairs get equal power. In FOC-Stim's 4-phase mode some
  current then also crosses between the pairs, which the original box never
  did. `--overlap multiplex` keeps the pairs isolated, at the cost of
  alternation.
- If the original used tied commons, use `--topology joined` with restim's
  3-phase mode.

### One channel of the original disappears

`--overlap dominant` drops the quieter channel. Use `blend` or `auto`.

---

## Device problems

### FOC-Stim stops with "Current limit exceeded"

This is FOC-Stim's over-current e-stop. In the cases analysed, one output drew
current that didn't return through the other electrodes, which points to
transformer saturation or a contact problem on that output
([10.3](10-open-issues.md#103-over-current-e-stops-during-playback)). What to
try:

1. **Check the electrode and lead** on the output named in the log (the
   position of the large value in "currents were: …"). Swap two electrodes to
   see whether the problem follows the electrode or stays on the output.
2. **Make sure restim's funscript ranges match** the converted files. A
   mismatch can lower every carrier.
3. **Raise the carrier floor**: `--range frequency=1000:2000` (same in restim).
   Lower carriers drive transformers closer to saturation.
4. **Soften pulse starts**: `--range pulse_rise_time=4:20` (same in restim)
   raises the minimum rise time to 4 cycles. That avoids abrupt starts at the
   cost of slightly softer attacks.
5. **Avoid pair switching**: `--overlap blend`.
6. Reduce restim's master volume or current limit.

Raising a range minimum works as a floor because values below the range are
clipped to its minimum. The run then warns that values were outside the range;
that's expected here.

### The pulse rate on the device is lower than requested

At high power FOC-Stim waits for its boost converter between pulses, which
lowers the actual rate. Lower the volume, or accept it.

---

## Reproducibility

### How do I keep settings that work?

`--emit-config my.json` saves the full configuration. Reuse it with
`--config my.json` (flags on the command line still override it). In the GUI,
save it as a preset.

### How do I compare two variants fairly?

Convert into separate folders, keep restim's settings identical, and use
`--normalize abs` with the same `--ref-level`, so volume differences come from
the option you changed, not from per-track normalisation.
