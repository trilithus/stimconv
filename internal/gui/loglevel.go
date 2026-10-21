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
// TestSymbolTiers renders every tier with Fyne's default theme and checks
// that consoleTier is the first drawable one.
func consoleSymbols() symbolSet { return symbolTiers[consoleTier] }

// consoleTier is the index into symbolTiers verified by TestSymbolTiers.
const consoleTier = 0 // emoji: Fyne bundles an emoji font
