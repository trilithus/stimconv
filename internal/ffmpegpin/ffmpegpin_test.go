package ffmpegpin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "b.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"ffmpeg-x/bin/ffmpeg.exe":  "exe",
		"ffmpeg-x/bin/ffplay.exe":  "not wanted",
		"ffmpeg-x/LICENSE.txt":     "LGPL",
		"ffmpeg-x/doc/ffmpeg.html": "not wanted",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()

	p := Pin{Variant: "win64-lgpl", Version: "n9.0.2-1-gabc", FFmpegHash: "abc", BtbNCommit: "def", SHA256: "00"}
	out := filepath.Join(dir, "rel")
	if err := Install(p, zp, out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bin/ffmpeg.exe", "LICENSE.txt", "NOTICE.txt", "ffmpeg.json"} {
		if _, err := os.Stat(filepath.Join(out, "libs", "ffmpeg", want)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "libs", "ffmpeg", "bin", "ffplay.exe")); err == nil {
		t.Error("ffplay.exe should not be installed")
	}
	n, _ := os.ReadFile(filepath.Join(out, "libs", "ffmpeg", "NOTICE.txt"))
	for _, want := range []string{"FFmpeg/FFmpeg/tree/abc", "FFmpeg-Builds/tree/def", "LGPL"} {
		if !strings.Contains(string(n), want) {
			t.Errorf("notice lacks %q", want)
		}
	}
}

func TestDownloadRefusesNonLGPL(t *testing.T) {
	if _, err := Download(Pin{Variant: "win64-gpl"}, t.TempDir()); err == nil {
		t.Fatal("GPL variant accepted")
	}
}
