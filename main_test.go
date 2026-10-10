package main

import "testing"

func TestPresetName(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "tri-original"},
		{[]string{"--preset", "tri-original"}, "tri-original"},
		{[]string{"--preset=[tri-normalized]", "--config", "x.json"}, "tri-normalized"},
		{[]string{"-preset", "mine"}, "mine"},
		{[]string{"--config", "/x/soft_quad.json", "f.mp3"}, "soft_quad"},
		{[]string{"--config=C:/p/strong.json"}, "strong"},
		{[]string{"--config", "my config.json"}, "tri-original"}, // not a valid preset name
	} {
		if got := presetName(c.args); got != c.want {
			t.Errorf("presetName(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
