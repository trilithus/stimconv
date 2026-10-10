package presets

import (
	"reflect"
	"strings"
	"testing"

	"github.com/trilithus/stimconv/internal/config"
)

func TestRoundTrip(t *testing.T) {
	Dir = t.TempDir()
	defer func() { Dir = "" }()
	c := config.Default()
	c.Topology, c.Gamma = "joined", 2
	if err := Save("mine", c); err != nil {
		t.Fatal(err)
	}
	nb := len(Builtins)
	if got := List(); len(got) != nb+1 || got[0] != Default || got[nb] != "mine" {
		t.Fatalf("List = %v", got)
	}
	l, err := Load("mine")
	if err != nil || l.Topology != "joined" || l.Gamma != 2 {
		t.Fatalf("Load = %+v, %v", l, err)
	}
	if d, _ := Load(Default); d.Gamma != config.Default().Gamma {
		t.Fatal("default preset not default")
	}
	for _, b := range Builtins {
		if Save(b.Name, c) == nil || Delete(b.Name) == nil || Save(FolderName(b.Name), c) == nil {
			t.Fatalf("built-in preset %s must be read-only", b.Name)
		}
	}
	if err := Delete("mine"); err != nil || len(List()) != nb {
		t.Fatal("delete failed")
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"mine", "Quad_soft-2", "a", strings.Repeat("x", 64)} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "has space", "dot.name", "../up", `back\slash`, "colon:", "ümlaut",
		strings.Repeat("x", 65), "default", "Default", "CON", "nul", "com1", "LPT9"} {
		if ValidateName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if FolderName(Default) != "tri-original" || FolderName("[tri-original]") != "tri-original" ||
		FolderName("my-preset") != "my-preset" || FolderName("bad name") != "tri-original" {
		t.Error("FolderName")
	}
}

// The built-ins differ only in what they are meant to: topology, the
// original (abs, gamma 1, no circuit, current) vs normalized (p99, gamma
// 0.85, circuit, effective) volume, and the beat imitation of tri-original.
func TestBuiltins(t *testing.T) {
	if c, _ := Load(Default); !reflect.DeepEqual(c, config.Default()) {
		t.Errorf("default built-in %s differs from config.Default", Default)
	}
	for _, c := range []struct {
		name, topology, normalize, intensity, ifc string
		gamma                                     float64
		circuit                                   bool
	}{
		{"[quad-original]", "dual", "abs", "current", "beat", 1, false},
		{"[quad-normalized]", "dual", "p99", "effective", "beat", 0.85, true},
		{"[tri-original]", "joined", "abs", "current", "beat", 1, false},
		{"[tri-original-smooth]", "joined", "abs", "current", "off", 1, false},
		{"[tri-normalized]", "joined", "p99", "effective", "beat", 0.85, true},
		{"[mono-original]", "joined", "abs", "current", "off", 1, false},
	} {
		b, ok := Lookup(c.name)
		if !ok {
			t.Errorf("%s missing", c.name)
			continue
		}
		g := b.Config()
		if g.Topology != c.topology || g.Normalize != c.normalize || g.Intensity != c.intensity || g.IFC != c.ifc ||
			g.Gamma != c.gamma || g.Circuit.Enabled != c.circuit || g.RefLevel != 0 || g.Validate() != nil {
			t.Errorf("%s = %+v", c.name, g)
		}
		if b.Desc == "" || ValidateName(FolderName(c.name)) == nil {
			t.Errorf("%s: needs a description and a reserved folder name", c.name)
		}
	}
	for _, b := range Builtins {
		if want := map[bool]string{true: "ab", false: "track"}[b.Name == "[mono-original]"]; b.Config().Position != want {
			t.Errorf("%s: position %q, want %q", b.Name, b.Config().Position, want)
		}
	}
	if _, ok := Lookup("tri-original"); !ok {
		t.Error("Lookup without brackets")
	}
}
