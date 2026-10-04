//go:build windows

package gui

import (
	"os"

	"github.com/gogpu/gogpu"
)

// graphicsAPI picks DX12 on Windows unless GOGPU_GRAPHICS_API says otherwise.
// With the default (auto) gogpu uses Vulkan, and the Qualcomm Adreno Vulkan
// driver on Windows on Arm presents only a black window.
func graphicsAPI(c gogpu.Config) gogpu.Config {
	if os.Getenv("GOGPU_GRAPHICS_API") == "" {
		c = c.WithGraphicsAPI(gogpu.GraphicsAPIDX12)
	}
	return c
}
