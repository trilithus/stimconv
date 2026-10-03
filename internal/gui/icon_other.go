//go:build !windows

package gui

func setWindowIcon(string) {} // the window icon is set via gogpu's Config.Icon
