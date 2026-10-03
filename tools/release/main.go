// Command release builds the Windows release zip: stimconv.exe plus the
// pinned FFmpeg build with its license and notice (docs/FFMPEG.md).
//
//	go run ./tools/release            -> dist/stimconv-<version>-win64.zip
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/trilithus/stimconv/internal/ffmpegpin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run() error {
	pin, err := ffmpegpin.Load(ffmpegpin.File)
	if err != nil {
		return fmt.Errorf("%s: %w (run from the repository root)", ffmpegpin.File, err)
	}
	version := "dev"
	if out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}
	stage := filepath.Join("dist", "stimconv")
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}

	build := exec.Command("go", "build", "-trimpath", "-o", filepath.Join(stage, "stimconv.exe"), ".")
	build.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	zipPath, err := ffmpegpin.Download(pin, filepath.Join(".cache", "ffmpeg"))
	if err != nil {
		return err
	}
	if err := ffmpegpin.Install(pin, zipPath, stage); err != nil {
		return err
	}
	readme := fmt.Sprintf(`stimconv %s

Double-click stimconv.exe to open the interface, or run
"stimconv.exe cli -h" in a terminal for the command line.

libs/ffmpeg contains FFmpeg %s (LGPL v3+), used to decode MP3 and other
formats. See libs/ffmpeg/NOTICE.txt for its license and source code, and
Help > About in stimconv for all other attributions.

stimconv itself is MIT No Attribution (see LICENSE.txt). It is provided as is,
without warranty, and its authors accept no liability for any harm or damage
arising from the software, its use, or the use of the files it produces.
`, version, pin.Version)
	if err := os.WriteFile(filepath.Join(stage, "README.txt"), []byte(readme), 0o644); err != nil {
		return err
	}

	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "LICENSE.txt"), license, 0o644); err != nil {
		return err
	}

	out := filepath.Join("dist", "stimconv-"+version+"-win64.zip")
	if err := zipDir(stage, out); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

// zipDir zips dir so the archive contains a single top-level "stimconv/".
func zipDir(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	base := filepath.Dir(dir)
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		r, err := os.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(w, r)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}
