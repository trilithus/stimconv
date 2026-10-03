package pipeline

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
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
		res, err := Convert(context.Background(), cfg, Options{Input: in, Preset: "p_" + topo}, &log)
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
	got := DefaultOutDir(filepath.Join("music", "song.mp3"), "")
	if want := filepath.Join("music", "song.default"); got != want {
		t.Errorf("DefaultOutDir = %q, want %q", got, want)
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
		"| Original file | `my track.wav` |",
		"| Preset | soft |",
		"| `topology` | joined | no (default dual) |",
		"| `gamma` | 1.5 | no (default 1) |",
		"| `overlap` | auto (not used) | yes |", // dual-only option, topology is joined
		"| `ifc` | beat | yes |",
		"| `ranges.frequency.min` | 500 | yes |",
		"`my track.alpha.funscript`",
		"## restim",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("README lacks %q", want)
		}
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
