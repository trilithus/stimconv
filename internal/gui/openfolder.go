package gui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/trilithus/stimconv/internal/presets"
)

func (u *ui) openPresetFolder() {
	d, err := presets.Folder()
	if err != nil {
		u.logf("error: preset folder: %v", err)
		return
	}
	if err := openFolder(d); err != nil {
		u.logf("error: open %s: %v", d, err)
	}
}

// openFolder shows dir in the platform file manager without waiting for it.
func openFolder(dir string) error {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("explorer", dir)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", dir)
	case os.Getenv("WSL_DISTRO_NAME") != "":
		// WSL usually has no Linux file manager; use Windows Explorer.
		if out, err := exec.Command("wslpath", "-w", dir).Output(); err == nil {
			cmd = exec.Command("explorer.exe", strings.TrimSpace(string(out)))
			break
		}
		fallthrough
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
