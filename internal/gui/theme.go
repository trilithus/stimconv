package gui

import (
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// lightTheme is Fyne's default theme, always in its light variant: the
// window stays light like the earlier UI even when the OS is in dark mode.
// FYNE_THEME=dark (or light) still chooses explicitly.
type lightTheme struct{ fyne.Theme }

func (t lightTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameDisabled {
		// help text uses LowImportance, drawn in this colour; Fyne's light
		// grey is too faint to read
		return color.NRGBA{R: 0x80, G: 0x80, B: 0x88, A: 0xff}
	}
	return t.Theme.Color(n, theme.VariantLight)
}

func appTheme() fyne.Theme {
	if os.Getenv("FYNE_THEME") != "" {
		return theme.DefaultTheme()
	}
	return lightTheme{theme.DefaultTheme()}
}
