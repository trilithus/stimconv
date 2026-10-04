package main

import (
	"bytes"
	"embed"
	"image"
	"image/png"
	"io/fs"
)

// The application icon is drawn by tools/icon. rsrc embeds icon.ico in
// rsrc_windows_{amd64,arm64}.syso, which `go build` links into Windows
// executables automatically (icon group resource ID 1, used for the window icon).
//
//go:generate go run ./tools/icon
//go:generate go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/icon.ico -o rsrc_windows_amd64.syso
//go:generate go run github.com/akavel/rsrc@v0.10.2 -arch arm64 -ico assets/icon.ico -o rsrc_windows_arm64.syso

// UI icons (Flaticon) and their attribution files, shown in the GUI's
// About window.
//
//go:embed assets/copy.png assets/copy.txt assets/trash.png assets/trash.txt
var assets embed.FS

//go:embed assets/icon.png
var appIconPNG []byte

func iconFS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}

func appIcon() image.Image {
	img, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		return nil
	}
	return img
}
