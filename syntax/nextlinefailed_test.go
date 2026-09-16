// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	. "github.com/blairham/sh/syntax"
)

// A line that did not read hands back none of itself — #3194.
//
// NextLine's contract is that the *line* is the unit: everything up to the
// newline is parsed before any of it runs, and a failure anywhere in it
// discards the whole line. The loop honored that only where the failure landed
// *between* statements, because parseStmt returns nil there and nothing is
// appended. Where the failure landed inside the last construct on the line,
// parseStmt handed back the tree it had built so far and it was appended and
// run.
//
// Both shapes are below, and the pair is the point: a probe with only the
// second would have passed throughout the life of the bug.
func TestNextLineHandsBackNothingFromAFailedLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
	}{
		// The failure lands between two statements: `echo one` is complete
		// and `{ fi; }` is not a command. This shape was always right.
		{"between statements", "echo one; { fi; }"},
		// The failure lands inside the construct that began the line, so the
		// only tree there is a half-built one. This shape was the bug: the
		// refused definition was bound, and an `f` defined earlier was
		// replaced by a function with an empty body that answered 0 and
		// printed nothing.
		{"inside the construct", "f() {\n}"},
		// And the same with something before it, to show it is the failing
		// line that goes and not the text: `echo one` is a line of its own
		// and survives.
		{"a later line", "echo one\nf() {\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := NewParser(tc.src, Core())
			failed := false
			for {
				f, ok := p.NextLine()
				if !ok {
					break
				}
				if p.Err() != nil {
					failed = true
					if len(f.Stmts) != 0 {
						t.Fatalf("a line that did not read handed back %d statements", len(f.Stmts))
					}
				}
			}
			if p.Err() == nil {
				t.Fatalf("%q was expected not to read", tc.src)
			}
			if !failed {
				// The failure has to arrive on a line NextLine handed back,
				// or the assertion above never ran and the test is green for
				// the wrong reason.
				t.Fatal("the failure never arrived on a line that was handed back")
			}
		})
	}
}

// The lines *before* a failing one are still handed back, which is the half
// that keeps the rule above from being "a failed text runs nothing".
func TestNextLineKeepsTheLinesBeforeAFailure(t *testing.T) {
	t.Parallel()
	p := NewParser("echo one\nf() {\n}", Core())
	first, ok := p.NextLine()
	if !ok || len(first.Stmts) != 1 {
		t.Fatalf("the first line: ok=%v stmts=%d", ok, len(first.Stmts))
	}
	if p.Err() != nil {
		t.Fatalf("the first line already failed: %v", p.Err())
	}
}
