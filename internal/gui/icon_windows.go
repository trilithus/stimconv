package gui

import (
	"syscall"
	"unsafe"
)

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procFindWindow = user32.NewProc("FindWindowW")
	procLoadImage  = user32.NewProc("LoadImageW")
	procSendMsg    = user32.NewProc("SendMessageW")
	procGetSysMet  = user32.NewProc("GetSystemMetrics")
	procGetModule  = kernel32.NewProc("GetModuleHandleW")
	procGetPID     = user32.NewProc("GetWindowThreadProcessId")
)

// setWindowIcon gives the window the exe's icon (resource ID 1, embedded by
// rsrc). gogpu registers its window class without an icon and ignores
// Config.Icon on Windows, so the title bar and taskbar would show the
// generic one.
func setWindowIcon(title string) {
	t, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(t)))
	if hwnd == 0 {
		return
	}
	var pid uint32
	procGetPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(syscall.Getpid()) {
		return // another program's window with the same title
	}
	const (
		imageIcon  = 1
		wmSetIcon  = 0x0080
		iconSmall  = 0
		iconBig    = 1
		smCxIcon   = 11
		smCxSmIcon = 49
	)
	mod, _, _ := procGetModule.Call(0)
	load := func(metric uintptr) uintptr {
		sz, _, _ := procGetSysMet.Call(metric)
		h, _, _ := procLoadImage.Call(mod, 1, imageIcon, sz, sz, 0)
		return h
	}
	if h := load(smCxIcon); h != 0 {
		procSendMsg.Call(hwnd, wmSetIcon, iconBig, h)
	}
	if h := load(smCxSmIcon); h != 0 {
		procSendMsg.Call(hwnd, wmSetIcon, iconSmall, h)
	}
}
