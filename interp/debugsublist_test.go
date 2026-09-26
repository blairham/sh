// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How an `&&`/`||` **list** fires the DEBUG trap, which is the layer outside
// the pipeline and the last one a statement has. This engine gave a list no
// rule of its own, so every column fired once per operand — right in four of
// them and wrong in the one that fires once for the list (#4556). See
// interp.DebugTrapSublist for the panel these rows are written from.
//
// The action writes to **stderr** for the reason the pipeline rows do: an
// operand may be an element of a pipeline, and a firing made inside one
// writes into a pipe nobody reads.

// sublistSem answers everything a list's firing needs, leaving the axis to
// the caller. Compound heads fire for every compound, because the row that
// separates the two readings hardest is an operand that *is* one: the list's
// single firing has to stand in for that head as well.
func sublistSem(how DebugTrapSublist) func(*Semantics) {
	return func(s *Semantics) {
		s.TrapHasDebugCondition = Yes
		s.DebugTrapRunsBeforeTheCommand = Yes
		s.DebugTrapRunsInsideCalls = Yes
		s.DebugTrapRunsInSubshells = Yes
		s.DebugTrapRefiresOnEnteringAFunction = No
		s.DebugTrapCompoundHeads = DebugTrapHeadsEveryCompound
		s.DebugTrapPipelines = DebugTrapPipelineOnceForThePipeline
		s.DebugTrapSublists = how
		// So that `$LINENO` inside the body names the line the firing was
		// made at, which is what the noun test below reads.
		s.CommandTrapBodyLine = TrapBodyLineWhereItFired
	}
}

// sublistDs runs src under one reading and counts the firings, wherever they
// happened.
func sublistDs(t *testing.T, src string, how DebugTrapSublist) int {
	t.Helper()
	const act = "trap 'echo D >&2' DEBUG\n"
	out, errs, _ := trapRun(t, act+src, sublistSem(how), Diagnostics{})
	if strings.Contains(out, "D") {
		t.Fatalf("ran %q: the action wrote to stdout: %q", src, out)
	}
	return strings.Count(errs, "D\n")
}

func TestHowASublistFiresADebugTrap(t *testing.T) {
	for _, c := range []struct {
		name, src            string
		perOperand, onceOnly int
	}{
		{
			// The reduction, and the row that separates the two readings on
			// its own.
			name: "two operands", src: ": && :",
			perOperand: 2, onceOnly: 1,
		},
		{
			// The other operator, so the reading is about the list and not
			// about `&&`.
			name: "the or operator", src: "! : || :",
			perOperand: 2, onceOnly: 1,
		},
		{
			// Three, which says the per-operand reading counts *operands*
			// rather than adding one firing to a list, and that the other
			// does not count them at all.
			name: "three operands", src: ": && : && :",
			perOperand: 3, onceOnly: 1,
		},
		{
			// A list that short-circuits still fires once for the whole of
			// it under one reading and once per operand that *ran* under
			// the other — so the counts part here too, and the single
			// firing is made before anything has decided to stop.
			name: "short-circuited", src: ": && ! : && :",
			perOperand: 2, onceOnly: 1,
		},
		{
			// An operand that is a compound fires no head of its own where
			// the list has fired: 1 for the list against 1 for the `:`, the
			// group's head, and the `:` inside it.
			name: "a compound operand", src: ": && { :; }",
			perOperand: 3, onceOnly: 2,
		},
		{
			// And the withholding stops at the operand. A list *inside* one
			// is a list in its own right and fires for itself, which is
			// what keeps the single firing from silencing a body.
			name: "a list inside an operand", src: ": && { : && :; }",
			perOperand: 4, onceOnly: 2,
		},
		{
			// A list nested in an operand that is **not the last**, which
			// is where the withholding has to be put back rather than
			// cleared: the third operand is still the outer list's and
			// still fires nothing of its own.
			name: "a list inside a middle operand", src: ": && { : && :; } && :",
			perOperand: 5, onceOnly: 2,
		},
		{
			// An operand that is a pipeline: one firing for the list where
			// the other reading leaves the pipeline to fire its own.
			name: "a pipeline operand", src: ": | : && :",
			perOperand: 2, onceOnly: 1,
		},
		{
			// The control, and the one row that must not move: a statement
			// that is not a list at all fires exactly once either way.
			name: "a simple command", src: ":",
			perOperand: 1, onceOnly: 1,
		},
		{
			// The second control, from the other side: two statements are
			// two firings under both readings, so the once-for-the-list
			// reading is not "once per line" and not "once per statement
			// run".
			name: "two statements", src: ":\n:",
			perOperand: 2, onceOnly: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := sublistDs(t, c.src, DebugTrapSublistPerOperand); got != c.perOperand {
				t.Errorf("per operand over %q: %d firings, want %d", c.src, got, c.perOperand)
			}
			if got := sublistDs(t, c.src, DebugTrapSublistOnceForTheList); got != c.onceOnly {
				t.Errorf("once for the list over %q: %d firings, want %d", c.src, got, c.onceOnly)
			}
		})
	}
}

// The noun is the **sublist**, and a count alone cannot say so: "once per
// line" and "once per list" agree on every row above, because every row above
// writes one list per line.
//
// These two hold the list fixed and move the line, and hold the line fixed
// and move the list. A list spread over two lines fires **once**, at the
// first of them; two lists on one line fire **twice**, both at that line. A
// reading keyed on the line would answer 1 and 1, and one keyed on the
// statement's text would have to call `: ; :` one statement.
func TestASublistFiresForTheListAndNotForTheLine(t *testing.T) {
	// Line 1 is the trap, line 2 holds two lists, lines 3 and 4 hold one.
	const src = ": ; :\n: &&\n:\n"
	const act = "trap 'echo \"D$LINENO\" >&2' DEBUG\n"
	for _, c := range []struct {
		how  DebugTrapSublist
		want string
	}{
		{DebugTrapSublistPerOperand, "D2 D2 D3 D4"},
		{DebugTrapSublistOnceForTheList, "D2 D2 D3"},
	} {
		_, errs, _ := trapRun(t, act+src, sublistSem(c.how), Diagnostics{})
		if got := strings.Join(strings.Fields(errs), " "); got != c.want {
			t.Errorf("%v: firings %q, want %q", c.how, got, c.want)
		}
	}
}

// A firing the list makes happens in the shell running it, which is what lets
// the action's own assignment outlive the list — the same property the
// pipeline's single firing has, and the one a firing moved into an operand
// would lose.
func TestASublistFiresInTheShellRunningIt(t *testing.T) {
	const src = "trap 'n=$((n+1))' DEBUG\n: && : && :\ntrap - DEBUG\necho \"n=$n\"\n"
	out, _, _ := trapRun(t, src, sublistSem(DebugTrapSublistOnceForTheList), Diagnostics{})
	// The list is one firing and `trap - DEBUG` is the second; the `echo`
	// runs with the trap already gone.
	if want := "n=2\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And the two axes compose rather than colliding: where a firing stands
// behind the command it would have preceded, the list's single firing stands
// behind the **whole list**, after everything its operands flushed.
//
// Measured on zsh 5.9.2 with `unsetopt DEBUG_BEFORE_CMD`, 2026-09-25: `print
// q && { print r; print s }` writes the firing for the `trap` command itself,
// then `q`, then `r` and its firing, `s` and its firing, and the list's last
// of all.
func TestASublistFiresBehindTheWholeListWhereFiringsGoBehindTheCommand(t *testing.T) {
	const src = "trap 'echo T' DEBUG\necho q && { echo r; echo s; }\n"
	out, _, _ := trapRun(t, src, func(s *Semantics) {
		sublistSem(DebugTrapSublistOnceForTheList)(s)
		s.DebugTrapRunsBeforeTheCommand = No
	}, Diagnostics{})
	if want := "T\nq\nr\nT\ns\nT\nT\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
