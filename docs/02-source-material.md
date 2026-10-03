# 2. Source material

The design is driven by what the tracks actually contain, not by assumptions
about stim audio in general. Two reference tracks were measured sample by
sample (zero crossings, burst segmentation, envelope spectra, L/R correlation,
waveform plots) before choosing an approach.

## 2.1 Findings

| Track / segment | Carrier | Fast envelope (rhythm) | Slow envelope | A/B relation |
|---|---|---|---|---|
| `sample_stayh.mp3` (MPEG-2 Layer III, 22.05 kHz, 104 s) | ~650–660 Hz **square** wave | 20 Hz bursts, ~44 ms on: sharp attack, decay to ~30 %, short gap | Peak level swells over seconds | Mono (L ≡ R) |
| `sample_long.mp3` 0–12 min (MPEG-1 **Layer II** despite the extension, 48 kHz, 53 min) | A 800 Hz, B 900 Hz sine | none | A↔B anti-phase crossfade at ~0.7 Hz | Independent, different carriers |
| long, 13–24 min | Both ~800 Hz, clipped sine | A: 40 Hz gating (25 ms period). B: ~1.9 Hz sawtooth ramps (~300 ms ramp, ~200 ms off) | — | Different rhythms per channel, correlation ≈ 0.5 |
| long, 25–39 min | 662 Hz square | 20 Hz bursts, like stayh | Swells, some silent gaps | Mono |
| long, 39–53 min | **2400 Hz** sine | Sinusoidal amplitude modulation, stepped through 8, 10, 12 … 20 Hz in blocks | ~1.5 s sawtooth ramps | Mono |

## 2.2 Conclusions drawn

1. **Every segment is a carrier multiplied by an envelope.** No discrete
   single-pulse trains (TENS-style spikes) appeared. The analysis is therefore
   built around carrier + envelope rather than pulse detection. A track built
   from isolated spikes would be analysed poorly; see
   [10](10-open-issues.md).
2. **The envelope has two time scales**: a fast rhythm (roughly 2–40 Hz) that
   defines twitch versus contraction, and a slow movement (well below 1 Hz up
   to a few Hz) that defines level and A/B panning. This motivates splitting
   the envelope at a configurable frequency (`mod_split_hz`).
3. **Waveform shape varies** (square and sine). Equal amplitudes don't mean
   equal excitation, which motivates the form factor and strength–duration
   weighting.
4. **The carrier range exceeds the target** (2400 Hz versus FOC-Stim's 2000 Hz
   maximum, and restim's default 500–1000 Hz kit range).
5. **Burst lengths exceed the target**: 44 ms at 650 Hz is about 28 cycles,
   while FOC-Stim allows at most 20 cycles or 35 ms.
6. **Independent A/B content is real** (0–24 min of the long track), while most
   material is mono. A single fixed A/B strategy would be wrong for one of the
   two cases, which motivated the adaptive overlap mode.
7. **File extensions can't be trusted.** A `.mp3` that is really Layer II needs
   format sniffing.
8. A crude manual burst count can mislead. B's ramps at 13–24 min first looked
   like 4 Hz. Rhythm is therefore measured by autocorrelation over a window,
   not by counting bursts.

## 2.3 How representative this is

Two tracks from one source are a small sample. The thresholds in
[4](04-analysis.md) (rhythm depth, correlation limits, carrier limits) were
chosen to work on these segments and on synthetic signals modelled on them.
Other material, such as spike trains, carriers below about 330 Hz, or very
irregular rhythms, may need different settings.
