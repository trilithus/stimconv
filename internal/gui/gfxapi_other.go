//go:build !windows

package gui

import "github.com/gogpu/gogpu"

func graphicsAPI(c gogpu.Config) gogpu.Config { return c }
