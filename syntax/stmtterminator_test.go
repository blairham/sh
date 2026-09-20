// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// What [syntax.Stmt].Term records — #3830, where a listing that follows the
// source had only the line numbers to follow and the source's own answer was
// the token.
//
// A tree test rather than a behavioral one, deliberately and with the reason
// said out loud: the engine that reads this field is the `$( … )` reprint in
// dialect/bash, and its rows are in dialect/bash/substitutionterminator_test.go.
// What is here is the field itself, over shapes that engine never reaches — a
// `&`, the last statement of a body, and a short loop that took its body's
// terminator. Those rows are **pins on the tree, not controls on a behavior**:
// no arrangement in this repository writes them out today, so no mutation of
// the printer can fail them.
func TestAStatementRecordsWhichTokenTerminatedIt(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []syntax.Terminator
	}{
		{
			"a semicolon between two", "a;b",
			[]syntax.Terminator{syntax.TerminatedBySemicolon, syntax.TerminatedByNothing},
		},
		{
			"a newline between two", "a\nb",
			[]syntax.Terminator{syntax.TerminatedByNewline, syntax.TerminatedByNothing},
		},
		{
			// Both written, which is the pair the whole issue turns on: the
			// position is the same question for either and only this field
			// tells them apart.
			"a semicolon and then a newline", "a;\nb",
			[]syntax.Terminator{syntax.TerminatedBySemicolon, syntax.TerminatedByNothing},
		},
		{
			// A comment is not in the tree, so what terminated `a` here is
			// the newline after the comment and not the `;`.
			"a comment holding the line open", "a # c\nb",
			[]syntax.Terminator{syntax.TerminatedByNewline, syntax.TerminatedByNothing},
		},
		{
			// `&` is a terminator with a field of its own already, so this
			// one says nothing rather than saying it twice.
			"an ampersand says nothing here", "a & b",
			[]syntax.Terminator{syntax.TerminatedByNothing, syntax.TerminatedByNothing},
		},
		{
			// The trailing newline of a file terminates the last statement,
			// where the end of the input does not.
			"a trailing newline", "a\n",
			[]syntax.Terminator{syntax.TerminatedByNewline},
		},
		{
			"nothing after the last", "a",
			[]syntax.Terminator{syntax.TerminatedByNothing},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := terminators(t, tc.src, syntax.Core())
			if len(got) != len(tc.want) {
				t.Fatalf("%q gave %d statements, want %d", tc.src, len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%q statement %d terminated by %d, want %d",
						tc.src, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A short loop body is a whole statement, terminator and all, so the `;` in
// `for i (a b) echo $i; echo end` belongs to `echo $i` and the loop is
// terminated by it too — which the parser already says with the position and
// which has to say the kind with it, or the loop claims a terminator it cannot
// name.
//
// A pin on the tree for the reason the suite above gives: no arrangement here
// prints a short loop through the branch that reads this.
func TestAShortLoopTakesItsBodysTerminator(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      syntax.Terminator
	}{
		{"a semicolon", "for i (a b) echo $i; echo end", syntax.TerminatedBySemicolon},
		{
			// Not a control on the inheritance, and the mutation says so:
			// a newline is still the token in hand when the loop's own
			// statement asks, so this row reaches the same answer without
			// the line that carries the kind out of the body.
			"a newline", "for i (a b) echo $i\necho end", syntax.TerminatedByNewline,
		},
		{
			// The control: a brace body is closed by its own `}` and takes
			// no terminator with it, so the loop has none — which is the
			// same rule the parser states for the position.
			"a brace body takes none", "for i (a b) { echo $i }", syntax.TerminatedByNothing,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := terminators(t, tc.src, short())
			if len(got) == 0 {
				t.Fatalf("%q gave no statements", tc.src)
			}
			if got[0] != tc.want {
				t.Errorf("%q loop terminated by %d, want %d", tc.src, got[0], tc.want)
			}
		})
	}
}

func terminators(t *testing.T, src string, d syntax.Dialect) []syntax.Terminator {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out := make([]syntax.Terminator, 0, len(f.Stmts))
	for _, st := range f.Stmts {
		out = append(out, st.Term)
	}
	return out
}
