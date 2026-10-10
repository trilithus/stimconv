package gui

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogpu/ui/app"
)

// writeStimWAV writes a 16-bit stereo 650 Hz square with 20 Hz bursts.
func writeStimWAV(t *testing.T, path string, seconds float64) {
	const sr = 22050
	var pcm bytes.Buffer
	for i := 0; i < int(seconds*sr); i++ {
		ts := float64(i) / sr
		v := math.Copysign(0.6, math.Sin(2*math.Pi*650*ts))
		l := v
		if math.Mod(ts*20, 1) > 0.5 {
			l = 0
		}
		binary.Write(&pcm, binary.LittleEndian, [2]int16{int16(l * 32767), int16(v * 32767)})
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

// pump drains posted UI work until cond holds or the timeout passes.
func pump(u *ui, timeout time.Duration, cond func() bool) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		u.drain()
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestBatch(t *testing.T) {
	dir := t.TempDir()
	a, b, bad := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav"), filepath.Join(dir, "bad.mp3")
	writeStimWAV(t, a, 3)
	writeStimWAV(t, b, 3)
	os.WriteFile(bad, []byte("not audio"), 0o644)
	os.Setenv("STIMCONV_FFMPEG", filepath.Join(dir, "no-ffmpeg")) // keep the bad file from reaching a real ffmpeg
	defer os.Unsetenv("STIMCONV_FFMPEG")

	appl := app.New()
	u := newUI(nil, appl)
	appl.SetRoot(u.build())
	u.setInputs([]string{a, b, bad, a}) // duplicates are dropped
	if len(u.inputs) != 3 || !strings.Contains(u.input.Get(), "3 files") {
		t.Fatalf("inputs = %q, field = %q", u.inputs, u.input.Get())
	}
	if !pump(u, 20*time.Second, func() bool { return !strings.Contains(u.info.Get(), "checking") }) {
		t.Fatalf("probe did not finish: %q", u.info.Get())
	}
	u.startOrCancel()
	if !pump(u, 60*time.Second, func() bool { return !u.running.Get() }) {
		t.Fatal("batch did not finish")
	}
	for _, d := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dir, d, d+".alpha.funscript")); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	if st := u.status.Get(); !strings.Contains(st, "2 of 3 converted, 1 failed") {
		t.Errorf("status = %q", st)
	}
	log := u.logText(tagSet)
	for _, want := range []string{"1/3 · a.wav", "3/3 · bad.mp3", "[error] failed: bad.mp3", "batch finished"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q", want)
		}
	}

	// typing a single path leaves batch mode
	u.setInput(a)
	if u.inputs != nil {
		t.Error("still in batch mode")
	}
}
