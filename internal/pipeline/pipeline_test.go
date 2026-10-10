package pipeline

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
	"github.com/trilithus/stimconv/internal/funscript"
)

// writeWAV writes a 16-bit stereo 650 Hz square burst train (20 Hz) on L and
// a steady 800 Hz square on R.
func writeWAV(t *testing.T, path string, seconds float64) {
	const sr = 44100
	n := int(seconds * sr)
	var pcm bytes.Buffer
	for i := 0; i < n; i++ {
		ts := float64(i) / sr
		l, r := 0.0, 0.5*math.Copysign(1, math.Sin(2*math.Pi*800*ts))
		if math.Mod(ts*20, 1) < 0.5 {
			l = 0.8 * math.Copysign(1, math.Sin(2*math.Pi*650*ts))
		}
		binary.Write(&pcm, binary.LittleEndian, [2]int16{int16(l * 32767), int16(r * 32767)})
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+pcm.Len()))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(2), uint32(sr), uint32(sr * 4), uint16(4), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(pcm.Len()))
	b.Write(pcm.Bytes())
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConvertWritesFunscripts(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "track.wav")
	writeWAV(t, in, 4)
	for _, topo := range []string{"dual", "joined"} {
		cfg := config.Default()
		cfg.Topology = topo
		var log bytes.Buffer
		res, err := Convert(context.Background(), cfg, Options{Input: in, Preset: "p_" + topo, PresetSuffix: true}, &log)
		if err != nil {
			t.Fatalf("%s: %v\n%s", topo, err, log.String())
		}
		out := filepath.Join(dir, "track.p_"+topo)
		if len(res.Files) == 0 || res.OutDir != out {
			t.Fatalf("%s: no files written (%+v)", topo, res)
		}
		want := "track.e1.funscript"
		if topo == "joined" {
			want = "track.alpha.funscript"
		}
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("%s: %v", topo, err)
		}
		if !strings.Contains(log.String(), res.Hints) {
			t.Errorf("%s: hints missing from log", topo)
		}
	}
}

func TestConvertCancelled(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "track.wav")
	writeWAV(t, in, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Convert(ctx, config.Default(), Options{Input: in}, &bytes.Buffer{}); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDefaultOutDir(t *testing.T) {
	in := filepath.Join("music", "song.mp3")
	if got, want := DefaultOutDir(in, "", FolderPreset("", true)), filepath.Join("music", "song.default"); got != want {
		t.Errorf("DefaultOutDir = %q, want %q", got, want)
	}
	if got, want := DefaultOutDir(in, "", FolderPreset("soft", false)), filepath.Join("music", "song"); got != want {
		t.Errorf("DefaultOutDir without preset suffix = %q, want %q", got, want)
	}
}

func TestOutName(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "PEP11.fr.wav")
	writeWAV(t, in, 2)
	res, err := Convert(context.Background(), config.Default(), Options{Input: in, OutName: "PEP11"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "PEP11"); res.OutDir != want {
		t.Errorf("OutDir = %q, want %q", res.OutDir, want)
	}
	for _, f := range res.Files {
		if b := filepath.Base(f); !strings.HasPrefix(b, "PEP11.") || strings.Contains(b, ".fr.") {
			t.Errorf("funscript %q not named after OutName", b)
		}
	}
	out := t.TempDir()
	if _, err := Convert(context.Background(), config.Default(), Options{Input: in, OutDir: out, OutName: "PEP11"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "PEP11.md")); err != nil {
		t.Errorf("report not named after OutName: %v", err)
	}
	for _, bad := range []string{"a/b", `a\b`, "..", "x:y", "trail."} {
		if ValidateOutName(bad) == nil {
			t.Errorf("ValidateOutName(%q) accepted", bad)
		}
	}
	if _, err := Convert(context.Background(), config.Default(), Options{Input: in, OutName: "../x"}, &bytes.Buffer{}); err == nil {
		t.Error("Convert accepted a path as OutName")
	}
}

func TestReadme(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "my track.wav")
	writeWAV(t, in, 2)
	cfg := config.Default()
	cfg.Topology, cfg.Gamma = "joined", 1.5

	// default folder -> README.md
	res, err := Convert(context.Background(), cfg, Options{Input: in, Preset: "soft"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(res.OutDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	for _, want := range []string{
		"converted from the audio drive signal in `my track.wav`",
		"restim " + RestimVersion, "FOC-Stim firmware " + FOCStimVersion,
		"| Device wizard | Device type | FOC-Stim 3-phase |",
		"| Preset | soft |",
		"| `topology` | joined | yes |",
		"| `gamma` | 1.5 | no (default 1) |",
		"| `overlap` | auto (not used) | yes |", // dual-only option, topology is joined
		"| `ifc` | beat | yes |",
		"| `ranges.frequency.min` | 500 | yes |",
		"`my track.alpha.funscript`",
		"## Setup",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("README lacks %q", want)
		}
	}
	// it gets shared, so no local paths
	if strings.Contains(md, dir) || strings.Contains(md, filepath.ToSlash(dir)) {
		t.Error("README contains the local input path")
	}
	// the recipient's part comes before the converter's
	if strings.Index(md, "## Setup") > strings.Index(md, "### stimconv settings") {
		t.Error("restim setup is not ahead of the stimconv settings")
	}
	// every option is listed
	for _, o := range config.Options {
		if !strings.Contains(md, "`"+o.Key+"`") {
			t.Errorf("README lacks setting %s", o.Key)
		}
	}

	// chosen folder -> <name>.md
	out := filepath.Join(dir, "chosen")
	if _, err := Convert(context.Background(), cfg, Options{Input: in, OutDir: out}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "my track.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(out, "README.md")); err == nil {
		t.Error("README.md written to a chosen folder")
	}

	// dry run writes nothing
	dry := filepath.Join(dir, "dry")
	if _, err := Convert(context.Background(), cfg, Options{Input: in, OutDir: dry, DryRun: true}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dry); err == nil {
		t.Error("dry run created the output folder")
	}
}

func TestRestimSettings(t *testing.T) {
	cfg := config.Default()
	cfg.Intensity = "effective" // tau-matched volume
	axes := []*funscript.Axis{
		{Name: "frequency", Min: 300, Max: 2000, Values: []float64{250, 800, 2000}},
		{Name: "pulse_frequency", Min: 0, Max: 100, Values: []float64{20}},
		{Name: "volume", Min: 0, Max: 1, Values: []float64{0.5}},
	}
	got := map[string]RestimSetting{}
	for _, s := range RestimSettings(cfg, axes) {
		got[s.Name] = s
	}
	for name, change := range map[string]bool{
		"Minimum frequency [Hz]":               true, // 300 written, restim 500
		"Maximum frequency [Hz]":               true, // tau-matched to 2000, restim 1000
		"Nerve time constant [µs]":             false,
		"Burst gap instead of pulse frequency": true,
		"frequency limit min – max":            true,
		"volume limit min – max":               false,
		"Waveform amplitude [mA]":              false,
	} {
		s, ok := got[name]
		if !ok {
			t.Errorf("%s missing", name)
		} else if s.Change != change {
			t.Errorf("%s: change = %v, want %v (%+v)", name, s.Change, change, s)
		}
	}
	// with plain peak current the volume is the recorded level, so restim's
	// carrier derating must be off (tau 0)
	cur := config.Default()
	for _, s := range RestimSettings(cur, axes) {
		if s.Name == "Nerve time constant [µs]" && (!s.Change || !strings.HasPrefix(s.Need, "0 ")) {
			t.Errorf("current intensity: tau row %+v, want 0 and a change", s)
		}
	}
	if s := got["Minimum frequency [Hz]"]; s.Need != "300 or lower" {
		t.Errorf("min carrier need %q: values must be clamped to the written range", s.Need)
	}
}

// The restim defaults in restim.go were read from the reference submodules;
// a submodule bump must update the versions (and re-check the defaults).
func TestReferenceVersions(t *testing.T) {
	for dir, want := range map[string]string{"../../reference/restim": RestimVersion, "../../reference/FOC-Stim": FOCStimVersion} {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Logf("%s absent, skipped", dir)
			continue
		}
		out, err := exec.Command("git", "-C", dir, "describe", "--tags", "--always").Output()
		if err != nil {
			t.Skipf("git describe: %v", err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s is at %s, restim.go says %s: re-check the defaults there and update the version", dir, got, want)
		}
	}
}
