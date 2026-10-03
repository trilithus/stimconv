package decode

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func sample(t *testing.T, name string) string {
	p := filepath.Join("..", "..", "reference", "audio", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("sample %s not present", name)
	}
	return p
}

func TestSniffSamples(t *testing.T) {
	cases := map[string]Format{"sample_stayh.mp3": FormatMP3, "sample_long.mp3": FormatMP2}
	for name, want := range cases {
		p := sample(t, name)
		got, err := Sniff(p)
		if err != nil || got != want {
			t.Errorf("Sniff(%s) = %v, %v; want %v", name, got, err, want)
		}
	}
}

// TestDumpPCM writes the first seconds of a decoded file as raw float32 for
// comparison against a reference decoder. Enabled via STIMCONV_DUMP=<in>|<out>.
func TestDumpPCM(t *testing.T) {
	spec := os.Getenv("STIMCONV_DUMP")
	if spec == "" {
		t.Skip("STIMCONV_DUMP not set")
	}
	var in, out string
	for i := range spec {
		if spec[i] == '|' {
			in, out = spec[:i], spec[i+1:]
		}
	}
	src, format, err := Open(in, Options{NativeMP3: os.Getenv("STIMCONV_NATIVE_MP3") != ""})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	buf := make([]float32, src.SampleRate()*2*10)
	if skip := os.Getenv("STIMCONV_SKIP"); skip != "" {
		for i := 0; i < 10; i++ {
			src.Read(buf)
		}
	}
	n, _ := src.Read(buf)
	b := make([]byte, 0, n*8)
	for _, v := range buf[:n*2] {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	t.Logf("format %s rate %d frames %d", format, src.SampleRate(), n)
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
