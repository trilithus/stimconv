# 7. Research basis

For each area this document gives the finding relied on, how stimconv applies
it, and its limits, so each rule can be checked against its source. The
literature was gathered from Google Scholar searches, plus the restim wiki
(`reference/restim/restim.wiki`), which contains its author's own measurements.

## 7.1 Strength–duration and chronaxie

**Finding.** The charge needed to excite a nerve grows with pulse width:
Q_t = Q₀ (1 + pw/τ), or equivalently I_t = I₀ (1 + τ/pw), where τ is the
chronaxie (Lapicque; Weiss). restim's wiki measures τ ≈ 355 µs and a rheobase
of about 11 mA for its own setup (`nerve-activation.md`). Merrill, Bikson &
Jefferys (2005) review charge per phase as the governing quantity for both
efficacy and safety.

**Applied as.** `intensity = effective` weights each phase's charge by
pw/(pw + τ) ([6.3](06-mapping.md#63-intensity)). restim's TauCalibration uses
the same relation, so the two match exactly.

**Limits.**

- A single τ is an idealisation. Chronaxie differs between sensory and motor
  fibres, muscle, electrode sizes and depths.
- The relation describes threshold, not suprathreshold perceived intensity.
  Using it to equalise strong stimulation is an extrapolation.
- It treats each half-cycle as an isolated phase. With carriers this fast,
  charge balance and accommodation also play a role.

## 7.2 Kilohertz-frequency alternating current (burst-modulated)

**Findings.**

- Ward (2009, *Physical Therapy*) reviews burst-modulated kHz AC. Burst
  duration, not carrier frequency alone, governs torque and discomfort. Short
  bursts of about 2–4 ms are the most efficient and comfortable. Carriers of
  about 1–2.5 kHz give the most torque, and around 4 kHz is the most
  comfortable.
- Ward, Robertson & Ioannou (2004) and Ward & Chuen (2009) cover the effects of
  duty cycle and burst duration on torque and on sensory, motor and pain
  thresholds.
- Laufer & Elboim (2008) compare burst frequency and duration with pulsed
  currents.
- Modesto et al. (2023 systematic review; 2024 randomised crossover trial)
  report the highest evoked torque at 1–2.5 kHz with about 50 % burst duty
  cycles.
- "Russian current": a 2.5 kHz carrier gated at 50 Hz.

**Applied as.**

- The source material is itself burst-modulated AC: the 2.4 kHz section with
  8–20 Hz AM is close to Russian current. That supports treating bursts as the
  unit of stimulation and mapping each burst to one FOC pulse.
- Long bursts that exceed FOC-Stim's limits are shortened (`pulses`), or moved
  into a high-rate pulse train modulated by volume (`volume`).

**Limits.** These studies use large muscles (quadriceps, wrist extensors),
clinical electrodes and 1–10 kHz carriers. The tracks use 650–2400 Hz carriers
with different electrodes and placement, so the findings support the
*structure* of the mapping, not specific numbers.

## 7.3 Interferential current

**Findings.**

- Goats (1990) and Ozcan, Ward & Robertson (2004): two medium-frequency
  currents that cross in tissue produce amplitude modulation at their
  difference frequency (the beat). Its effect depends on that modulation.
- Opančar et al. (2025, *Nature Communications*) show that temporal
  interference and direct kHz stimulation of peripheral nerves share one
  biophysical mechanism.

**Applied as.** `ifc = beat` in joined topology turns the beat of different
carriers into the FOC pulse rate ([6.7](06-mapping.md#67-interferential-beat-joined)).

**Limits.**

- It is unknown whether the original authors intended a beat. In the 800/900 Hz
  section the beat (100 Hz) exists physically only with joined commons.
- With true IFC the beat's depth depends on where the two current paths
  overlap. A uniform pulse rate is a simplification.

## 7.4 Muscle fusion and fatigue

**Finding.** Motor-unit twitches fuse into a sustained (tetanic) contraction as
stimulation frequency rises. Below roughly 10–20 Hz individual twitches are
distinguishable, and above roughly 20–30 Hz contraction is smooth. Fatigue
increases with frequency; Binder-Macleod's work on variable-frequency trains
shows this. A brief high-rate onset (catch-like property, doublets) increases
force.

**Applied as.**

- `fusion_hz` (default 12 Hz) splits rhythms between *pulses* (fused, so the
  rhythm becomes the FOC pulse rate) and *volume* (twitch range, so each burst
  becomes a volume bump).
- Sharp burst attacks are kept as short rise times rather than smoothed away.

**Limits.**

- Fusion frequency depends on muscle, fibre type and fatigue. 12 Hz is a
  pragmatic value that also stays inside the update rate restim can express
  through volume.
- The choice also has a technical motive: rhythms above about 12–15 Hz can't be
  expressed through volume at restim's ~30 ms resolution anyway.

## 7.5 Electrotactile perception

**Findings.**

- Kaczmarek et al. (2017): perceived frequency and intensity interact. To keep
  intensity constant, current or pulse width must fall as frequency rises.
- Akhtar et al. (2018, *Science Robotics*): sensation intensity can be held
  constant by adjusting amplitude and pulse duration together, using an
  impedance-aware model.
- Marcus & Fuglevand (2009); Szeto (1985): magnitude-estimation functions for
  electrical skin stimulation, and the trade-off between pulse rate and pulse
  width.
- Alotaibi, Williamson & Brewster (CHI 2022): intensity and pulse frequency as
  design parameters for electrotactile cues.
- Stevens' power law: perceived intensity of electric current grows with a
  large exponent, much steeper than for loudness.
- restim's own `PulseFrequencyCalibration`: about ±5 % intensity change from
  10 to 100 Hz.

**Applied as.**

- Volume stays electrically linear by default (`gamma = 1`), so that the
  original's current profile is reproduced. Perceptual reshaping is left to
  `gamma`.
- No perceptual compensation is applied when pulse rate changes. The pulse rate
  is copied from the source, not altered, so there is nothing to compensate in
  `pulses` mode. `volume`/continuous mode does switch to a 100 Hz pulse train,
  a known gap (see [10](10-open-issues.md)).

**Limits.** These studies mostly concern fingertip electrotactile displays and
sensory, not motor, responses.

## 7.6 Skin and electrode impedance

**Findings.**

- Dorgan & Reilly (1999): skin impedance model for surface neuromuscular
  stimulation, comparing current-controlled and voltage-controlled stimulation.
- Vargas Luna et al. (2015, *PLoS ONE*): a dynamic skin–electrode model.
  Impedance depends strongly on current and charge (electroporation).
- Medina & Grill (2014): volume-conductor model for kHz transcutaneous
  stimulation, where voltage-regulated stimulation behaves differently.
- Keller & Kuhn (2008): review of surface electrodes and of R‖C interface
  models.

**Applied as.** The circuit model uses series R plus parallel R‖C, with values
in the literature range ([3.3](03-original-hardware-model.md#33-parameter-derivation)).

**Limits.** The model is linear and static. Real skin impedance falls with
current and over time, and differs between electrodes.

## 7.7 EMS in HCI

Background for evaluation methodology and per-user calibration:

- Faltaous, Koelle & Schneegass (2022), taxonomy of EMS in HCI;
- Duente, Schneegass & Pfeiffer (2017), challenges and calibration;
- Pfeiffer, Duente & Rohs (2016), prototyping toolkit;
- Tanaka & Lopes (CHI 2025), course on interactive electrical stimulation.

stimconv itself does no per-user calibration. Absolute intensity, electrode
calibration and limits are left to restim.

## 7.8 References

- Akhtar A, Sombeck J, Boyce B, Bretl T. Controlling sensation intensity for electrotactile stimulation in human–machine interfaces. *Science Robotics*, 2018.
- Alotaibi Y, Williamson JH, Brewster SA. First steps towards designing electrotactons: investigating intensity and pulse frequency as parameters for electrotactile cues. *CHI*, 2022.
- Dorgan SJ, Reilly RB. A model for human skin impedance during surface functional neuromuscular stimulation. *IEEE Trans. Rehabil. Eng.*, 1999.
- Duente T, Schneegass S, Pfeiffer M. EMS in HCI: challenges and opportunities in actuating human bodies. *MobileHCI*, 2017.
- Faltaous S, Koelle M, Schneegass S. From perception to action: a review and taxonomy on electrical muscle stimulation in HCI. *MUM*, 2022.
- Goats GC. Interferential current therapy. *Br J Sports Med*, 1990.
- Kaczmarek KA, Tyler ME, Okpara UO, Haase SJ. Interaction of perceived frequency and intensity in fingertip electrotactile stimulation. *IEEE Trans. Haptics*, 2017.
- Keller T, Kuhn A. Electrodes for transcutaneous (surface) electrical stimulation. *J Automatic Control*, 2008.
- Laufer Y, Elboim M. Effect of burst frequency and duration of kilohertz-frequency alternating currents and of low-frequency pulsed currents on strength of contraction, muscle fatigue, and perceived discomfort. *Phys Ther*, 2008.
- Marcus PL, Fuglevand AJ. Perception of electrical and mechanical stimulation of the skin: implications for electrotactile feedback. *J Neural Eng*, 2009.
- Medina LE, Grill WM. Volume conductor model of transcutaneous electrical stimulation with kilohertz signals. *J Neural Eng*, 2014.
- Merrill DR, Bikson M, Jefferys JGR. Electrical stimulation of excitable tissue: design of efficacious and safe protocols. *J Neurosci Methods*, 2005.
- Modesto KAG et al. Effects of kilohertz frequency, burst duty cycle, and burst duration on evoked torque, perceived discomfort and muscle fatigue: a systematic review. *Am J Phys Med Rehabil*, 2023.
- Modesto KAG et al. Influence of kilohertz frequency, burst duty cycle and burst duration on evoked torque, discomfort and muscle efficiency: a randomized crossover trial. *Physiological Reports*, 2024.
- Opančar A et al. The same biophysical mechanism is involved in both temporal interference and direct kHz stimulation of peripheral nerves. *Nature Communications*, 2025.
- Ozcan J, Ward AR, Robertson VJ. A comparison of true and premodulated interferential currents. *Arch Phys Med Rehabil*, 2004.
- Pfeiffer M, Duente T, Rohs M. Let your body move: a prototyping toolkit for wearable force feedback with electrical muscle stimulation. *MobileHCI*, 2016.
- Szeto AYJ. Relationship between pulse rate and pulse width for a constant-intensity level of electrocutaneous stimulation. *Ann Biomed Eng*, 1985.
- Tanaka Y, Lopes P. How to design, build, and use interactive electrical stimulation. *CHI EA*, 2025.
- Vargas Luna JL, Krenn M, Cortés Ramírez JA, Mayr W. Dynamic impedance model of the skin–electrode interface for transcutaneous electrical stimulation. *PLoS ONE*, 2015.
- Ward AR. Electrical stimulation using kilohertz-frequency alternating current. *Phys Ther*, 2009.
- Ward AR, Chuen WLH. Lowering of sensory, motor, and pain-tolerance thresholds with burst duration using kilohertz-frequency alternating current electric stimulation: part II. *Arch Phys Med Rehabil*, 2009.
- Ward AR, Robertson VJ, Ioannou H. The effect of duty cycle and frequency on muscle torque production using kilohertz frequency range alternating current. *Med Eng Phys*, 2004.
- restim wiki (`reference/restim/restim.wiki`): `nerve-activation.md`, `software-basics.md`, `software-algos.md`, `estim-safety.md`, `skin-resistance.md`.
