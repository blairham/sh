// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What the dialect says reaches the editor.
//
// The editor is only built where there is a terminal, so nothing else here
// can look at this — and a mutation that dropped the dialect's answer on the
// way survived every other test, because an answer thrown away looks exactly
// like a dialect that did not answer.
func TestTheDialectsMarkReachesTheEditor(t *testing.T) {
	for _, want := range []string{"^C", "", "<interrupted>"} {
		s := Shell{Editor: EditorStyle{Interrupt: want}}
		if got := s.newEditor().interrupt; got != want {
			t.Errorf("editor marks with %q, want %q", got, want)
		}
	}
}

// And so does everything it says about words, for the same reason: each of
// these is one assignment in one function, and an assignment left out looks
// exactly like a dialect that answers the other way.
func TestTheDialectsWordsReachTheEditor(t *testing.T) {
	const chars = "_-/"
	e := Shell{Editor: EditorStyle{
		WordCharacters:                         chars,
		KillToStartOfLineTakesTheWholeLine:     true,
		KillWordBeforeCursorUsesWordCharacters: true,
		ForwardWordStopsBeforeTheNextWord:      true,
		TransposeAtTheStartSwapsTheFirstTwo:    true,
	}}.newEditor()
	if e.wordChars != chars {
		t.Errorf("word characters are %q, want %q", e.wordChars, chars)
	}
	for _, c := range []struct {
		name string
		got  bool
	}{
		{"kill to the start takes the whole line", e.wholeLineKill},
		{"the kill before the cursor uses word characters", e.killBeforeCursorUsesWords},
		{"forward stops before the next word", e.forwardWordStopsBeforeNext},
		{"transpose acts at the start", e.transposeAtStart},
	} {
		if !c.got {
			t.Errorf("%s: the dialect's answer did not reach the editor", c.name)
		}
	}
	// The zero value has to arrive as the zero value too, rather than as
	// whatever the editor was built with.
	zero := Shell{}.newEditor()
	if zero.wordChars != "" || zero.wholeLineKill || zero.killBeforeCursorUsesWords ||
		zero.forwardWordStopsBeforeNext || zero.transposeAtStart {
		t.Errorf("a shell that said nothing got %+v", zero)
	}
}
