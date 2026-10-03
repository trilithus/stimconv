//go:build !windows

package decode

import "os/exec"

func hideWindow(*exec.Cmd) {}
