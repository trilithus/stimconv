package config

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// PEMType labels the settings block a settings report ends with:
// -----BEGIN STIMCONV SETTINGS----- … -----END STIMCONV SETTINGS-----.
const PEMType = "STIMCONV SETTINGS"

// Embedded is the content of a settings block: the complete configuration
// (every field, defaults included) and what produced it.
type Embedded struct {
	Version string `json:"stimconv_version"` // stimconv version that wrote the block
	Preset  string `json:"preset"`           // preset name at the time, informational
	Config  Config `json:"config"`
}

// EncodePEM returns cfg as a PEM settings block (base64 of Embedded's JSON).
// Version and Preset are repeated as PEM headers so the block can be told
// apart without decoding it.
func EncodePEM(cfg Config, version, preset string) ([]byte, error) {
	data, err := json.Marshal(Embedded{Version: version, Preset: preset, Config: cfg})
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: PEMType,
		Headers: map[string]string{"Version": version, "Preset": preset}, Bytes: data}), nil
}

// HasPEM reports whether data contains a settings block.
func HasPEM(data []byte) bool { return bytes.Contains(data, []byte("-----BEGIN "+PEMType+"-----")) }

// DecodePEM extracts the first settings block from data, such as a settings
// report. Fields missing from the block (written by an older stimconv) keep
// their current defaults, and the result is validated.
func DecodePEM(data []byte) (Embedded, error) {
	for rest := data; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			return Embedded{}, errors.New("no " + PEMType + " block found")
		}
		if b.Type != PEMType {
			continue
		}
		e := Embedded{Config: Default()}
		if err := json.Unmarshal(b.Bytes, &e); err != nil {
			return Embedded{}, fmt.Errorf("settings block: %w", err)
		}
		return e, e.Config.Validate()
	}
}
