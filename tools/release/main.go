// Command release builds a release package. For Windows that is a zip with
// stimconv.exe plus the pinned FFmpeg build with its license and notice
// (docs/FFMPEG.md). For Linux it is a tar.gz with the binary only: ffmpeg
// comes from the system PATH.
//
//	go run ./tools/release            -> dist/stimconv-<version>-win64.zip
//	go run ./tools/release -os linux  -> dist/stimconv-<version>-linux-amd64.tar.gz
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"flag"
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
	goos := flag.String("os", "windows", "target: windows or linux")
	flag.Parse()
	if *goos != "windows" && *goos != "linux" {
		return fmt.Errorf("unsupported -os %q", *goos)
	}
	version := "dev"
	if v := os.Getenv("STIMCONV_VERSION"); v != "" {
		version = v
	} else if out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}
	stage := filepath.Join("dist", "stimconv")
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}

	if err := writeLicense(stage); err != nil {
		return err
	}
	if *goos == "linux" {
		return releaseLinux(stage, version)
	}

	pin, err := ffmpegpin.Load(ffmpegpin.File)
	if err != nil {
		return fmt.Errorf("%s: %w (run from the repository root)", ffmpegpin.File, err)
	}
	if err := goBuild("windows", filepath.Join(stage, "stimconv.exe")); err != nil {
		return err
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

More information: https://github.com/trilithus/stimconv#readme

stimconv itself is MIT No Attribution (see LICENSE.txt). It is provided as is,
without warranty, and its authors accept no liability for any harm or damage
arising from the software, its use, or the use of the files it produces.
`, version, pin.Version)
	if err := os.WriteFile(filepath.Join(stage, "README.txt"), []byte(readme), 0o644); err != nil {
		return err
	}

	out := filepath.Join("dist", "stimconv-"+version+"-win64.zip")
	if err := zipDir(stage, out); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func goBuild(goos, out string) error {
	build := exec.Command("go", "build", "-trimpath", "-o", out, ".")
	build.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	return nil
}

func writeLicense(stage string) error {
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, "LICENSE.txt"), license, 0o644)
}

func releaseLinux(stage, version string) error {
	if err := goBuild("linux", filepath.Join(stage, "stimconv")); err != nil {
		return err
	}
	readme := fmt.Sprintf(`stimconv %s

Run ./stimconv to open the interface, or "./stimconv cli -h" for the
command line.

FFmpeg is not bundled: install it from your distribution (it must be on
PATH, or pass --ffmpeg / set STIMCONV_FFMPEG). It is needed to decode MP3
and other formats. See Help > About in stimconv for all attributions.

More information: https://github.com/trilithus/stimconv#readme

stimconv itself is MIT No Attribution (see LICENSE.txt). It is provided as is,
without warranty, and its authors accept no liability for any harm or damage
arising from the software, its use, or the use of the files it produces.
`, version)
	if err := os.WriteFile(filepath.Join(stage, "README.txt"), []byte(readme), 0o644); err != nil {
		return err
	}
	out := filepath.Join("dist", "stimconv-"+version+"-linux-amd64.tar.gz")
	if err := tarDir(stage, out); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

// tarDir writes a gzipped tar with the contents of dir at its root.
func tarDir(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	base := dir
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		if rel == "." {
			return nil
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Uname, h.Gname, h.Uid, h.Gid = "", "", 0, 0
		h.Mode = 0o644
		if info.IsDir() {
			h.Name += "/"
			h.Mode = 0o755
		} else if filepath.Base(p) == "stimconv" {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil || info.IsDir() {
			return err
		}
		r, err := os.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(tw, r)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// zipDir zips the contents of dir at the archive root.
func zipDir(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	base := dir
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
