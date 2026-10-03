package gui

import "strings"

// level is the channel of a console entry.
type level int

const (
	levelInfo level = iota
	levelWarn
	levelError
)

// logEntry is one console line.
type logEntry struct {
	lvl  level
	text string
}

// classify splits the "warning:" / "error:" prefix that pipeline output
// (and the CLI) use into a level.
func classify(line string) logEntry {
	for _, p := range []struct {
		prefix string
		lvl    level
	}{{"warning:", levelWarn}, {"error:", levelError}} {
		if len(line) >= len(p.prefix) && strings.EqualFold(line[:len(p.prefix)], p.prefix) {
			return logEntry{p.lvl, strings.TrimSpace(line[len(p.prefix):])}
		}
	}
	return logEntry{levelInfo, line}
}

// symbolSet is one way of marking the channel in the console.
type symbolSet struct {
	name string
	sym  [3]string // info, warning, error
}

// symbolTiers are tried in order; the first the UI font can draw wins.
var symbolTiers = []symbolSet{
	{"emoji", [3]string{"ℹ️", "⚠️", "❌"}},
	{"unicode", [3]string{"●", "⚠", "✗"}},
	{"text", [3]string{"[info]", "[warning]", "[error]"}},
}

// tagSet is used for text that leaves the app (clipboard).
var tagSet = symbolTiers[len(symbolTiers)-1]

// consoleSymbols returns the symbol tier the console uses: the first one
// (emoji → unicode → text tags) whose characters the UI font draws.
//
// The choice is made at test time, not at run time: TestSymbolTiers draws
// every tier offscreen against the font gogpu/ui embeds (Inter) and checks
// that this is the first drawable one. Doing that offscreen rendering inside
// the app broke all text rendering in the GPU window on Windows, so the app
// must never call renderGlyphs.
func consoleSymbols() symbolSet { return symbolTiers[consoleTier] }

// consoleTier is the index into symbolTiers verified by TestSymbolTiers.
const consoleTier = 1 // unicode: Inter has no emoji
