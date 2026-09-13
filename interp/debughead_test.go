// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which commands other than the simple ones fire a DEBUG trap, and the panel
// gives two readings rather than a rule with exceptions. See
// interp.DebugTrapHeads for the table this is written from; the measurements
// are 2026-09-13, one construct per line under `trap 'echo D' DEBUG`.
//
// This engine fired at simple commands and nowhere else, which is neither
// reading — and the comment at the firing site asserted that a compound
// heading fires nothing, which no column in the panel does.

func headSem(heads DebugTrapHeads) func(*Semantics) {
	return func(s *Semantics) {
		s.TrapHasDebugCondition = Yes
		s.DebugTrapRunsInsideCalls = No
		s.DebugTrapRunsInSubshells = No
		s.DebugTrapFiresOnEnteringAFunction = No
		s.DebugTrapCompoundHeads = heads
	}
}

// Ds is how many heads the run wrote. Counting rather than matching, because
// what divides the readings is a number: the same script writes a different
// count under each and an identical set of other lines.
func Ds(t *testing.T, src string, heads DebugTrapHeads) int {
	t.Helper()
	out, errs, _ := trapRun(t, "trap 'echo D' DEBUG\n"+src, headSem(heads), Diagnostics{})
	if errs != "" {
		t.Fatalf("ran %q: stderr %q", src, errs)
	}
	return strings.Count(out, "D\n")
}

func TestWhichHeadsFireADebugTrap(t *testing.T) {
	for _, c := range []struct {
		name, src                string
		word, every, none, plain int
	}{
		// `case` fires a head under both readings, so it is the row that
		// says the two are not "some fire" against "none fire".
		{"case", "case x in x) :;; esac", 2, 2, 1, 1},
		// The list `for` is where they part: one reading counts passes and
		// the other counts the construct.
		{"a for over two items", "for i in a b; do :; done", 4, 3, 2, 2},
		// And the row that says the first reading is counting passes rather
		// than adding one: a loop over nothing writes no head at all, where
		// the reading that counts the construct still writes one.
		{"a for over nothing", "for i in; do :; done", 0, 1, 0, 0},
		// `if` and `while` fire under one reading only, which is the other
		// half of the split — a rule of "every compound" would have them
		// both and a rule of "no compound" neither.
		{"an if", "if :; then :; fi", 2, 3, 2, 2},
		{"a while", "while false; do :; done", 1, 2, 1, 1},
		// A group standing on its own, which is the shape a function body
		// has and is deliberately not the same question: see
		// TestAFunctionBodyIsNeverAHead.
		{"a group", "{ :; }", 1, 2, 1, 1},
		// `[[` and `((` are heads whose whole work is a word or an
		// expression, which is the first reading's rule in one line each.
		{"a condition", "[[ x == x ]]", 1, 1, 0, 0},
		{"an arithmetic command", "(( 1 ))", 1, 1, 0, 0},
		// The arithmetic `for` counts its three expressions one at a time
		// under the first reading: the initializer, a condition per pass and
		// one more that ends it, and a step per pass.
		{"an arithmetic for", "for ((i=0;i<2;i++)); do :; done", 8, 3, 2, 2},
		// A function *definition* is a command, and one reading writes a
		// head for it.
		{"defining a function", "zz() { :; }\n:", 1, 2, 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Ds(t, c.src, DebugTrapHeadsWordAndArithmetic); got != c.word {
				t.Errorf("word-and-arithmetic heads: %d D, want %d", got, c.word)
			}
			if got := Ds(t, c.src, DebugTrapHeadsEveryCompound); got != c.every {
				t.Errorf("every compound head: %d D, want %d", got, c.every)
			}
			if got := Ds(t, c.src, DebugTrapHeadsNone); got != c.none {
				t.Errorf("no heads: %d D, want %d", got, c.none)
			}
		})
	}
}

// A head names its own line, not the line the body has got to — which is a
// question only a head that fires more than once can raise, and the list
// `for` is the one that does.
func TestAForHeadNamesItsOwnLineOnEveryPass(t *testing.T) {
	src := "trap 'echo at=$LINENO' DEBUG\nfor i in a b\ndo\n  :\ndone"
	// The body counted from where it fired, which is the reading that makes
	// `$LINENO` inside one the firing line rather than 1; see
	// Semantics.CommandTrapBodyLine. Without it every head would read 1 and
	// this test could not tell one head's line from another's.
	out, _, _ := trapRun(t, src, func(s *Semantics) {
		headSem(DebugTrapHeadsWordAndArithmetic)(s)
		s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
	}, Diagnostics{Location: LocationLineWord})
	// Two passes: the head on line 2 and the body on line 4, twice each.
	if want := "at=2\nat=4\nat=2\nat=4\n"; out != want {
		t.Errorf("lines named: %q, want %q", out, want)
	}
}

// A function's body is a group and is never a head, in either reading. The
// column that writes most heads writes one for a group standing on its own
// and none for a body, so the two cannot be one question.
func TestAFunctionBodyIsNeverAHead(t *testing.T) {
	const src = "f() { :; }\nset -T\ntrap 'echo D' DEBUG\nf"
	for _, heads := range []DebugTrapHeads{
		DebugTrapHeadsWordAndArithmetic, DebugTrapHeadsEveryCompound, DebugTrapHeadsNone,
	} {
		out, _, _ := trapRun(t, src, func(s *Semantics) {
			headSem(heads)(s)
			s.SetHasTraceLetters = Yes
		}, Diagnostics{})
		// The call and the body's one command, and nothing for the body's
		// own braces.
		if got := strings.Count(out, "D\n"); got != 2 {
			t.Errorf("%v: %d D, want 2 — the call and the body's command", heads, got)
		}
	}
}

// The one column that writes a head as a call *enters* a body writes it in
// addition to those two, and names the line the body opens on.
func TestTheEntryHeadIsItsOwnAnswer(t *testing.T) {
	const src = "f()\n{\n  :\n}\nset -T\ntrap 'echo at=$LINENO' DEBUG\nf"
	entry := func(a Answer) string {
		out, _, _ := trapRun(t, src, func(s *Semantics) {
			headSem(DebugTrapHeadsWordAndArithmetic)(s)
			s.SetHasTraceLetters = Yes
			s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
			s.DebugTrapFiresOnEnteringAFunction = a
		}, Diagnostics{Location: LocationLineWord})
		return out
	}
	// The call on line 7, the body opening on line 2, the body's command on
	// line 3.
	if want := "at=7\nat=2\nat=3\n"; entry(Yes) != want {
		t.Errorf("with the entry head: %q, want %q", entry(Yes), want)
	}
	if want := "at=7\nat=3\n"; entry(No) != want {
		t.Errorf("without it: %q, want %q", entry(No), want)
	}
}
