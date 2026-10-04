// Command ffmpeg manages the bundled FFmpeg build (docs/FFMPEG.md).
//
//	go run ./tools/ffmpeg fetch [-dir bin]       download the pinned build into <dir>/libs/ffmpeg
//	go run ./tools/ffmpeg update [-branch 9.0]   pin the newest BtbN LGPL build of a branch
//	go run ./tools/ffmpeg show                   print the pin and its notice
//
// -arch arm64 works on ffmpeg-arm64.json (Windows on Arm) instead of ffmpeg.json.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/trilithus/stimconv/internal/ffmpegpin"
)

const cacheDir = ".cache/ffmpeg"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/ffmpeg fetch|update|show [flags]   (see docs/FFMPEG.md)")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	dir := fs.String("dir", "bin", "fetch: directory that holds stimconv.exe")
	branch := fs.String("branch", "", "update: FFmpeg release branch (default: the pinned one)")
	arch := fs.String("arch", "amd64", "Windows architecture of the build: amd64 or arm64")
	fs.Parse(args)

	file, err := ffmpegpin.FileFor(*arch)
	if err != nil {
		return err
	}
	pin, err := ffmpegpin.Load(file)
	if err != nil && cmd != "update" {
		return fmt.Errorf("%s: %w (run from the repository root)", file, err)
	}
	switch cmd {
	case "show":
		fmt.Print(pin.Notice())
	case "fetch":
		zip, err := ffmpegpin.Download(pin, cacheDir)
		if err != nil {
			return err
		}
		if err := ffmpegpin.Install(pin, zip, *dir); err != nil {
			return err
		}
		fmt.Printf("installed FFmpeg %s into %s/libs/ffmpeg\n", pin.Version, *dir)
	case "update":
		b, variant := pin.Branch, pin.Variant
		if *branch != "" {
			b = *branch
		}
		if b == "" && *arch != "amd64" {
			if p, err := ffmpegpin.Load(ffmpegpin.File); err == nil {
				b = p.Branch // follow the x64 pin
			}
		}
		if b == "" {
			return fmt.Errorf("no pinned branch; pass -branch (e.g. -branch 9.0)")
		}
		if variant == "" {
			variant = ffmpegpin.VariantFor(*arch)
		}
		np, err := ffmpegpin.Latest(b, variant)
		if err != nil {
			return err
		}
		if np.Release == pin.Release && np.Asset == pin.Asset {
			fmt.Println("already pinned to the newest build:", np.Version, np.Release)
			return nil
		}
		if err := np.Save(file); err != nil {
			return err
		}
		fmt.Printf("pinned FFmpeg %s (%s); now run `go run ./tools/ffmpeg fetch`, test, and commit %s\n", np.Version, np.Release, file)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}
