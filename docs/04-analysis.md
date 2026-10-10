# 4. Analysis

Code: `internal/decode`, `internal/analysis`. The analysis runs in two stages:

1. **Streaming** (`Measure`): every audio sample passes once through the
   circuit model and the per-channel trackers. Memory stays flat even for
   hour-long files: a 53-minute track is about 150 million stereo samples.
2. **Offline** (`Derive`): the low-rate arrays (1 kHz and 100 Hz) are
   post-processed into envelopes and rhythm descriptors.

`--dump-features file.csv` writes every derived feature at 100 Hz, for
inspection and tuning.

## 4.1 Decoding

The format is identified from the file's header bytes, not its extension. A
`.mp3` is accepted as MPEG audio only if two consecutive consistent frame
headers are found.

| Format | Decoder | Rationale |
|---|---|---|
| MPEG-1/2 Layer II | Built-in pure-Go decoder | One reference track is Layer II with an `.mp3` extension. The decoder was verified against libavcodec: 0 sample lag, 83 dB SNR (float rounding only). |
| MPEG Layer III | ffmpeg by default | A pure-Go decoder (go-mp3) is bit-accurate on typical encodes, but produced a corrupted waveform on a short-block-heavy MPEG-2 file (stayh: 99 % short blocks). For a drive signal, a wrong waveform that goes unnoticed is the worst outcome. go-mp3 stays available as an opt-in, experimental decoder. |
| FLAC, integer WAV | Built-in | Lossless; no reason to depend on ffmpeg. |
| Anything else | ffmpeg, resampled to 48 kHz stereo float | — |

Mono input is duplicated to both channels, so a mono track is treated as
identical A and B.

**Lossy coding caveat.** MP3 and Layer II are perceptual codecs, designed to
throw away what the ear doesn't notice. That is not necessarily what a nerve
doesn't notice: pre-echo before sharp attacks and smearing of square-wave edges
are examples. The tracks are converted as decoded. The decoded signal is the
best available description of what the original box played, because the box
was fed the same decoded audio.

## 4.2 Circuit model or DC blocker

With `circuit.enabled` (on in the `*-normalized` presets, off by default), each channel passes through the circuit
model ([3](03-original-hardware-model.md)), and all later quantities are in
**amperes of electrode current**. Without it, a 5 Hz first-order high-pass
removes DC, as the transformer would, and quantities are in full-scale audio
units.

## 4.3 Carrier frequency and form factor (half-cycle tracker)

Each channel is split into **half-cycles** at zero crossings with hysteresis:

- the sign changes only when the signal exceeds ±2 % of a running peak, which
  decays with a 2 s time constant. This rejects noise around zero;
- a half-cycle is accepted only if it lasts less than 1/(2·150 Hz) ≈ 3.3 ms
  (carrier ≥ 150 Hz) and its peak exceeds 5 % of the running peak. This
  rejects slow wander and low-level noise.

Per 10 ms hop (100 Hz), with at least two accepted half-cycles:

- **carrier** = number of half-cycles / (2 · total half-cycle duration).
  This is a duration-weighted mean frequency. It is robust to gated signals,
  because only the time *inside* half-cycles counts;
- **form factor** = Σ charge / Σ (peak · duration) = mean |x| ÷ peak over
  the half-cycles. An ideal square wave gives 1, a sine 2/π ≈ 0.637. This is
  what converts waveform shape into charge ([6.3](06-mapping.md#63-intensity)).

Hops with no valid half-cycles (gaps between bursts, silence) report 0.

**Carrier smoothing:** gaps are filled by holding the last valid value (the
leading gap is back-filled with the first valid value), then a running median
over 25 hops (250 ms) is taken. This keeps the carrier steady across burst
gaps and rejects single-hop outliers. The cost is that genuine carrier changes
become visible about 125 ms late.

*Why zero crossings and not an FFT peak:* square waves put strong energy into
harmonics, and amplitude modulation creates sidebands (for example 641/680 Hz
around 660 Hz at 20 Hz AM). Zero crossings give the fundamental period
directly, can be streamed, and also yield the charge per phase that the form
factor needs.

## 4.4 Peak envelope

Every 1 ms the maximum |x| is recorded ("buckets", 1 kHz). The envelope E is
the maximum over the centred three buckets (±1 ms). That bridges half-cycles up
to about 1.5 ms long, so carriers from about 330 Hz upwards give a ripple-free
peak envelope.

*Known gap:* carriers between 150 and about 330 Hz pass the carrier tracker,
but their envelope ripples at twice the carrier frequency. That would show up
as false rhythm.

**Silence gate:** envelope values below the louder channel's overall maximum
times 10^(`silence_db`/20) are set to 0 (default −40 dB, i.e. 1 %). The gate is
relative to the whole track, so quiet passages next to very loud ones can be
gated.

## 4.5 Slow / fast split

```
S = moving_average( moving_max(E, w), w ),   w = 1000 / mod_split_hz  ms
F = min(E / S, 1)                             (0 where S is gated)
```

- **S (slow envelope)**: a centred running maximum over w ms, then a running
  mean over w. The maximum first makes S ride on the burst *peaks* rather than
  the average, so S stays at the burst level through short gaps. With
  `mod_split_hz = 3`, w = 333 ms.
- **F (fast envelope, 0–1)**: the envelope relative to its local peak level.
  Bursts, gating and AM appear as F swinging between about 0 and 1.

S drives level and position. F drives rhythm detection. The split frequency is
a trade-off. Slow panning, like the 0.7 Hz crossfade, must stay in S. But any
rhythm with a period longer than w also stays partly in S and is **not
detected** as rhythm: B's ~1.9 Hz ramps (510 ms period) fall in this category.
Such slow rhythms still reach the output, because non-pulse modes take their
level from the full envelope E ([6.2](06-mapping.md#62-rendering-modes)).

## 4.6 Rhythm descriptors

Computed on F every 200 ms (5 Hz frames), using a 1 s window centred on the
frame.

1. **Activity**: skipped if S is gated for more than half of the window.
2. **Depth** = p95 − p5 of F. If depth < 0.35 the envelope counts as
   *steady*: rate 0, no rhythm.
3. **Autocorrelation** of F − mean, normalised by energy, with an
   unbiased-length correction m/(m−lag). The lags searched run from 10 ms
   (100 Hz) up to w ms (`mod_split_hz`), so the rhythm and slow-envelope bands
   don't overlap.
4. If the highest correlation is below 0.3, there is no rhythm.
5. **Period** = the *smallest* local maximum that reaches at least 85 % of the
   highest. This avoids locking onto sub-harmonics (2× or 3× the period).
   Parabolic interpolation then refines the lag below one millisecond.
6. **Rate** = 1000 / period (Hz).
7. **Duty** = fraction of the window where F > 0.5.
8. **Attack**: at each upward crossing of F through 0.5, walk back to F ≤ 0.1
   and forward to F ≥ 0.9, bounded by one period, then average. This is a
   10→90 % rise time in seconds. Sharp bursts give a few ms; sinusoidal AM
   gives a large fraction of the period.
9. **Jitter** = standard deviation / mean of the intervals between onsets (at
   least three onsets). Regular rhythms give ≈ 0.

**Constants and their basis:** 0.35 depth, 0.3 minimum correlation and 85 %
peak selection were tuned so that the observed segments are classified
correctly: 20 Hz bursts, 40 Hz gating, 14 Hz sinusoidal AM, and no rhythm in a
0.7 Hz crossfade. They are internal constants, not options. See
[10](10-open-issues.md).

## 4.7 A/B covariance

Per 10 ms hop: mean(L²), mean(L·R), mean(R²) of the (current) waveforms,
smoothed over three hops (30 ms). This feeds:

- the **joined** position inversion ([6.6](06-mapping.md#66-joined-topology));
- the **independence** test of `overlap = auto` (normalised correlation
  L·R / √(L²·R²)).

Two carriers that differ by Δf give a correlation that oscillates at Δf and
averages out to about 0 over a hop when Δf · 10 ms is large enough. That is why
the independence test also checks carrier difference explicitly.

## 4.8 Feature summary

| Feature | Rate | Unit | Used for |
|---|---|---|---|
| E (peak envelope) | 1 kHz | A or full scale | level in non-pulse modes |
| S (slow envelope) | 1 kHz | A or full scale | level in pulse mode, gating |
| F (fast envelope) | 1 kHz | 0–1 | rhythm detection |
| carrier | 100 Hz | Hz | FOC carrier, strength–duration, independence, IFC |
| form factor | 100 Hz | 0–1 | strength–duration (charge) |
| rate, duty, attack, jitter, depth | 5 Hz | Hz, 0–1, s, ratio, 0–1 | pulse parameters, rendering mode |
| L/R covariance | 100 Hz | A² | joined position, independence |
