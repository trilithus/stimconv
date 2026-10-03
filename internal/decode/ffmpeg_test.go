//go:build !windows

package decode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFFmpegStderrCaptured uses a fake ffmpeg that fails: its stderr must
// end up in the error instead of being written to our (possibly invalid)
// stderr handle.
func TestFFmpegStderrCaptured(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'Invalid data found when processing input' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := FFmpegPath
	FFmpegPath = fake
	defer func() { FFmpegPath = old }()

	src, err := openFFmpeg("whatever.mp3", 0)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 64)
	src.Read(buf)
	err = src.Close()
	if err == nil || !strings.Contains(err.Error(), "Invalid data found") {
		t.Fatalf("Close() = %v, want ffmpeg's stderr in the error", err)
	}
}
