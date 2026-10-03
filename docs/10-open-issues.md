# 10. Open issues and decisions to challenge

## 10.1 Decisions most worth challenging

| Decision | Why it was made | Why it might be wrong | How to test |
|---|---|---|---|
| Priority order: envelope > rhythm > place > charge > carrier ([1.3](01-problem-and-signal-chain.md#13-what-similar-means-here)) | Rhythm and level dominate how muscle stimulation feels; FOC-Stim can't do everything | Carrier frequency changes sensation quality (depth, "sharpness") more than assumed | A/B comparisons that hold rhythm fixed and vary only the carrier |
| Fast rhythm → FOC pulse rate; one burst = one FOC pulse | FOC's pulse train is a rhythm generator; volume can't carry > ~15 Hz | An original burst is many carrier cycles at constant amplitude, while a FOC pulse is a short wavelet. Burst *content* (e.g. 28 cycles) is lost beyond 20 cycles | Compare `rhythm = pulses` with `volume` on burst tracks |
| Width from duty at F > 0.5 | The weak tail of a decaying burst contributes little | For decaying bursts the tail may matter for perceived duration | Use a 0.3 threshold instead, or charge-weighted duty |
| `fusion_hz = 12` | Twitch/fusion boundary, and the limit of volume-axis resolution | Fusion depends on muscle; 12 Hz may be too low for small muscles | Sweep 8–20 Hz |
| Strength–duration with a single τ = 355 µs | Matches restim's model exactly | τ varies by tissue; the relation is defined at threshold | Measure thresholds at several carriers on the target placement |
| Circuit model on by default | The analysis should see current, not voltage | Default skin values make square waves spiky (form factor about 0.5), which shifts square/sine balance | Compare runs with and without the model; measure real skin impedance |
| Volume = max(A, B) | Simple, never exceeds either channel | In blend the quieter pair also receives current; in joined, the common electrode carries the sum | Sum or RMS variants |
| p99 normalisation per track | Robust to spikes | Levels aren't comparable between tracks | `normalize = abs` with a fixed reference |
| Auto overlap: multiplex independent content at 4 Hz | Keeps each pair's carrier and rhythm, isolates pairs | Adds a 4 Hz alternation and halves duty; transitions are 30 % of each slot | Compare against `blend` |
| Mono → equal positions (1,1,1,1) in dual | Both pairs were driven identically | FOC-Stim's (1,1,1,1) also sends current across the pairs, which the original never did | Compare `multiplex` with very low `mux_hz`, or physical rewiring |

## 10.2 Known technical limitations

- **Carriers between 150 and about 330 Hz** pass the carrier tracker, but their
  envelope ripples at twice the carrier frequency. That could be detected as
  false rhythm ([4.4](04-analysis.md#44-peak-envelope)).
- **Rhythms slower than the split window** (< `mod_split_hz`, e.g. 1.9 Hz) are
  not *detected*. They still reach volume through the full envelope, but are
  reported as "continuous".
- **Spike/pulse-train sources** (TENS-like single pulses) are not modelled. The
  analysis assumes carrier × envelope.
- **Low-level carrier estimates** at fade-ins are noisy. Single values such as
  385 Hz can appear, though they are clipped to the funscript range.
- **Rhythm thresholds** (depth 0.35, correlation 0.3, 85 % peak) are tuned on
  two tracks and are not options.
- **Continuous mode adds a 100 Hz pulse structure** that the original didn't
  have, with no perceptual compensation.
- **Joined + circuit model**: the circuit is modelled per channel, which ignores
  how currents interact through the shared electrode.
- **Lossy decoding artefacts** (pre-echo, smeared edges) pass straight through.

## 10.3 Over-current e-stops during playback

During one playback of the converted `sample_long` with auto overlap, FOC-Stim
v4 (firmware 1.3.2) stopped three times with "Current limit exceeded":

```
currents were: -0.001303 -0.045350 -0.286370 0.001344
currents were: -0.001345 -0.042063 -0.293027 0.001212
currents were: -0.001802 -0.000024 -0.445609 0.001123
```

**Analysis (from the firmware source).**

- The check (`fourphase_model.cpp`) trips when any output's measured
  driver-side current exceeds the commanded driving current plus 0.12 A.
- In all three events only **output 3** carries significant current. Current
  that flows through the body must return through another electrode, so the
  four currents should roughly sum to zero. Here they don't, by 0.3–0.45 A.
  That points to **magnetising current in output 3's transformer, i.e. core
  saturation**, rather than current through the skin. A fault on output 3
  (lead, connector, electrode contact) can't be ruled out.
- FOC-Stim computes each pulse's voltages open-loop from a learned impedance
  model and limits volt-seconds with that same model. If the model is wrong,
  the limit doesn't protect against saturation.
- Separately, the 4-phase path's volt-seconds check uses only electrodes 1–3.
  That is a firmware bug, but it doesn't explain these events, because output 3
  is included.

**Signal features that plausibly raise the risk.** Each is a hypothesis.

1. **Low carriers** (650–900 Hz for most of the track): volt-seconds per
   half-cycle scale with 1/f.
2. **Abrupt pulse starts**: rise time 2 cycles (the minimum) in continuous and
   volume modes and for sharp attacks. An abrupt start can push the core
   towards saturation in the first cycle, much like transformer inrush.
3. **Pair switching** under auto/multiplex: pair 3–4 goes from off to full
   every 125 ms, and the carrier changes at each switch. Each switch is an
   abrupt start, and the impedance model of an idle pair doesn't update.
4. **Range mismatch**: the logged pulse timings fit restim reading the files
   with its default kit ranges (carrier 500–1000 Hz, width 4–10) rather than
   the ranges they were written with. That would lower every carrier further
   (e.g. 660 → about 553 Hz).

**Not yet established:** the exact media time of each trip; whether the trip
follows the electrode or the output when electrodes 3 and 4 are swapped; and
restim's actual funscript-kit settings during the session.

**Possible mitigations in the conversion** (not implemented):

- a carrier floor (e.g. ≥ 1000 Hz);
- a rise-time floor (e.g. ≥ 4–5 cycles), the firmware default being 5;
- softer pair switching, for example ramping positions or keeping a small
  minimum level on the idle pair so its impedance model stays trained;
- a warning when output is written with non-default ranges, reminding the user
  to configure restim.

## 10.4 Ideas for future work

- **"Split" parameter strategy**: keep the louder channel's pulse rate, and
  write the quieter channel's fast envelope into its pair's position values
  (useful up to about 10–15 Hz).
- **Per-pulse A/B interleaving.** Would need restim or firmware support:
  funscripts can't address individual pulses.
- **Two simultaneous pulse trains.** The four-output hardware could in
  principle superpose two pair signals, since each sums to zero. It would need
  firmware that synthesises and limits a summed waveform, and a protocol
  extension.
- **Charge-weighted duty and burst-shape-aware width.**
- **A spike-train analysis path** for TENS-style sources.
- **Measured circuit parameters** (transformer inductance, skin impedance at
  the electrode sites).
- **A perceptual or EMG study** comparing the original box with the converted
  output.
