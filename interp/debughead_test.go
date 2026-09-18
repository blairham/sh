// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Which commands other than the simple ones fire a DEBUG trap, and the panel
// gives three readings rather than a rule with exceptions. See
// interp.DebugTrapHeads for the table these are written from; the
// measurements are 2026-09-13, one construct per line under
// `trap 'echo D $LINENO' DEBUG`.
//
// This engine fired at simple commands and nowhere else, which is none of the
// three — and the comment at the firing site asserted that a compound heading
// fires nothing, which no column in the panel does.

func headSem(heads DebugTrapHeads) func(*Semantics) {
	return func(s *Semantics) {
		s.TrapHasDebugCondition = Yes
		s.DebugTrapRunsInsideCalls = No
		s.DebugTrapRunsInSubshells = No
		s.DebugTrapRefiresOnEnteringAFunction = No
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
		name, src               string
		word, pass, every, none int
	}{
		// `case` fires a head under three readings, so it is the row that
		// says they are not "some fire" against "none fire".
		{name: "case", src: "case x in x) :;; esac", word: 2, pass: 2, every: 2, none: 1},
		// The list `for` is where they part: two readings count passes and
		// the third counts the construct.
		{name: "a for over two items", src: "for i in a b; do :; done", word: 4, pass: 4, every: 3, none: 2},
		// And the row that says those two are counting passes rather than
		// adding one: a loop over nothing writes no head at all, where the
		// reading that counts the construct still writes one.
		{name: "a for over nothing", src: "for i in; do :; done", word: 0, pass: 0, every: 1, none: 0},
		// `if` and `while` fire under one reading only, which is the other
		// half of the split — a rule of "every compound" would have them
		// both and a rule of "no compound" neither.
		{name: "an if", src: "if :; then :; fi", word: 2, pass: 2, every: 3, none: 2},
		{name: "a while", src: "while false; do :; done", word: 1, pass: 1, every: 2, none: 1},
		// A group standing on its own, which is the shape a function body
		// has and is deliberately not the same question: see
		// TestAFunctionBodyIsNeverAHead.
		{name: "a group", src: "{ :; }", word: 1, pass: 1, every: 2, none: 1},
		// `[[` and `((` are heads whose whole work is a word or an
		// expression, which is the first readings' rule in one line each.
		{name: "a condition", src: "[[ x == x ]]", word: 1, pass: 1, every: 1, none: 0},
		{name: "an arithmetic command", src: "(( 1 ))", word: 1, pass: 1, every: 1, none: 0},
		// The arithmetic `for` counts its three expressions one at a time
		// under the first two readings: the initializer, a condition per
		// pass and one more that ends it, and a step per pass.
		{
			name: "an arithmetic for", src: "for ((i=0;i<2;i++)); do :; done",
			word: 8, pass: 8, every: 3, none: 2,
		},
		// And with the initializer and the step left out, which is where
		// the two part: one reading counts an expression the script did not
		// write and the other does not. Measured as a count and not as a
		// rule about emptiness — `for ((;;))` writes two heads per round in
		// the bash columns and one in ksh93, for every combination of the
		// three parts a script can leave out.
		{
			name: "an arithmetic for with two parts left out",
			src:  "i=0\nfor ((;i<2;)); do i=$((i+1)); done",
			word: 9, pass: 6, every: 4, none: 3,
		},
		// A function *definition* is a command, and one reading writes a
		// head for it.
		{name: "defining a function", src: "zz() { :; }\n:", word: 1, pass: 1, every: 2, none: 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Ds(t, c.src, DebugTrapHeadsWordAndArithmetic); got != c.word {
				t.Errorf("word-and-arithmetic heads: %d D, want %d", got, c.word)
			}
			if got := Ds(t, c.src, DebugTrapHeadsEveryPassAndWrittenParts); got != c.pass {
				t.Errorf("every pass and the written parts: %d D, want %d", got, c.pass)
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

// A function's body is a group and is never a head, in any reading. The
// column that writes most heads writes one for a group standing on its own
// and none for a body, so the two cannot be one question.
func TestAFunctionBodyIsNeverAHead(t *testing.T) {
	const src = "f() { :; }\nset -T\ntrap 'echo D' DEBUG\nf"
	for _, heads := range []DebugTrapHeads{
		DebugTrapHeadsWordAndArithmetic, DebugTrapHeadsEveryPassAndWrittenParts,
		DebugTrapHeadsEveryCompound, DebugTrapHeadsNone,
	} {
		out, _, _ := trapRun(t, src, func(s *Semantics) {
			headSem(heads)(s)
			s.SetHasTheErrtraceLetter = Yes
			s.SetHasTheFunctraceLetter = Yes
		}, Diagnostics{})
		// The call and the body's one command, and nothing for the body's
		// own braces.
		if got := strings.Count(out, "D\n"); got != 2 {
			t.Errorf("%v: %d D, want 2 — the call and the body's command", heads, got)
		}
	}
}

// selectRun is trapRun with replies, because the menu loop is the one head
// whose count cannot be read off a script alone.
func selectRun(t *testing.T, src, replies string, heads DebugTrapHeads) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	headSem(heads)(&sem)
	var out, errs bytes.Buffer
	dg := Diagnostics{}
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdin: strings.NewReader(replies), Stdout: &out, Stderr: &errs,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String()
}

// A menu loop's head is the one that parts the two readings that both count a
// word loop's passes: one writes it once for the construct and the other
// repeats it with the replies.
//
// Two replies on purpose. One reply cannot tell a head per pass from a head
// per construct, which is the shape of probe the rest of this file exists to
// correct. And the reply that *ends* the loop is not a pass: neither reading
// writes a third head when the input runs out.
func TestAMenuLoopsHeadFiresOncePerConstructOrOncePerPass(t *testing.T) {
	const src = "trap 'echo D' DEBUG\nselect w in a b\ndo\n  echo B\ndone"
	// The count is the whole run: two bodies in every reading, and one head
	// or two beside them.
	for _, c := range []struct {
		heads DebugTrapHeads
		want  int
	}{
		{DebugTrapHeadsWordAndArithmetic, 3},
		{DebugTrapHeadsEveryPassAndWrittenParts, 4},
		{DebugTrapHeadsEveryCompound, 3},
		{DebugTrapHeadsNone, 2},
	} {
		out := selectRun(t, src, "1\n2\n", c.heads)
		// The bodies are the control: the two readings differ in the heads
		// and in nothing else, so a run that lost a pass would show here.
		if got := strings.Count(out, "B\n"); got != 2 {
			t.Fatalf("%v: %d bodies, want 2 — the loop did not run twice", c.heads, got)
		}
		if got := strings.Count(out, "D\n"); got != c.want {
			t.Errorf("%v: %d D, want %d", c.heads, got, c.want)
		}
	}
}
