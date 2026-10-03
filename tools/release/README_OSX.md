# macOS

There is no official macOS release. A macOS build might be trivial: stimconv
is pure Go (no cgo), and it cross-compiles for `darwin/arm64` and
`darwin/amd64` without changes. But the author/maintainer has no Mac to test
it on, so it has never been run on macOS, and it is not shipped.

If you have a Mac and want to try it, the steps below should get you there.
Reports (working or not) are welcome as GitHub issues.

## Building

Install Go (the version in `go.mod` or newer, e.g. `brew install go`) and
FFmpeg (`brew install ffmpeg`), then:

```sh
git clone https://github.com/trilithus/stimconv.git
cd stimconv
CGO_ENABLED=0 go build -trimpath -o stimconv .
./stimconv            # GUI
./stimconv cli -h     # command line
```

Use `GOARCH=amd64` or `GOARCH=arm64` to build for the other architecture.
Cross-compiling from Linux or Windows works too:
`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o stimconv .`

## Known gaps

- **FFmpeg is not bundled.** It is needed to decode MP3 and most other
  formats. Started from a terminal, stimconv finds Homebrew's `ffmpeg` on
  `PATH`. Apps started from Finder don't get the shell `PATH`, so pass
  `--ffmpeg /opt/homebrew/bin/ffmpeg` (Intel: `/usr/local/bin/ffmpeg`) or
  set `STIMCONV_FFMPEG`.
- **No `.app` bundle.** Double-clicking the bare binary opens it through
  Terminal. A proper `stimconv.app` needs an `Info.plist` and an `.icns`
  icon (`assets/icon.png` is the source).
- **Not signed or notarized.** A binary downloaded from elsewhere is blocked
  by Gatekeeper. Remove the quarantine flag with
  `xattr -dr com.apple.quarantine stimconv`. Builds made on the Mac itself
  are not affected.
- **GUI untested.** Rendering (Metal), HiDPI scaling and the file dialogs
  have only been tested on Windows and Linux.
