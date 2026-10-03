package gui

import "os"

// preferX11OnWSL makes gogpu use XWayland under WSLg. WSLg's Wayland
// compositor lacks fractional scaling, so gogpu's Wayland backend with a
// GDK_SCALE/QT_SCALE_FACTOR override renders a scaled buffer into an
// unscaled window and gets clipped. The X11 backend sizes the window from
// Xft.dpi consistently (e.g. `xrdb -merge <<< "Xft.dpi: 144"` for 150%).
func preferX11OnWSL() {
	if os.Getenv("WSL_DISTRO_NAME") != "" && os.Getenv("DISPLAY") != "" {
		os.Unsetenv("WAYLAND_DISPLAY")
	}
}
