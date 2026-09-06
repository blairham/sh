// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// One operator may stand where a `case` pattern belongs in this shell, and it
// may stand where *any* of them belongs rather than only the first.
//
// The reach was measured at half its size, and the half that was missing is
// what a pattern list ending in a separator runs into: `(a|b|)` is refused
// here, but the token this shell blames is not the `)` — the slot swallows
// that, and the complaint lands on the `echo` after it. Which is why this
// shell says `word unexpected` about a line whose offending character is a
// paren, and why four such lines were reported with the wrong wording.
//
// Measured 2026-09-06 against /bin/dash, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, `-n` over a script file for the refusals and a plain run for
// the rest.

const dashCaseWant = "s.sh: 1: Syntax error: word unexpected (expecting \")\")\n"

// The four shapes whose emptiness leaves the `)` where a pattern belongs. All
// four are one wording and one status here, and the wording names a word.
func TestASeparatorAtTheEndOfAPatternListBlamesTheWordAfterTheParen(t *testing.T) {
	for _, src := range []string{
		"case a in (a|b|) echo m;; esac\n",
		"case a in (a|) echo m;; esac\n",
		"case a in a|) echo m;; esac\n",
		"case a in () echo m;; esac\n",
		"case a in ) echo m;; esac\n",
	} {
		_, err := syntax.Parse(src, dash.Dialect())
		if err == nil {
			t.Fatalf("%q parsed; this shell refuses it", src)
		}
		d := dash.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, src); got != dashCaseWant {
			t.Errorf("%q:\n got %q\nwant %q", src, got, dashCaseWant)
		}
		if got, want := d.SyntaxStatus(), 2; got != want {
			t.Errorf("%q: status = %d, want %d", src, got, want)
		}
	}
}

// A separator with an operator after it is *accepted*, and the arm keeps the
// patterns beside it: `(a|;|b)` matches `a` and `b` and nothing else — not
// even the empty subject, which is what separates the slot from the empty
// alternative one other shell has.
func TestAnOperatorAfterASeparatorIsTakenAndContributesNoPattern(t *testing.T) {
	for _, tc := range []struct {
		list    string
		matches map[string]bool
	}{
		{"(a|;)", map[string]bool{"a": true, "b": false, "": false}},
		{"(;|a)", map[string]bool{"a": true, "": false}},
		{"(a|;|b)", map[string]bool{"a": true, "b": true, "": false}},
		{"(a|))", map[string]bool{"a": true, "": false}},
		{"())", map[string]bool{"a": false, "": false}},
		{"(;;)", map[string]bool{"a": false, "": false}},
		{"(;|;)", map[string]bool{"a": false, "": false}},
		{"(a|;|;|b)", map[string]bool{"a": true, "b": true, "": false}},
	} {
		src := "case $x in " + tc.list + " echo m;; *) echo no;; esac\n"
		for subject, want := range tc.matches {
			out, st := runDashWith(t, src, map[string]string{"x": subject})
			answer := "no\n"
			if want {
				answer = "m\n"
			}
			if out != answer {
				t.Errorf("%q with x=%q: out = %q, want %q", src, subject, out, answer)
			}
			if st != 0 {
				t.Errorf("%q with x=%q: status = %d, want 0", src, subject, st)
			}
		}
	}
}

// runDashWith runs one snippet with the named variables already set, which is
// how a subject reaches a `case` without the snippet being rewritten for it.
func runDashWith(t *testing.T, src string, vars map[string]string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Vars: vars}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// Two operators in one pattern position is still a refusal, and it is a
// different wording — the offending token is named rather than classed. So the
// slot is one operator wide and the widening above did not make it a run.
func TestTwoOperatorsInOnePatternPositionIsStillRefused(t *testing.T) {
	const src = "case a in (a|; ;) echo m;; esac\n"
	const want = "s.sh: 1: Syntax error: \";\" unexpected (expecting \")\")\n"
	_, err := syntax.Parse(src, dash.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses it")
	}
	d := dash.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := d.SyntaxStatus(); got != 2 {
		t.Errorf("status = %d, want 2", got)
	}
}

// `||` is one token and it is not the slot's: at the *separator* position this
// shell blames the whole operator and says it wanted the paren. The empty
// alternative another shell reads out of the same two characters is what this
// contrasts with.
func TestTheDoubledSeparatorIsBlamedWholeAtTheSeparatorPosition(t *testing.T) {
	const src = "case a in (a||b) echo m;; esac\n"
	const want = "s.sh: 1: Syntax error: \"||\" unexpected (expecting \")\")\n"
	_, err := syntax.Parse(src, dash.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses it")
	}
	d := dash.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := d.SyntaxStatus(); got != 2 {
		t.Errorf("status = %d, want 2", got)
	}
}

// The shapes that were already right and must stay so: an operator with no
// separator in front of it is refused, and it is refused by name.
func TestAnOperatorWithNoSeparatorBeforeItIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"case a in (a;) echo m;; esac\n", "s.sh: 1: Syntax error: \";\" unexpected (expecting \")\")\n"},
		{"case a in (a&) echo m;; esac\n", "s.sh: 1: Syntax error: \"&\" unexpected (expecting \")\")\n"},
		{"case a in (a b) echo m;; esac\n", dashCaseWant},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Fatalf("%q parsed; this shell refuses it", tc.src)
		}
		d := dash.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
