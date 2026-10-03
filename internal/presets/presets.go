// Package presets stores named configurations as JSON files that are
// interchangeable with --config files.
package presets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/trilithus/stimconv/internal/config"
)

// Default is the pseudo preset that resets everything to config.Default.
const Default = "[default]"

// Dir is the preset directory. Empty means <user config dir>/stimconv/presets.
var Dir string

func dir() (string, error) {
	if Dir != "" {
		return Dir, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "stimconv", "presets"), nil
}

// DefaultName is the name the [default] pseudo preset uses in output paths.
const DefaultName = "default"

var validName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// windowsReserved are device names Windows refuses as file names, with or
// without an extension.
var windowsReserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// ValidateName reports why name cannot be used as a preset name, or nil.
// Names become file and folder names, so they are limited to
// [A-Za-z0-9_-], at most 64 characters, on every platform.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("enter a preset name")
	case !validName.MatchString(name):
		return errors.New("use only letters A-Z, digits, '-' and '_' (max 64 characters)")
	case strings.EqualFold(name, DefaultName):
		return errors.New(`"default" is reserved for the [default] preset`)
	case windowsReserved[strings.ToUpper(name)]:
		return fmt.Errorf("%q is a reserved device name on Windows", name)
	}
	return nil
}

// FolderName is the preset's name as used in output folder names.
func FolderName(name string) string {
	if name == "" || name == Default || ValidateName(name) != nil {
		return DefaultName
	}
	return name
}

func path(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == Default {
		return "", errors.New("the [default] preset cannot be changed")
	}
	if err := ValidateName(name); err != nil {
		return "", err
	}
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+".json"), nil
}

// List returns Default followed by the saved presets in name order.
func List() []string {
	names := []string{Default}
	d, err := dir()
	if err != nil {
		return names
	}
	m, _ := filepath.Glob(filepath.Join(d, "*.json"))
	sort.Strings(m)
	for _, p := range m {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	return names
}

// Load returns the named preset merged over the defaults.
func Load(name string) (config.Config, error) {
	if name == Default {
		return config.Default(), nil
	}
	p, err := path(name)
	if err != nil {
		return config.Default(), err
	}
	return config.Load(p)
}

// Save writes cfg under name, replacing an existing preset.
func Save(name string, cfg config.Config) error {
	p, err := path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return cfg.Save(p)
}

// Delete removes a saved preset.
func Delete(name string) error {
	p, err := path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
