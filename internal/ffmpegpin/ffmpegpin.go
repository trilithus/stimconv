// Package ffmpegpin manages the pinned BtbN FFmpeg build that is shipped
// next to stimconv.exe (see docs/FFMPEG.md). ffmpeg.json in the repository
// root is the single source of truth.
package ffmpegpin

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// File is the pin file, relative to the repository root.
const File = "ffmpeg.json"

// Pin identifies one BtbN build exactly.
type Pin struct {
	Branch     string `json:"branch"`        // FFmpeg release branch, e.g. "9.0"
	Variant    string `json:"variant"`       // BtbN variant; must stay LGPL ("win64-lgpl")
	Release    string `json:"release"`       // BtbN release tag (autobuild-YYYY-MM-DD-HH-MM)
	Asset      string `json:"asset"`         // zip file name
	URL        string `json:"url"`           // download URL
	SHA256     string `json:"sha256"`        // of the zip
	Version    string `json:"version"`       // FFmpeg describe string, e.g. n9.0.2-22-g46d8f462ee
	FFmpegHash string `json:"ffmpeg_commit"` // abbreviated FFmpeg commit
	BtbNCommit string `json:"btbn_commit"`   // FFmpeg-Builds commit the release was built from
	Updated    string `json:"updated"`       // date the pin was written
}

func Load(path string) (Pin, error) {
	var p Pin
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(b, &p)
}

func (p Pin) Save(path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// FFmpegSourceURL and BtbNSourceURL point at the exact sources of the build.
func (p Pin) FFmpegSourceURL() string {
	return "https://github.com/FFmpeg/FFmpeg/tree/" + p.FFmpegHash
}

func (p Pin) BtbNSourceURL() string {
	return "https://github.com/BtbN/FFmpeg-Builds/tree/" + p.BtbNCommit
}

// Notice is the text shipped as libs/ffmpeg/NOTICE.txt.
func (p Pin) Notice() string {
	return fmt.Sprintf(`FFmpeg %s (%s build)

This directory contains ffmpeg.exe from FFmpeg (https://ffmpeg.org), an
unmodified binary built by BtbN/FFmpeg-Builds. stimconv runs it as a separate
program to decode audio; stimconv itself does not include FFmpeg code.

License: GNU Lesser General Public License version 3 or later (LGPL v3+),
see LICENSE.txt. The build also statically includes third-party libraries
under their own licenses; they are listed by the build scripts below.

Source code of this exact build:
  FFmpeg source:        %s
  Build scripts:        %s
                        (scripts.d/ pins every included library by commit)
  Binary distribution:  %s
  SHA-256 of that zip:  %s

FFmpeg is a trademark of Fabrice Bellard, originator of the FFmpeg project.
`, p.Version, p.Variant, p.FFmpegSourceURL(), p.BtbNSourceURL(), p.URL, p.SHA256)
}

// --- update ----------------------------------------------------------------

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:<hex>"
}

type ghRelease struct {
	Tag    string    `json:"tag_name"`
	Assets []ghAsset `json:"assets"`
}

func getJSON(url string, v any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Latest finds the newest BtbN autobuild of branch/variant.
func Latest(branch, variant string) (Pin, error) {
	var rels []ghRelease
	if err := getJSON("https://api.github.com/repos/BtbN/FFmpeg-Builds/releases?per_page=30", &rels); err != nil {
		return Pin{}, err
	}
	// ffmpeg-n9.0.2-22-g46d8f462ee-win64-lgpl-9.0.zip
	re := regexp.MustCompile(`^ffmpeg-(n` + regexp.QuoteMeta(branch) + `[^-]*-\d+-g([0-9a-f]+))-` + regexp.QuoteMeta(variant) + `-` + regexp.QuoteMeta(branch) + `\.zip$`)
	sort.Slice(rels, func(i, j int) bool { return rels[i].Tag > rels[j].Tag })
	for _, r := range rels {
		if !strings.HasPrefix(r.Tag, "autobuild-") {
			continue // "latest" is a moving tag
		}
		for _, a := range r.Assets {
			m := re.FindStringSubmatch(a.Name)
			if m == nil {
				continue
			}
			var ref struct {
				Object struct{ SHA string } `json:"object"`
			}
			if err := getJSON("https://api.github.com/repos/BtbN/FFmpeg-Builds/git/ref/tags/"+r.Tag, &ref); err != nil {
				return Pin{}, err
			}
			return Pin{
				Branch: branch, Variant: variant, Release: r.Tag, Asset: a.Name, URL: a.URL,
				SHA256: strings.TrimPrefix(a.Digest, "sha256:"), Version: m[1], FFmpegHash: m[2],
				BtbNCommit: ref.Object.SHA, Updated: time.Now().Format("2006-01-02"),
			}, nil
		}
	}
	return Pin{}, fmt.Errorf("no BtbN autobuild found for FFmpeg %s %s (is the branch still built? see docs/FFMPEG.md)", branch, variant)
}

// --- fetch -----------------------------------------------------------------

// Download returns the pinned zip from cacheDir, downloading and verifying it
// when missing.
func Download(p Pin, cacheDir string) (string, error) {
	if !strings.Contains(p.Variant, "lgpl") {
		return "", fmt.Errorf("variant %q is not an LGPL build; refusing (see docs/FFMPEG.md)", p.Variant)
	}
	dst := filepath.Join(cacheDir, p.Asset)
	if sum, err := fileSHA256(dst); err == nil && sum == p.SHA256 {
		return dst, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	resp, err := http.Get(p.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s: %s (BtbN deletes old builds; run `go run ./tools/ffmpeg update`)", p.URL, resp.Status)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	if sum := hex.EncodeToString(h.Sum(nil)); sum != p.SHA256 {
		os.Remove(tmp)
		return "", fmt.Errorf("sha256 mismatch for %s: got %s, pinned %s", p.Asset, sum, p.SHA256)
	}
	return dst, os.Rename(tmp, dst)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Install extracts ffmpeg.exe and LICENSE.txt from the zip into
// <dir>/libs/ffmpeg and writes NOTICE.txt and ffmpeg.json next to them.
func Install(p Pin, zipPath, dir string) error {
	out := filepath.Join(dir, "libs", "ffmpeg")
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		return err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	want := map[string]string{"bin/ffmpeg.exe": "bin/ffmpeg.exe", "LICENSE.txt": "LICENSE.txt"}
	for _, f := range zr.File {
		_, rel, ok := strings.Cut(f.Name, "/") // strip the top-level directory
		target, wanted := want[rel]
		if !ok || !wanted {
			continue
		}
		if err := extract(f, filepath.Join(out, target)); err != nil {
			return err
		}
		delete(want, rel)
	}
	if len(want) > 0 {
		return errors.New("zip lacks bin/ffmpeg.exe or LICENSE.txt")
	}
	if err := os.WriteFile(filepath.Join(out, "NOTICE.txt"), []byte(p.Notice()), 0o644); err != nil {
		return err
	}
	return p.Save(filepath.Join(out, File))
}

func extract(f *zip.File, dst string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
