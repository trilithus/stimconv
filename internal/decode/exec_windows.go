package decode

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps ffmpeg (a console program) from opening a console window
// when stimconv runs without one (GUI started from Explorer).
func hideWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
