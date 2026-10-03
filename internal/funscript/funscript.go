// Package funscript writes restim-compatible .funscript files.
package funscript

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Action is one funscript point: time in milliseconds, position 0..100.
type Action struct {
	At  int64 `json:"at"`
	Pos int   `json:"pos"`
}

// Script is the on-disk funscript document.
type Script struct {
	Version  string   `json:"version"`
	Inverted bool     `json:"inverted"`
	Range    int      `json:"range"`
	Actions  []Action `json:"actions"`
}

// Axis is a named time series in physical units together with the
// range that maps it onto funscript positions 0..100.
type Axis struct {
	Name     string    // funscript suffix, e.g. "volume"
	Min, Max float64   // physical value mapped to pos 0 and pos 100
	Times    []float64 // seconds
	Values   []float64 // physical units
}

// ToPos converts a physical value to a funscript position (float, unclamped rounding later).
func (a *Axis) ToPos(v float64) float64 {
	if a.Max == a.Min {
		return 0
	}
	p := (v - a.Min) / (a.Max - a.Min) * 100
	return math.Max(0, math.Min(100, p))
}

// Actions converts the axis to simplified funscript actions. epsilon is the
// maximum deviation in position units allowed by the Ramer-Douglas-Peucker
// simplification (0 disables simplification).
func (a *Axis) Actions(epsilon float64) []Action {
	n := len(a.Times)
	if n == 0 {
		return nil
	}
	pos := make([]float64, n)
	ms := make([]float64, n)
	for i := range a.Times {
		pos[i] = a.ToPos(a.Values[i])
		ms[i] = a.Times[i] * 1000
	}
	keep := Simplify(ms, pos, epsilon)
	out := make([]Action, 0, len(keep))
	var lastAt int64 = -1
	for _, i := range keep {
		at := int64(math.Round(ms[i]))
		if at == lastAt {
			continue
		}
		lastAt = at
		out = append(out, Action{At: at, Pos: int(math.Round(pos[i]))})
	}
	return out
}

// Simplify returns the indices of the points kept by Ramer-Douglas-Peucker
// with vertical (value) distance. First and last points are always kept.
func Simplify(x, y []float64, epsilon float64) []int {
	n := len(x)
	if n <= 2 || epsilon <= 0 {
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	keep := make([]bool, n)
	keep[0], keep[n-1] = true, true
	type span struct{ a, b int }
	stack := []span{{0, n - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s.b-s.a < 2 {
			continue
		}
		maxD, maxI := -1.0, -1
		dx := x[s.b] - x[s.a]
		for i := s.a + 1; i < s.b; i++ {
			var yl float64
			if dx == 0 {
				yl = y[s.a]
			} else {
				yl = y[s.a] + (y[s.b]-y[s.a])*(x[i]-x[s.a])/dx
			}
			if d := math.Abs(y[i] - yl); d > maxD {
				maxD, maxI = d, i
			}
		}
		if maxD > epsilon {
			keep[maxI] = true
			stack = append(stack, span{s.a, maxI}, span{maxI, s.b})
		}
	}
	var idx []int
	for i, k := range keep {
		if k {
			idx = append(idx, i)
		}
	}
	return idx
}

// Write stores actions as a funscript file.
func Write(path string, actions []Action) error {
	s := Script{Version: "1.0", Range: 100, Actions: actions}
	if s.Actions == nil {
		s.Actions = []Action{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Read loads a funscript file (used by tests and tooling).
func Read(path string) (*Script, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Script
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}
