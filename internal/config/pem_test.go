package config

import (
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPEMRoundTrip(t *testing.T) {
	c := Default()
	c.Topology, c.Gamma, c.Ranges["frequency"] = "dual", 1.3, Range{Min: 300, Max: 2000}
	block, err := EncodePEM(c, "v1.2.3", "mine")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(block), "Version: v1.2.3") {
		t.Errorf("version header missing:\n%s", block)
	}
	// found inside surrounding Markdown, after another PEM block
	doc := "# report\n\n" + string(pem.EncodeToMemory(&pem.Block{Type: "OTHER", Bytes: []byte("x")})) + "```\n" + string(block) + "```\n"
	e, err := DecodePEM([]byte(doc))
	if err != nil || e.Version != "v1.2.3" || e.Preset != "mine" || !reflect.DeepEqual(e.Config, c) {
		t.Fatalf("decoded %+v, %v", e, err)
	}
}

// A block from an older stimconv lacks newer fields: they get the defaults.
// Invalid values are refused.
func TestPEMOlderAndInvalid(t *testing.T) {
	enc := func(js string) []byte {
		return pem.EncodeToMemory(&pem.Block{Type: PEMType, Bytes: []byte(js)})
	}
	e, err := DecodePEM(enc(`{"stimconv_version":"v0.0.1","config":{"topology":"dual"}}`))
	if err != nil || e.Config.Topology != "dual" || e.Config.Position != Default().Position {
		t.Errorf("older block: %+v, %v", e.Config, err)
	}
	if _, err := DecodePEM(enc(`{"config":{"topology":"hexa"}}`)); err == nil {
		t.Error("invalid topology accepted")
	}
	if _, err := DecodePEM([]byte("no block " + base64.StdEncoding.EncodeToString([]byte("{}")))); err == nil {
		t.Error("missing block not reported")
	}
}

// Files written before the longest-pulse train keep the fixed-rate train
// they were made with; new files round-trip the setting.
func TestOlderFilesKeepRateTrain(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"topology":"dual","continuous_pulse_hz":70}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(old)
	if err != nil || c.ContinuousPulse != "rate" || !c.Firmware.IsZero() || c.Topology != "dual" {
		t.Fatalf("old file: %+v, %v", c, err)
	}
	cur := filepath.Join(dir, "new.json")
	if err := Default().Save(cur); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(cur); err != nil || c.ContinuousPulse != "longest" {
		t.Fatalf("new file: %q, %v", c.ContinuousPulse, err)
	}
	e, err := DecodePEM([]byte("-----BEGIN " + PEMType + "-----\n" +
		base64.StdEncoding.EncodeToString([]byte(`{"config":{"gamma":1}}`)) + "\n-----END " + PEMType + "-----\n"))
	if err != nil || e.Config.ContinuousPulse != "rate" {
		t.Fatalf("old report block: %q", e.Config.ContinuousPulse)
	}
}
