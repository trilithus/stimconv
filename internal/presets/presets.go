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

// Builtin is a read-only preset shipped with stimconv. Its Name is shown in
// brackets; without them it is the name used in output folder names.
type Builtin struct {
	Name string
	Desc string
	cfg  func() config.Config
}

// Config returns the built-in preset's configuration.
func (b Builtin) Config() config.Config { return b.cfg() }

// normalized are the settings of the *-normalized presets: each track scaled
// to the full volume range (99th percentile = 1), the volume weighted by
// waveform and carrier through the circuit model, quiet passages lifted.
func normalized(topology string) func() config.Config {
	return func() config.Config {
		c := config.Default()
		c.Topology = topology
		c.Normalize, c.Gamma = "p99", 0.85
		c.Circuit.Enabled, c.Intensity = true, "effective"
		return c
	}
}

func original(topology, ifc string) func() config.Config {
	return func() config.Config {
		c := config.Default() // config.Default holds the original settings
		c.Topology, c.IFC = topology, ifc
		return c
	}
}

// Builtins lists the built-in presets; the first is selected at start and
// matches config.Default.
var Builtins = []Builtin{
	{"[tri-original]", "Tri-phase. Volume follows the track exactly as recorded. Where A and B use different carriers, their beat is imitated with pulses, as on a shared common electrode.", original("joined", "beat")},
	{"[tri-original-smooth]", "Tri-phase. As tri-original, but without imitating the beat between different carriers: the output stays continuous.", original("joined", "off")},
	{"[tri-normalized]", "Tri-phase. Scales each track to the full volume range and evens out quiet passages.", normalized("joined")},
	{"[quad-original]", "Quad-phase. Volume follows the track exactly as recorded; quiet tracks stay quiet.", original("dual", "beat")},
	{"[quad-normalized]", "Quad-phase. Scales each track to the full volume range and evens out quiet passages.", normalized("dual")},
}

// Default is the built-in preset selected at start.
var Default = Builtins[0].Name

// DefaultName is Default as used in output folder names.
var DefaultName = FolderName(Default)

// Lookup returns the built-in preset called name (with or without brackets).
func Lookup(name string) (Builtin, bool) {
	n := "[" + strings.Trim(strings.TrimSpace(name), "[]") + "]"
	for _, b := range Builtins {
		if strings.EqualFold(b.Name, n) {
			return b, true
		}
	}
	return Builtin{}, false
}

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

// Folder returns the preset directory, creating it if needed.
func Folder() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return d, os.MkdirAll(d, 0o755)
}

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
	case strings.EqualFold(name, "default"):
		return errors.New(`"default" is reserved`)
	case windowsReserved[strings.ToUpper(name)]:
		return fmt.Errorf("%q is a reserved device name on Windows", name)
	}
	if _, ok := Lookup(name); ok {
		return fmt.Errorf("%q is the name of a built-in preset", name)
	}
	return nil
}

// FolderName is the preset's name as used in output folder names: a
// built-in's name without brackets, a saved preset's name, or the default's
// for anything else.
func FolderName(name string) string {
	if b, ok := Lookup(name); ok {
		return strings.Trim(b.Name, "[]")
	}
	if name == "" || ValidateName(name) != nil {
		return strings.Trim(Builtins[0].Name, "[]")
	}
	return name
}

func path(name string) (string, error) {
	name = strings.TrimSpace(name)
	if b, ok := Lookup(name); ok {
		return "", fmt.Errorf("the built-in preset %s cannot be changed", b.Name)
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

// List returns the built-in presets followed by the saved presets in name
// order.
func List() []string {
	var names []string
	for _, b := range Builtins {
		names = append(names, b.Name)
	}
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

// Load returns the named preset (built-in or saved) merged over the defaults.
func Load(name string) (config.Config, error) {
	if b, ok := Lookup(name); ok {
		return b.Config(), nil
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
