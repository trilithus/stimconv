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

	"github.com/trilithus/stimconv/internal/pipeline"
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
	// next to each input by default
	for _, d := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dir, d+".alpha.funscript")); err != nil {
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

// Re-converting asks before overwriting: "Skip" keeps a file's output,
// "Overwrite all" answers for the rest of the batch.
func TestOverwritePrompt(t *testing.T) {
	dir := t.TempDir()
	in := []string{filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav"), filepath.Join(dir, "c.wav")}
	for _, p := range in {
		writeStimWAV(t, p, 3)
	}
	appl := app.New()
	u := newUI(nil, appl)
	appl.SetRoot(u.build())
	var asked []string
	answers := [][2]bool{{false, false}, {true, true}} // skip a, then overwrite all
	u.askOverwrite = func(c []pipeline.Conflict, batch bool) (bool, bool) {
		if !batch {
			t.Error("a batch must offer Overwrite all")
		}
		a := answers[min(len(asked), len(answers)-1)]
		asked = append(asked, filepath.Base(c[0].Path))
		return a[0], a[1]
	}
	run := func() {
		u.setInputs(in)
		if !pump(u, 20*time.Second, func() bool { return !strings.Contains(u.info.Get(), "checking") }) {
			t.Fatal("probe did not finish")
		}
		u.startOrCancel()
		if !pump(u, 60*time.Second, func() bool { return !u.running.Get() }) {
			t.Fatal("batch did not finish")
		}
	}
	run() // fresh: nothing to ask
	if len(asked) != 0 {
		t.Fatalf("asked %v on a fresh folder", asked)
	}
	run()
	if len(asked) != 2 || !strings.HasPrefix(asked[0], "a.") || !strings.HasPrefix(asked[1], "b.") {
		t.Errorf("asked about %v, want a then b (c covered by Overwrite all)", asked)
	}
	if st := u.status.Get(); !strings.Contains(st, "2 of 3 converted, 1 skipped") {
		t.Errorf("status = %q", st)
	}
}
