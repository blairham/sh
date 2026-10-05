// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The four incremental searches as one walk (#5904): a direction and a
// matcher over a position, driven through the editor's own read the way a
// person drives it. Every expectation here was measured against zsh 5.9.2
// through a pty on 2026-10-04 — see search.go for the tables — with the
// history below, whose markers are what the probes looked for.

var walkHistory = []string{
	"echo alpha one",
	"echo bravo two",
	"echo alpha three",
	"echo charlie",
}

// zshWalkStyle is the part of the zsh dialect's answers these rows read.
var zshWalkStyle = HistoryStyle{
	SearchPrompt:                  "bck-i-search: %s_",
	SearchFailedPrompt:            "failing bck-i-search: %s_",
	SearchForwardPrompt:           "fwd-i-search: %s_",
	SearchForwardFailedPrompt:     "failing fwd-i-search: %s_",
	SearchInvalidPrompt:           "invalid bck-i-search: %s_",
	SearchForwardInvalidPrompt:    "invalid fwd-i-search: %s_",
	SearchForwardCursorAtMatchEnd: true,
	SearchIgnoresCaseUnlessTold:   true,
	SearchCaretAnchors:            true,
}

// walking reads one line through an editor holding the history, with the
// zsh answers above, `^X r`/`^X s` live, the shell's own matcher, and the
// pattern searches bound where the measurement bound them.
func walking(t *testing.T, style HistoryStyle, history []string, keys string) (*editor, string, string) {
	t.Helper()
	var out strings.Builder
	table := map[string]Binding{
		"\x18p": {Widget: WidgetPatternSearchHistoryBackward},
		"\x18n": {Widget: WidgetPatternSearchHistoryForward},
	}
	sh := Shell{
		History:     style,
		Editor:      EditorStyle{SearchOnControlX: true},
		Runner:      newTestRunner(nil),
		KeyBindings: func(Keymap) map[string]Binding { return table },
	}
	e := sh.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = append([]string(nil), history...)
	e.browsing = len(e.history)
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil && !strings.HasSuffix(keys, stopHere) {
		t.Fatalf("%q: %v", keys, err)
	}
	return e, line, out.String()
}

// stopHere ends a search without moving anything — `^X` is a key the search
// does not take, and `q` after it is one the prefix does nothing with — and
// leaves the line and the cursor where the search put them for the test to
// read. Input that simply ran out would abandon the search and put the line
// back.
const stopHere = "\x18q"

const up = "\x1b[A"

func TestTheSearchTurnsRoundWithoutMoving(t *testing.T) {
	for _, tc := range []struct {
		name, keys, want string
		pos              int
	}{
		// C-s after C-r stays on the match, the cursor at its end.
		{"turned round", "\x12alpha\x12\x13", "echo alpha one", 10},
		// Then a step forward goes on to the next newer match.
		{"a step forward", "\x12alpha\x12\x13\x13", "echo alpha three", 10},
		// Past the newest it fails and keeps the line.
		{"past the newest", "\x12alpha\x12\x13\x13\x13", "echo alpha three", 10},
		// And C-r turns it back, the cursor at the start again.
		{"turned back", "\x12alpha\x12\x13\x13\x12", "echo alpha three", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No accepting key: the input runs out inside the search, which
			// abandons it, so the line and cursor are read off the run that
			// stops one key short and then checked against the accepted line.
			_, line, _ := walking(t, zshWalkStyle, walkHistory, tc.keys+"\r")
			if line != tc.want {
				t.Errorf("accepted %q, want %q", line, tc.want)
			}
			e, _, _ := walking(t, zshWalkStyle, walkHistory, tc.keys+stopHere)
			if e.pos != tc.pos {
				t.Errorf("cursor at %d, want %d", e.pos, tc.pos)
			}
		})
	}
}

// A forward search starts at the cursor of the entry being shown: from the
// end of `alpha one`, `br` is in `bravo two` and nothing nearer.
func TestAForwardSearchStartsFromTheEntryOnTheScreen(t *testing.T) {
	_, line, _ := walking(t, zshWalkStyle, walkHistory, up+up+up+up+"\x13br\r")
	if line != "echo bravo two" {
		t.Errorf("accepted %q, want echo bravo two", line)
	}
	// And from the end of `alpha three` there is no `t` ahead at all: the
	// `t` of `three` is behind the cursor.
	_, _, out := walking(t, zshWalkStyle, walkHistory, up+up+"\x13t\r")
	if !strings.Contains(out, "failing fwd-i-search: t_") {
		t.Errorf("drew %q, want the failing forward wording", out)
	}
}

// The line being typed is searched first, and a second match in one entry is
// a step of its own.
func TestTheSearchWalksPositionsNotEntries(t *testing.T) {
	e, _, _ := walking(t, zshWalkStyle, walkHistory, "echo zz charlie \x12ch"+stopHere)
	if string(e.line) != "echo zz charlie " || e.pos != 8 {
		t.Errorf("line %q at %d, want the typed line's own charlie at 8", string(e.line), e.pos)
	}
	e, _, _ = walking(t, zshWalkStyle, walkHistory, "echo zz charlie \x12ch\x12"+stopHere)
	if string(e.line) != "echo zz charlie " || e.pos != 1 {
		t.Errorf("line %q at %d, want the same line's echo at 1", string(e.line), e.pos)
	}
	dup := []string{"echo dup dup"}
	e, _, _ = walking(t, zshWalkStyle, dup, "\x12dup"+stopHere)
	if e.pos != 9 {
		t.Errorf("C-r dup at %d, want the second dup at 9", e.pos)
	}
	e, _, _ = walking(t, zshWalkStyle, dup, "\x12dup\x12"+stopHere)
	if e.pos != 5 {
		t.Errorf("a second C-r at %d, want the first dup at 5", e.pos)
	}
}

func TestControlXIsTheTwoPlainSearchesWhereTheDialectSaysSo(t *testing.T) {
	_, line, _ := walking(t, zshWalkStyle, walkHistory, "\x18rbra\r")
	if line != "echo bravo two" {
		t.Errorf("^X r bra gave %q", line)
	}
	_, line, _ = walking(t, zshWalkStyle, walkHistory, up+up+up+up+"\x18sbr\r")
	if line != "echo bravo two" {
		t.Errorf("^X s br gave %q", line)
	}
}

func TestAPatternSearchReadsTheQueryAsAPattern(t *testing.T) {
	for _, tc := range []struct {
		name, keys, line string
		pos              int
	}{
		{"a wildcard", "\x18pb*o", "echo bravo two", 5},
		// The match that ends last, and of those the one starting first: the
		// `h` of `echo` rather than charlie's.
		{"back is the mirror of forward", "\x18ph*e", "echo charlie", 2},
		{"a plain letter is the last one", "\x18ph", "echo charlie", 6},
		// Forward takes the longest match from where it starts.
		{"forward is the longest", up + up + up + up + "\x18nv*", "echo bravo two", 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := walking(t, zshWalkStyle, walkHistory, tc.keys+stopHere)
			if string(e.line) != tc.line || e.pos != tc.pos {
				t.Errorf("line %q at %d, want %q at %d", string(e.line), e.pos, tc.line, tc.pos)
			}
		})
	}
	// A plain search over the same keys reads `*` as a character.
	_, _, out := walking(t, zshWalkStyle, walkHistory, "\x12b*o\r")
	if !strings.Contains(out, "failing bck-i-search: b*o_") {
		t.Errorf("a plain search drew %q, want it failing", out)
	}
}

// What a widget calling each search by name is answered with — see
// searchEnd.status for the measurement.
func TestEachSearchEndsWithItsStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    Widget
		keys string
		want int
		key  string
	}{
		{"forward found", WidgetSearchHistoryForward, up + up + up + up + "thr\r", 0, "\r"},
		{"forward failing", WidgetSearchHistoryForward, "zzz\r", 1, "\r"},
		{"forward abandoned", WidgetSearchHistoryForward, up + "thr\a", 3, "\a"},
		{"pattern found", WidgetPatternSearchHistoryBackward, "b*o\r", 0, "\r"},
		{"pattern failing", WidgetPatternSearchHistoryBackward, "q*q\r", 1, "\r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			sh := Shell{History: zshWalkStyle, Runner: newTestRunner(nil)}
			e := sh.newEditor(t.Context(), nil)
			e.history = append([]string(nil), walkHistory...)
			e.browsing = len(e.history)
			e.drafts = map[int][]rune{}
			// Each leading Up puts the walk one entry further back before the
			// search is called by name with the rest of the keys to read.
			rest := tc.keys
			for strings.HasPrefix(rest, up) {
				e.browsing--
				rest = strings.TrimPrefix(rest, up)
			}
			if e.browsing < len(e.history) {
				e.line = []rune(e.history[e.browsing])
				e.pos = len(e.line)
			}
			e.in, e.out = typing(rest), &out
			got, _ := editorActions{e: e, prompt: drawPrompt("$ ")}.Perform(tc.w, e.give())
			if got.Status != tc.want {
				t.Errorf("status %d, want %d", got.Status, tc.want)
			}
			if got.Keys != tc.key {
				t.Errorf("keys %q, want %q", got.Keys, tc.key)
			}
		})
	}
}

// A query with no upper-case letter ignores case, one with any is read as
// written, and a leading `^` anchors to the start of the entry — zsh's
// answers, measured on 2026-10-04 (#5932). The history is built so that the
// ranked fallback, which has a smart case of its own, would land somewhere
// else: `r_a_n` scores higher as a subsequence than `xRAN` does, so only the
// contiguous pass ignoring case reaches `xRAN`.
func TestALowerCaseQueryIgnoresCaseAndACaretAnchors(t *testing.T) {
	history := []string{"echo xRAN one", "echo r_a_n two"}
	for _, tc := range []struct {
		name, keys, line string
		pos              int
	}{
		{"lower case ignores case", "\x12ran", "echo xRAN one", 6},
		{"a caret anchors", "\x12^ec", "echo r_a_n two", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := walking(t, zshWalkStyle, history, tc.keys+stopHere)
			if string(e.line) != tc.line || e.pos != tc.pos {
				t.Errorf("line %q at %d, want %q at %d", string(e.line), e.pos, tc.line, tc.pos)
			}
		})
	}
	// A caret anywhere else is a letter like any other.
	_, _, out := walking(t, zshWalkStyle, history, "\x12o ^\r")
	if !strings.Contains(out, "failing bck-i-search: o ^_") {
		t.Errorf("drew %q, want a later caret failing", out)
	}
	// Any upper-case letter means the case was meant.
	_, _, out = walking(t, zshWalkStyle, history, "\x12ECHO\r")
	if !strings.Contains(out, "failing bck-i-search: ECHO_") {
		t.Errorf("drew %q, want ECHO failing", out)
	}
	// And a dialect that says neither reads the caret as a character.
	_, _, out = walking(t, HistoryStyle{}, history, "\x12^ec\r")
	if !strings.Contains(out, defaultFailedText("^ec")) {
		t.Errorf("drew %q, want ^ec failing without the knob", out)
	}
}
