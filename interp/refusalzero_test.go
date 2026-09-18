// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// One dialect leaves **0** behind when a store through an operand refuses,
// where every other refusal it makes leaves 1 — and that 0 is not always what
// a caller reads.
//
// Three places raise it back to 1, and none of them is the route the program
// reached the shell by, which is what the two axes that record the 0 used to
// say. See Runner.refusalZeroIsRaisedBackToOne for the sixteen rows this was
// measured over (#3504).

// refusalZeroSrc wraps the refusal in a subshell so the number is readable at
// all: the give-up ends the shell, so at the top level the only observable is
// the process's own exit and a script file reports 1 there whatever the
// refusal left.
func refusalZeroSrc(inner string) string {
	return "a=(x y)\n( " + inner + " )\n" + `echo "next=$?"`
}

// The three raisers, and the shapes that are not raisers. Every row is the
// same refusal in the same dialect, so what varies is only where the shell was
// when it unwound.
func TestWhereARefusalsZeroIsRaisedBackToOne(t *testing.T) {
	const refuse = `typeset "a[-5]"=v`
	for _, c := range []struct {
		name  string
		inner string
		want  string
	}{
		// Raised: the left operand of an `&&` list, a loop's condition and a
		// loop's body.
		{"an && list's left operand", refuse + " && echo yes", "next=1"},
		{"a brace group on an && list's left", "{ " + refuse + "; } && echo yes", "next=1"},
		{"a while condition", "while " + refuse + "; do break; done", "next=1"},
		{"a while body", "while :; do " + refuse + "; break; done", "next=1"},
		{"a for body", "for i in 1; do " + refuse + "; done", "next=1"},
		// Not raised. `||` and `!` continue on a *failure*, which is what
		// makes the first row above about `&&` rather than about and-or
		// lists, and the rest have nothing to unwind out of at all.
		{"alone", refuse, "next=0"},
		{"an || list's left operand", refuse + " || echo no", "next=0"},
		{"negated", "! " + refuse, "next=0"},
		{"an if condition", "if " + refuse + "; then echo t; fi", "next=0"},
		{"an && list's right operand", "true && " + refuse, "next=0"},
		{"a brace group", "{ " + refuse + "; }", "next=0"},
		{"a statement with one behind it", refuse + "; echo tail", "next=0"},
		{"a pipeline's last element", "echo z | " + refuse, "next=0"},
		{"a case body", "case x in x) " + refuse + ";; esac", "next=0"},
		{"a nested subshell", "( " + refuse + " )", "next=0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := refusalZeroRun(t, Yes, refusalZeroSrc(c.inner))
			if !strings.Contains(out, c.want) {
				t.Errorf("out %q is missing %q", out, c.want)
			}
			// The give-up happened on every row: nothing the refusal was in
			// front of ran. A 0 from a command that succeeded would be a
			// different answer wearing the same number.
			for _, ran := range []string{"yes", "no", "t", "tail"} {
				if strings.Contains(out, "\n"+ran+"\n") {
					t.Errorf("out %q ran %q past the give-up", out, ran)
				}
			}
		})
	}
}

// The **top level of a script file** is the third raiser, and it is the one
// the old condition was standing in for: the shell is ending either way and
// that route reports 1 whatever the refusal left, where `-c` reports the
// refusal's own number.
func TestAScriptFilesTopLevelRaisesARefusalsZero(t *testing.T) {
	const src = "a=(x y)\n" + `typeset "a[-5]"=v` + "\necho after"
	for _, c := range []struct {
		name  string
		route Route
		want  int
	}{
		{"a script file", RouteScriptFile, 1},
		{"a command string", RouteCommandString, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := refusalZeroRunAs(t, Yes, src, c.route)
			if st != c.want {
				t.Errorf("status %d, want %d", st, c.want)
			}
			if strings.Contains(out, "after") {
				t.Errorf("out %q ran past a give-up", out)
			}
		})
	}
	// And a **subshell** in a script file keeps the 0, which is the row that
	// says the route was never the rule.
	out, _ := refusalZeroRunAs(t, Yes, refusalZeroSrc(`typeset "a[-5]"=v`), RouteScriptFile)
	if !strings.Contains(out, "next=0") {
		t.Errorf("out %q raised a subshell's zero on the file route", out)
	}
}

// Answered No, the refusal leaves the 1 every other one leaves, wherever it
// stands — so the raisers are about the 0 and not about the give-up.
func TestARefusalThatDoesNotLeaveZeroIsUnmovedByTheRaisers(t *testing.T) {
	for _, inner := range []string{`typeset "a[-5]"=v`, `typeset "a[-5]"=v && echo yes`} {
		out, _ := refusalZeroRun(t, No, refusalZeroSrc(inner))
		if !strings.Contains(out, "next=1") {
			t.Errorf("out %q, want the refusal's own 1", out)
		}
	}
}

// A `!` does not invert a status the shell is **ending with**, which is what
// made the negated row above read 1: the 0 the refusal left was turned into a
// 1 by a negation that was not testing anything.
//
// Measured 2026-09-17 on zsh 5.9.2, a script file, each inside `( … )`:
// `! exit 3` leaves 3 and `! typeset "a[b c]"=v` leaves the refusal's 0, where
// a command that merely *failed* is still inverted — `! nosuchcommand` is 0
// there. The `exit` row was wrong here before this and in the other direction.
func TestANegationDoesNotInvertAStatusTheShellIsEndingWith(t *testing.T) {
	out, _ := refusalZeroRun(t, Yes, "a=(x y)\n( ! exit 3 )\n"+`echo "next=$?"`)
	if !strings.Contains(out, "next=3") {
		t.Errorf("out %q inverted the status the shell exited with", out)
	}
	// The control: an ordinary status is inverted as it always was, in both
	// directions.
	out, _ = refusalZeroRun(t, Yes, "( ! true )\n"+`echo "next=$?"`+"\n( ! false )\n"+`echo "then=$?"`)
	if !strings.Contains(out, "next=1") || !strings.Contains(out, "then=0") {
		t.Errorf("out %q did not invert an ordinary status", out)
	}
}

func refusalZeroRun(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return refusalZeroRunAs(t, answer, src, RouteUnspecified)
}

// refusalZeroRunAs answers the axes on the way to the store, so that a row
// varies where the shell was and nothing else.
func refusalZeroRunAs(t *testing.T, answer Answer, src string, route Route) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.ArrayBaseIsZero = Yes
		s.TypesetTakesASubscript = Yes
		s.NegativeSubscriptPastTheStartInserts = No
		s.FatalErrorStatusIsOne = Yes
		s.StoreRefusalOfADeclaredElementLeavesZero = answer
	}, Diagnostics{}, src, route)
}
