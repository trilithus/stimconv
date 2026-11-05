package config

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFirmware(t *testing.T) {
	for in, want := range map[string]FirmwareVersion{
		"v2.0.3": {2, 0, 3}, "2.3.1": {2, 3, 1}, " v1.3 ": {1, 3, 0}, "2": {2, 0, 0},
	} {
		if got, err := ParseFirmware(in); err != nil || got != want {
			t.Errorf("ParseFirmware(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "v", "2.x", "1.2.3.4", "-1", "0.9", "v2..1"} {
		if _, err := ParseFirmware(bad); err == nil {
			t.Errorf("ParseFirmware(%q) accepted", bad)
		}
	}
	if s := (FirmwareVersion{2, 3, 1}).String(); s != "v2.3.1" {
		t.Errorf("String = %s", s)
	}
	if !(FirmwareVersion{1, 9, 9}).Less(FirmwareVersion{2, 0, 0}) || (FirmwareVersion{2, 0, 3}).Less(FirmwareVersion{2, 0, 3}) ||
		!(FirmwareVersion{2, 0, 3}).Less(FirmwareVersion{2, 1, 0}) {
		t.Error("Less")
	}
}

// The settings block stores the firmware in numeric form and names it in
// readable form in its header.
func TestFirmwareInPEM(t *testing.T) {
	c := Default()
	c.Firmware = FirmwareVersion{2, 3, 1}
	block, err := EncodePEM(c, "v9", "p")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(block), "Firmware: v2.3.1") {
		t.Errorf("no readable firmware header:\n%s", block)
	}
	body := strings.Split(strings.SplitN(string(block), "\n\n", 2)[1], "-----END")[0]
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Config struct {
			Firmware map[string]int `json:"firmware"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Config.Firmware["major"] != 2 || m.Config.Firmware["minor"] != 3 || m.Config.Firmware["revision"] != 1 {
		t.Errorf("embedded firmware %v (%v) in %s", m.Config.Firmware, err, raw)
	}
	e, err := DecodePEM(block)
	if err != nil || e.Config.Firmware != c.Firmware {
		t.Errorf("round trip: %v, %v", e.Config.Firmware, err)
	}
}

// Opening settings authored for firmware below MinFirmware, or without a
// recorded one, names both versions and moves them to CurrentFirmware.
func TestUpgradeFirmware(t *testing.T) {
	for _, fw := range []FirmwareVersion{{}, Firmware1} {
		c := Default()
		c.Firmware = fw
		msg := UpgradeFirmware(&c)
		if msg == "" || c.Firmware != CurrentFirmware || !strings.Contains(msg, MinFirmware.String()) || !strings.Contains(msg, "re-imported") {
			t.Errorf("%v: %q, now %v", fw, msg, c.Firmware)
		}
		if !fw.IsZero() && !strings.Contains(msg, "firmware "+fw.String()) {
			t.Errorf("%v: message doesn't name the authored firmware: %s", fw, msg)
		}
	}
	for _, fw := range []FirmwareVersion{MinFirmware, CurrentFirmware, {2, 3, 1}} {
		c := Default()
		c.Firmware = fw
		if msg := UpgradeFirmware(&c); msg != "" || c.Firmware != fw {
			t.Errorf("%v flagged: %q", fw, msg)
		}
	}
}
