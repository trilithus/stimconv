# Bundled FFmpeg

stimconv decodes MPEG Layer III (and other non-native formats) by running
`ffmpeg.exe` as a separate program. The Windows release zip ships an
unmodified FFmpeg build from [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds)
in `libs/ffmpeg/`, where stimconv finds it automatically.

## What is pinned

`ffmpeg.json` (x64) and `ffmpeg-arm64.json` (Windows on Arm, BtbN's
`winarm64-lgpl` variant) in the repository root are the single source of
truth. Each holds the BtbN
release tag, zip name, download URL, SHA-256, FFmpeg version and commit, and
the FFmpeg-Builds commit the binary was built from. Nothing else needs editing
when FFmpeg is updated.

## Commands (run from the repository root)

| Task | Command |
|---|---|
| Put the pinned ffmpeg next to your dev build (`bin/libs/ffmpeg`) | `go run ./tools/ffmpeg fetch` |
| Show the pin and the notice that ships with it | `go run ./tools/ffmpeg show` |
| Pin the newest build of the current branch | `go run ./tools/ffmpeg update` |
| Switch to another FFmpeg release branch | `go run ./tools/ffmpeg update -branch 9.1` |
| Build the release zip (`dist/stimconv-<version>-windows-x64.zip`) | `go run ./tools/release` |
| Build the Arm64 zip (`dist/stimconv-<version>-windows-arm64.zip`) | `go run ./tools/release -arch arm64` |

Add `-arch arm64` to the `tools/ffmpeg` commands to work on the Arm64 pin.

Downloads are cached in `.cache/ffmpeg/` and verified against the pinned
SHA-256. Set `GITHUB_TOKEN` if the GitHub API rate limit gets in the way.

## Updating FFmpeg (e.g. in two years)

1. `go run ./tools/ffmpeg update` (or `-branch X.Y` to move to a newer FFmpeg
   release; branches are listed on the BtbN releases page as `nX.Y`).
2. `go run ./tools/ffmpeg fetch`, then convert a Layer III file such as
   `reference/audio/sample_stayh.mp3` (GUI or `bin/stimconv.exe cli --dry-run …`).
3. Repeat step 1 with `-arch arm64` so both pins stay on the same version.
4. Commit `ffmpeg.json` and `ffmpeg-arm64.json`.
5. `go run ./tools/release` (and `-arch arm64`) for new zips.

BtbN deletes old autobuilds after a while. If `fetch` or `release` reports a
404 for the pinned zip, that is expected: run step 1 to pin a current build.
If `update` finds no build for the branch, BtbN has stopped building it; pick
a newer branch with `-branch`.

## License: how we comply, and its limits

- **LGPL build only.** The tools refuse variants without `lgpl` in the name.
  Never switch to a `gpl` or `nonfree` variant: GPL would impose GPL terms on
  distribution, and nonfree builds may not be redistributed at all.
- **Unmodified, separate program.** stimconv runs `ffmpeg.exe` as a child
  process and does not link FFmpeg, so stimconv's own license is unaffected.
- **What ships in `libs/ffmpeg/`:**
  - `LICENSE.txt`: BtbN's license text (LGPL v3);
  - `NOTICE.txt`, generated from the pin: the FFmpeg version, links to the
    exact FFmpeg source commit and the FFmpeg-Builds commit (whose
    `scripts.d/` pins every library compiled into the binary), the
    download URL and its SHA-256;
  - `ffmpeg.json`: a copy of the pin.

  The About page shows the notice when a bundled build is present.
- **Known gap (accepted pragmatically).** BtbN's static build includes about
  60 third-party libraries, and its zip only carries FFmpeg's LGPL text. We
  point to the sources through links rather than shipping the full
  corresponding source and every library's license text. We rely on GitHub
  keeping those repositories available. That is the "pragmatic middle
  ground" chosen for a small research tool. It is not a complete compliance
  package, and this is not legal advice. If stimconv is ever distributed more
  widely, either publish an archive of the sources (FFmpeg-Builds'
  `download.sh` fetches all of them) next to the release, or stop bundling and
  let users download FFmpeg themselves.
