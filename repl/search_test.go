// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Reverse incremental search, driven the way the rest of the editor is: bytes
// in, drawn bytes out, no terminal.
//
// The expectations here are measured. Every screen this file asserts against
// was recorded from bash 5.3.15 and zsh 5.9.2 under a pseudo-terminal on
// 2026-09-05 and reconstructed from the bytes with a terminal model, because
// `C-r` is a screen behavior and reading the raw stream by eye reads the
// optimisations rather than the result.

// searching runs a line through an editor that already has a history.
func searching(t *testing.T, style HistoryStyle, history []string, keys string) (*editor, string, string) {
	t.Helper()
	var out strings.Builder
	e := &editor{
		in: strings.NewReader(keys), out: &out,
		history:      history,
		searchPrompt: style.SearchPrompt,
		searchFailed: style.SearchFailedPrompt,
		searchBelow:  style.SearchBelowTheLine,
		width:        func() int { return 80 },
	}
	line, _ := e.readLine(drawPrompt("$ "))
	return e, line, out.String()
}

var fourLines = []string{"echo one", "echo two", "git status", "ls -la"}

// The walk: the newest match first, then older ones, and a floor at the end.
func TestReverseSearchWalksBackThroughTheMatches(t *testing.T) {
	// `C-r`, `echo`, then Enter takes the newest line containing it.
	_, line, _ := searching(t, HistoryStyle{}, fourLines, "\x12echo\r")
	if line != "echo two" {
		t.Errorf("C-r echo gave %q, want the newest match", line)
	}
	// A second `C-r` steps to the one before it.
	_, line, _ = searching(t, HistoryStyle{}, fourLines, "\x12echo\x12\r")
	if line != "echo one" {
		t.Errorf("a second C-r gave %q, want the older match", line)
	}
	// And a third has nowhere to go. The line that did match stays, which is
	// measured: both shells change only the wording and ring the bell.
	e, line, out := searching(t, HistoryStyle{}, fourLines, "\x12echo\x12\x12\r")
	if line != "echo one" {
		t.Errorf("an exhausted search gave %q, want the last match kept", line)
	}
	if !strings.Contains(out, defaultFailedText("echo")) {
		t.Errorf("drew %q, want the failed wording", out)
	}
	if !strings.Contains(out, bell) {
		t.Error("an exhausted search rang no bell")
	}
	if e.pos != 0 {
		t.Errorf("cursor at %d, want it left at the match", e.pos)
	}
}

// The cursor lands where the match is, not at either end of the line.
//
// Measured in both shells: `C-r cho` on `echo two` leaves the cursor on the
// `c`, which is what makes a search also a way of getting to a place in a long
// command.
func TestTheCursorLandsOnTheMatch(t *testing.T) {
	e, _, _ := searching(t, HistoryStyle{}, fourLines, "\x12cho\r")
	if e.pos != 1 {
		t.Errorf("cursor at %d, want 1 — the offset of `cho` in `echo two`", e.pos)
	}
	// Runes and not bytes. A match after a wide character is one column per
	// character to the right of the start, not one per byte.
	e, _, _ = searching(t, HistoryStyle{}, []string{"echo 日本 tail"}, "\x12tail\r")
	if e.pos != 8 {
		t.Errorf("cursor at %d, want 8 — `tail` is the ninth character, not the eleventh byte", e.pos)
	}
}

// Extending the query stays on the entry it is already showing rather than
// jumping back to a newer one that also matches.
func TestExtendingTheQueryDoesNotJumpForward(t *testing.T) {
	// `C-r echo` finds `echo two`; `C-r` again steps to `echo one`; typing a
	// space then `o` must stay there rather than returning to `echo two`.
	_, line, _ := searching(t, HistoryStyle{}, fourLines, "\x12echo\x12 o\r")
	if line != "echo one" {
		t.Errorf("gave %q, want the older entry kept once the query still matches", line)
	}
}

// Backspace shortens the query, and on an empty one it is not a delete.
func TestBackspaceInSearchShortensTheQuery(t *testing.T) {
	// `zz` matches nothing; backspacing to `z` still matches nothing, and
	// backspacing again to `e` finds a line.
	_, line, _ := searching(t, HistoryStyle{}, fourLines, "\x12ec\x7f\x7f\x12\r")
	if line != "echo one" {
		t.Errorf("gave %q, want an emptied query to search again from the entry shown", line)
	}
	// With nothing typed there is nothing to take away: the line is a search
	// result rather than something being edited, so the bell is the answer.
	e, line, out := searching(t, HistoryStyle{}, fourLines, "\x12\x7f\x07\r")
	if !strings.Contains(out, bell) {
		t.Error("backspace on an empty query rang no bell")
	}
	if line != "" || len(e.line) != 0 {
		t.Errorf("gave %q, want the empty line the search started from", line)
	}
}

// `C-g` puts back the line the search started from, cursor and all.
func TestAbortRestoresTheLine(t *testing.T) {
	e, line, _ := searching(t, HistoryStyle{}, fourLines, "abc\x12echo\x07\r")
	if line != "abc" {
		t.Errorf("C-g gave %q, want the half-typed line back", line)
	}
	if e.pos != 3 {
		t.Errorf("cursor at %d, want it back at the end of `abc`", e.pos)
	}
}

// A key that ends the search still means what it means.
//
// Measured in both shells, and the part a mode that swallowed its own exit key
// would get wrong: `C-r cho C-e` leaves the search, keeps `echo two`, *and*
// puts the cursor at the end of it.
func TestTheKeyThatEndsTheSearchStillActs(t *testing.T) {
	e, line, _ := searching(t, HistoryStyle{}, fourLines, "\x12cho\x05\r")
	if line != "echo two" {
		t.Errorf("gave %q, want the match kept", line)
	}
	if e.pos != len("echo two") {
		t.Errorf("cursor at %d, want C-e to have moved it to the end", e.pos)
	}
	// `C-k` from the match kills the rest of the line, which is what it does
	// anywhere: measured, `C-r cho C-k` in bash leaves `e`.
	_, line, _ = searching(t, HistoryStyle{}, fourLines, "\x12cho\x0b\r")
	if line != "e" {
		t.Errorf("C-k after a search gave %q, want `e`", line)
	}
	// And an arrow, which arrives as three bytes the escape reader has to see
	// in order — the one case where pushing a byte back has to leave the rest
	// of the sequence where it was.
	e, line, _ = searching(t, HistoryStyle{}, fourLines, "\x12cho\x1b[C\r")
	if line != "echo two" || e.pos != 2 {
		t.Errorf("Right after a search gave %q at %d, want `echo two` at 2", line, e.pos)
	}
}

// The search and the arrows are one walk: Up from a found entry goes to the
// one before it, and Down eventually comes back to what was being typed.
func TestTheArrowsCarryOnFromWhatTheSearchFound(t *testing.T) {
	_, line, _ := searching(t, HistoryStyle{}, fourLines, "\x12git\x1b[A\r")
	if line != "echo two" {
		t.Errorf("Up after a search gave %q, want the entry before `git status`", line)
	}
	_, line, _ = searching(t, HistoryStyle{}, fourLines, "half\x12git\x1b[B\x1b[B\r")
	if line != "half" {
		t.Errorf("Down twice after a search gave %q, want the typed line back", line)
	}
}

// A search of an empty history finds nothing and breaks nothing.
func TestSearchingAnEmptyHistory(t *testing.T) {
	e, line, out := searching(t, HistoryStyle{}, nil, "\x12ec\r")
	if line != "" || len(e.line) != 0 {
		t.Errorf("gave %q, want nothing found and the line untouched", line)
	}
	if !strings.Contains(out, defaultFailedText("ec")) {
		t.Errorf("drew %q, want the failed wording", out)
	}
}

// The two shapes, which is the dialect axis.
//
// bash replaces the prompt with the search; zsh keeps the prompt and the line
// where they are and puts the search on a row of its own below them.
func TestTheTwoShapesOfTheSearchOnScreen(t *testing.T) {
	bashish := HistoryStyle{
		SearchPrompt:       "(reverse-i-search)`%s': ",
		SearchFailedPrompt: "(failed reverse-i-search)`%s': ",
	}
	_, _, out := searching(t, bashish, fourLines, "\x12echo\x07\r")
	if !strings.Contains(out, "(reverse-i-search)`echo': echo two") {
		t.Errorf("drew %q, want the search where the prompt was, with the match after it", out)
	}
	if strings.Contains(out, "$ echo two") {
		t.Errorf("drew %q, want the prompt replaced rather than kept", out)
	}

	zshish := HistoryStyle{
		SearchPrompt:       "bck-i-search: %s_",
		SearchFailedPrompt: "failing bck-i-search: %s_",
		SearchBelowTheLine: true,
	}
	_, _, out = searching(t, zshish, fourLines, "\x12echo\x07\r")
	if !strings.Contains(out, "$ echo two") {
		t.Errorf("drew %q, want the prompt and the match kept where they were", out)
	}
	// On the row below, and the cursor comes back up to the line afterwards.
	if !strings.Contains(out, "\r\n\x1b[Kbck-i-search: echo_\r\x1b[1A") {
		t.Errorf("drew %q, want the search on a row below and the cursor returned", out)
	}
}

// Without a terminal width there is no second row to put anything on, so the
// dialect that draws below falls back to drawing in place.
//
// It is a real case rather than a hypothetical: the width comes from an ioctl
// on the input, and a session whose input will not answer has none.
func TestWithoutAWidthTheSearchIsDrawnInPlace(t *testing.T) {
	var out strings.Builder
	e := &editor{
		in: strings.NewReader("\x12echo\x07\r"), out: &out,
		history:      fourLines,
		searchPrompt: "bck-i-search: %s_",
		searchFailed: "failing bck-i-search: %s_",
		searchBelow:  true,
	}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\r\n\x1b[K") {
		t.Errorf("drew %q, want nothing placed on a row whose position is unknown", out.String())
	}
	if !strings.Contains(out.String(), "bck-i-search: echo_echo two") {
		t.Errorf("drew %q, want the search drawn in place instead", out.String())
	}
}

// The substrate's own wording names no shell, because the core does not know
// its successors — a default borrowed from one of them would make every other
// dialect look like a deviation from it.
func TestTheDefaultSearchWordingNamesNoShell(t *testing.T) {
	for _, s := range []string{defaultSearchPrompt, defaultSearchFailed} {
		for _, name := range []string{"bash", "zsh", "ksh", "dash", "i-search", "bck"} {
			if strings.Contains(s, name) {
				t.Errorf("the substrate's wording %q carries %q", s, name)
			}
		}
	}
	_, _, out := searching(t, HistoryStyle{}, fourLines, "\x12echo\x07\r")
	if !strings.Contains(out, defaultPromptText("echo")) {
		t.Errorf("drew %q, want the substrate's own wording where a dialect said nothing", out)
	}
}

func defaultPromptText(query string) string {
	return strings.Replace(defaultSearchPrompt, "%s", query, 1)
}

func defaultFailedText(query string) string {
	return strings.Replace(defaultSearchFailed, "%s", query, 1)
}
