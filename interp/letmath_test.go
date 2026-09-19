// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// refusesAtSign gives the grammar a byte its arithmetic reader will not read,
// which is what the failure under test is: the core has no such byte, so
// without it `1 @` is an operator failure and never reaches the question.
func refusesAtSign(d *syntax.Dialect) { d.ArithBytesRefusedOutright = "@" }

// letRun is run() with that byte, and with the status a parse failure would
// otherwise carry pinned to 1 — the value `let` reports is the measurement
// here, and the POSIX preset's syntax status is 2, which would stand in for it
// on every failing row and hide the answer.
func letRun(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	diag := Diagnostics{SyntaxErrorStatus: 1}
	out, _ := runGrammar(t, src, refusesAtSign, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &diag
		r.Stderr = nil
	})
	return out
}

// What `let` does with an expression the arithmetic reader could not finish:
// what it reports, and whose complaint it is. Two questions, measured apart
// and answered apart.

// TestLetKeepsTheValueBeforeAnIllegalByte pins the axis by name: Yes leaves
// `let` with the value the reader had reached when it met a byte it refuses,
// No leaves it with nothing.
//
// `let` reports false for an expression that came out zero, which is not the
// question — it is unanimous — so the pair differing only in the digit is what
// makes the answer visible. `1 @` and `0 @` fail identically and part company
// only over the value that stood before the byte.
func TestLetKeepsTheValueBeforeAnIllegalByte(t *testing.T) {
	const src = `let '1 @'; echo "one=$?"
let '0 @'; echo "zero=$?"
let '1+2 @'; echo "sum=$?"
let '@'; echo "alone=$?"`

	for _, tc := range []struct {
		keeps Answer
		want  string
	}{
		{Yes, "one=0\nzero=1\nsum=0\nalone=1\n"},
		// Nothing is kept, so every one of them is the zero `let` calls false.
		{No, "one=1\nzero=1\nsum=1\nalone=1\n"},
	} {
		sem := PosixSemantics()
		sem.LetKeepsTheValueBeforeAnIllegalByte = tc.keeps
		if out := letRun(t, src, sem); out != tc.want {
			t.Errorf("%v: got %q, want %q", tc.keeps, out, tc.want)
		}
	}
}

// And it is that failure alone. A value stood before each of these too, and
// none of them keeps it under either answer — the reader gave up mid-stream in
// the row above and the grammar rejected the whole expression here.
func TestOnlyARefusedByteKeepsAnything(t *testing.T) {
	const src = `let '1+'; echo "ranout=$?"
let '5 5'; echo "leftover=$?"
let '1/0'; echo "divzero=$?"`

	for _, keeps := range []Answer{Yes, No} {
		sem := PosixSemantics()
		sem.LetKeepsTheValueBeforeAnIllegalByte = keeps
		if out, want := letRun(t, src, sem), "ranout=1\nleftover=1\ndivzero=1\n"; out != want {
			t.Errorf("%v: got %q, want %q", keeps, out, want)
		}
	}
}

// The expressions after the one that failed are not evaluated: `let '0 @' '3'`
// is the zero's answer and not the three's.
func TestLetStopsAtTheRefusedByte(t *testing.T) {
	sem := PosixSemantics()
	sem.LetKeepsTheValueBeforeAnIllegalByte = Yes
	if out, want := letRun(t, `let '0 @' '3'; echo "st=$?"`, sem), "st=1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// An unanswered axis is refused by name, and only a refused byte reaches it:
// an expression that reads cleanly, and one that fails another way, never ask.
func TestAnUnansweredLetValueIsRefused(t *testing.T) {
	sem := PosixSemantics()
	sem.LetKeepsTheValueBeforeAnIllegalByte = Unspecified
	for _, src := range []string{`let '1+2'; echo "ok=$?"`, `let '1+'; echo "ok=$?"`} {
		if out := letRun(t, src, sem); !strings.Contains(out, "ok=") {
			t.Errorf("%q got %q, want it never to have asked", src, out)
		}
	}
	out, _ := runGrammar(t, `let '1 @'`, refusesAtSign, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "a byte the reader refuses") {
		t.Errorf("got %q, want the unanswered axis named", out)
	}
}

// TestArithErrorNamesTheBuiltin pins the Diagnostics answer: the name goes in
// front of the sentence, or nowhere at all.
//
// Both halves are asserted under NamesBuiltinInLocation, which is the rule the
// answer is an exception to: a name written and then moved into the location
// is the wrong answer rendered the right way, and only the location rule
// running over both settings can tell the two apart.
func TestArithErrorNamesTheBuiltin(t *testing.T) {
	for _, tc := range []struct {
		names bool
		want  string
	}{
		// Written, and then moved into the location by the rule that moves
		// every other builtin's name there.
		{true, "sh:let:1: 1+: operand expected"},
		// Not written, so the rule has nothing to move and the complaint is
		// located as the shell's own.
		{false, "sh:1: 1+: operand expected"},
	} {
		diag := Diagnostics{
			ArithErrorNamesTheBuiltin: tc.names,
			NamesBuiltinInLocation:    true,
			Location:                  LocationTightLine,
		}
		out, _ := run(t, `let '1+'`, func(r *Runner) { r.Diagnostics = &diag })
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.names, got, tc.want)
		}
	}
}

// The same answer, one builtin over. A declaration whose *value* will not
// evaluate raises the evaluator's sentence through a builtin, and every column
// names the builtin for it exactly where it names one for `let` — measured
// 2026-09-18 over `typeset -i a=1+` in a script file:
//
//	bash 5.3.20	loc.sh: line 1: typeset: 1+: arithmetic syntax error: …
//	ksh93u+    	loc.sh[1]: typeset: 1+: more tokens expected
//	zsh 5.9.2  	loc.sh:1: bad math expression: operand expected …
//
// and `integer a=1+`, `float a=1+`, `local -i a=1+` and `a=1+; integer a` are
// the same three answers, which is why this rides on the message's rule rather
// than on a field of its own (#3342).
func TestADeclarationsBadValueNamesTheBuiltinWhereLetDoes(t *testing.T) {
	// Both doors: the expression the reader could not finish, and the one it
	// read and could not evaluate. They are separate call sites and the first
	// draft named the builtin at only one of them.
	for _, src := range []struct{ text, blamed string }{
		{`typeset -i a=1+`, "1+: operand expected"},
		{`typeset -i a=1/0`, "division by zero"},
	} {
		for _, tc := range []struct {
			names bool
			want  string
		}{
			{true, "sh:typeset:1: " + src.blamed},
			{false, "sh:1: " + src.blamed},
		} {
			diag := Diagnostics{
				ArithErrorNamesTheBuiltin: tc.names,
				NamesBuiltinInLocation:    true,
				Location:                  LocationTightLine,
			}
			out, _ := run(t, src.text, func(r *Runner) { r.Diagnostics = &diag })
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%q %v: got %q, want %q", src.text, tc.names, got, tc.want)
			}
		}
	}
}

// And only where a builtin is speaking. The same evaluator is reached from a
// plain assignment to a name already carrying the attribute, and bash writes
// no name in front of that one — `typeset -i a=1; a+=2+` is a bare `2+:
// arithmetic syntax error` there — because there is no builtin to name.
func TestAnIntegerAssignmentOutsideADeclarationNamesNobody(t *testing.T) {
	for _, names := range []bool{true, false} {
		diag := Diagnostics{
			ArithErrorNamesTheBuiltin: names,
			NamesBuiltinInLocation:    true,
			Location:                  LocationTightLine,
		}
		out, _ := run(t, "typeset -i a=1\na+=2+\n", func(r *Runner) { r.Diagnostics = &diag })
		if got, want := strings.TrimSpace(out), "sh:2: 2+: operand expected"; got != want {
			t.Errorf("%v: got %q, want %q", names, got, want)
		}
	}
}

// And the answer is about the *message* and not about the builtin: the same
// shell's `let` with no operand at all is the builtin's own complaint and
// still names it.
func TestALetWithNoOperandStillNamesTheBuiltin(t *testing.T) {
	diag := Diagnostics{
		ArithErrorNamesTheBuiltin: false,
		NamesBuiltinInLocation:    true,
		Location:                  LocationTightLine,
		LetNoExpression:           "not enough arguments",
	}
	out, _ := run(t, `let`, func(r *Runner) { r.Diagnostics = &diag })
	if want := "sh:let:1: not enough arguments"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}
