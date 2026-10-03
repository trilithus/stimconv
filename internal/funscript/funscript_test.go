package funscript

import (
	"path/filepath"
	"testing"
)

func TestSimplifyKeepsEndpointsAndCorners(t *testing.T) {
	x := []float64{0, 1, 2, 3, 4, 5, 6}
	y := []float64{0, 1, 2, 3, 2, 1, 0}
	idx := Simplify(x, y, 0.1)
	want := []int{0, 3, 6}
	if len(idx) != len(want) {
		t.Fatalf("got %v want %v", idx, want)
	}
	for i := range want {
		if idx[i] != want[i] {
			t.Fatalf("got %v want %v", idx, want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	a := &Axis{Name: "frequency", Min: 500, Max: 1000,
		Times:  []float64{0, 0.5, 1.0},
		Values: []float64{500, 750, 2000}}
	acts := a.Actions(0)
	p := filepath.Join(t.TempDir(), "x.frequency.funscript")
	if err := Write(p, acts); err != nil {
		t.Fatal(err)
	}
	s, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []Action{{0, 0}, {500, 50}, {1000, 100}}
	if len(s.Actions) != 3 || s.Range != 100 {
		t.Fatalf("bad script %+v", s)
	}
	for i := range want {
		if s.Actions[i] != want[i] {
			t.Fatalf("action %d: got %+v want %+v", i, s.Actions[i], want[i])
		}
	}
}
