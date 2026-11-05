package config

import (
	"fmt"
	"strconv"
	"strings"
)

// FirmwareVersion is a FOC-Stim firmware release, v<Major>.<Minor>.<Revision>.
// Settings files store it in this numeric form; reports show String.
// The zero value means a file did not record it (stimconv before firmware
// versions were tracked).
type FirmwareVersion struct {
	Major    int `json:"major"`
	Minor    int `json:"minor"`
	Revision int `json:"revision"`
}

var (
	// CurrentFirmware is the firmware the defaults were checked against
	// (reference/FOC-Stim; a pipeline test keeps them in sync).
	CurrentFirmware = FirmwareVersion{2, 0, 3}
	// MinFirmware is the oldest firmware stimconv supports. Files authored
	// for an older (or unrecorded) firmware are flagged when opened.
	MinFirmware = FirmwareVersion{2, 0, 0}
	// Firmware1 is the last firmware 1.x release (pairs with restim v1.66).
	Firmware1 = FirmwareVersion{1, 3, 2}
)

func (v FirmwareVersion) String() string {
	if v.IsZero() {
		return "unrecorded"
	}
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Revision)
}

// IsZero reports whether the version was not recorded.
func (v FirmwareVersion) IsZero() bool { return v == FirmwareVersion{} }

// Less reports whether v is an older release than o.
func (v FirmwareVersion) Less(o FirmwareVersion) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Revision < o.Revision
}

// ParseFirmware reads "v2.0.3", "2.0.3", "2.0" or "2" (missing parts are 0).
func ParseFirmware(s string) (FirmwareVersion, error) {
	t := strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.Split(t, ".")
	if t == "" || len(parts) > 3 {
		return FirmwareVersion{}, fmt.Errorf("invalid firmware version %q (want e.g. v2.0.3)", s)
	}
	var n [3]int
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return FirmwareVersion{}, fmt.Errorf("invalid firmware version %q (want e.g. v2.0.3)", s)
		}
		n[i] = x
	}
	v := FirmwareVersion{n[0], n[1], n[2]}
	if v.Major < 1 {
		return FirmwareVersion{}, fmt.Errorf("invalid firmware version %q: major version must be at least 1", s)
	}
	return v, nil
}

// Set implements flag.Value.
func (v *FirmwareVersion) Set(s string) error {
	p, err := ParseFirmware(s)
	if err == nil {
		*v = p
	}
	return err
}

// FirmwareNotice is the message shown when settings authored for firmware
// older than MinFirmware (or for an unrecorded one) are opened, or "".
func FirmwareNotice(authored FirmwareVersion) string {
	if !authored.IsZero() && !authored.Less(MinFirmware) {
		return ""
	}
	what := "FOC-Stim firmware " + authored.String()
	if authored.IsZero() {
		what = "an unrecorded FOC-Stim firmware (made before stimconv recorded it)"
	}
	return fmt.Sprintf("This file was originally authored for %s, but the minimum supported firmware is now %s. "+
		"Its settings were updated for firmware %s. Funscripts made with the original settings may play differently on current firmware, "+
		"so the original file may need to be re-imported and converted again.", what, MinFirmware, CurrentFirmware)
}

// UpgradeFirmware moves settings authored for firmware older than
// MinFirmware (or an unrecorded one) to CurrentFirmware and returns
// FirmwareNotice for the version they had, or "" if nothing changed.
func UpgradeFirmware(c *Config) string {
	msg := FirmwareNotice(c.Firmware)
	if msg != "" {
		c.Firmware = CurrentFirmware
	}
	return msg
}
