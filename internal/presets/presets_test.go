package presets

import (
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
	if got := List(); len(got) != 2 || got[0] != Default || got[1] != "mine" {
		t.Fatalf("List = %v", got)
	}
	l, err := Load("mine")
	if err != nil || l.Topology != "joined" || l.Gamma != 2 {
		t.Fatalf("Load = %+v, %v", l, err)
	}
	if d, _ := Load(Default); d.Gamma != config.Default().Gamma {
		t.Fatal("default preset not default")
	}
	if Save(Default, c) == nil || Delete(Default) == nil {
		t.Fatal("default preset must be read-only")
	}
	if err := Delete("mine"); err != nil || len(List()) != 1 {
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
	if FolderName(Default) != "default" || FolderName("my-preset") != "my-preset" || FolderName("bad name") != "default" {
		t.Error("FolderName")
	}
}
