package teleplot

import (
	"syscall"
	"unsafe"
)

// enableVT turns on ANSI escape processing for consoles that default it off
// (classic conhost). Windows Terminal already has it on.
func enableVT() {
	const enableVirtualTerminalProcessing = 0x0004
	k := syscall.NewLazyDLL("kernel32.dll")
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if r, _, _ := k.NewProc("GetConsoleMode").Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	k.NewProc("SetConsoleMode").Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
}
