package config

import (
	"encoding/json"
	"testing"
)

// flatten collects the JSON leaf paths of v.
func flatten(prefix string, v any, out map[string]bool) {
	if m, ok := v.(map[string]any); ok {
		for k, c := range m {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flatten(p, c, out)
		}
		return
	}
	out[prefix] = true
}

func TestOptionsCoverConfig(t *testing.T) {
	b, _ := json.Marshal(Default())
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	leaves := map[string]bool{}
	flatten("", m, leaves)
	keys := map[string]bool{}
	for _, o := range Options {
		if keys[o.Key] {
			t.Errorf("duplicate option %s", o.Key)
		}
		keys[o.Key] = true
	}
	for k := range leaves {
		if !keys[k] {
			t.Errorf("config field %s has no option", k)
		}
	}
	for k := range keys {
		if !leaves[k] {
			t.Errorf("option %s matches no config field", k)
		}
	}
	if Options[0].Key != "topology" {
		t.Errorf("first option is %s, want topology", Options[0].Key)
	}
}

func TestOptionsGetSetRoundTrip(t *testing.T) {
	c := Default()
	d := Default()
	for _, o := range Options {
		switch o.Kind {
		case Float:
			o.Set(&c, o.Get(&c).(float64)+1)
		case Bool:
			o.Set(&c, !o.Get(&c).(bool))
		case Enum:
			// any choice other than the default
			for _, ch := range o.Choices {
				if ch.Value != o.Get(&d) {
					o.Set(&c, ch.Value)
					break
				}
			}
		}
		if o.Get(&c) == o.Get(&d) {
			t.Errorf("%s: Set had no effect or leaked into another config", o.Key)
		}
	}
}
