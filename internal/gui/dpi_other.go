//go:build !windows

package gui

import "fyne.io/fyne/v2"

// watchDPI is only needed on Windows (see dpi_windows.go).
func watchDPI(fyne.App, fyne.Window) {}
