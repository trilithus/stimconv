package main

import "testing"

func TestPresetName(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "default"},
		{[]string{"--config", "/x/soft_quad.json", "f.mp3"}, "soft_quad"},
		{[]string{"--config=C:/p/strong.json"}, "strong"},
		{[]string{"--config", "my config.json"}, "default"}, // not a valid preset name
	} {
		if got := presetName(c.args); got != c.want {
			t.Errorf("presetName(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
