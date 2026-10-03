package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleProcList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole        = kernel32.NewProc("FreeConsole")
)

// hideOwnConsole detaches from the console when it was created only for this
// process (started from Explorer), which makes Windows close it. A console
// shared with a parent shell is left alone.
func hideOwnConsole() {
	var pids [2]uint32
	if n, _, _ := procGetConsoleProcList.Call(uintptr(unsafe.Pointer(&pids[0])), 2); n == 1 {
		procFreeConsole.Call()
	}
}
