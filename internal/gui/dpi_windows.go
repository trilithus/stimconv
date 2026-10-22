package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

var getDpiForWindow = windows.NewLazySystemDLL("user32.dll").NewProc("GetDpiForWindow")

// watchDPI re-applies the theme when the window moves to a monitor with
// another scale setting. Fyne's GLFW driver ignores Windows' DPI change and
// only rescales when its own size-based monitor guess changes, which left
// the window at the old scale until it was resized a few times. Setting the
// theme makes Fyne recompute every window's scale from the current DPI. The
// rescale keeps the window's pixel size, which shrinks it on a higher scale,
// so the size in Fyne units from before the move is restored afterwards.
func watchDPI(a fyne.App, w fyne.Window) {
	if getDpiForWindow.Find() != nil { // before Windows 10 1607
		return
	}
	var last uintptr
	var size fyne.Size
	check := func() {
		nw, ok := w.(driver.NativeWindow)
		if !ok {
			return
		}
		nw.RunNative(func(ctx any) {
			wc, ok := ctx.(driver.WindowsWindowContext)
			if !ok || wc.HWND == 0 {
				return
			}
			dpi, _, _ := getDpiForWindow.Call(wc.HWND)
			if dpi == 0 {
				return
			}
			if last != 0 && dpi != last {
				a.Settings().SetTheme(appTheme())
				w.Resize(size)
			}
			last = dpi
			size = w.Canvas().Size()
		})
	}
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			fyne.Do(check)
		}
	}()
}
