// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `${ cmd;}` runs in the shell that read it, which is the only reason the
// spelling exists: what it assigns survives, where `$( … )` loses it.
func TestABracedSubstitutionRunsInTheCurrentShell(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"what it writes is the value", "echo ${ echo one;}\n", "one\n"},
		{
			// The whole point, next to the form that does not do it.
			"and what it assigns survives",
			"x=0\ny=${ x=1; echo hi;}\necho \"[$x][$y]\"\n", "[1][hi]\n",
		},
		{
			"where the subshell form loses it",
			"x=0\ny=$(x=1; echo hi)\necho \"[$x][$y]\"\n", "[0][hi]\n",
		},
		{"quoted, it is one field", "echo \"${ echo a b;}\"\n", "a b\n"},
		{"two of them join", "echo ${ echo a;}${ echo b;}\n", "ab\n"},
		{"one inside another", "echo ${ echo ${ echo in;};}\n", "in\n"},
		{"an empty body is an empty value", "echo \"[${ }]\"\n", "[]\n"},
		{"a function is a command like any other", "f() { echo fn; }\necho ${ f;}\n", "fn\n"},
		{"trailing newlines go, as they do for the other form", "echo \"[${ echo x;}]\"\n", "[x]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := braceRun(t, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The status is the body's last command's, because it is this runner's
// status — set where every other command sets it.
func TestABracedSubstitutionCarriesItsStatus(t *testing.T) {
	// An assignment with no command name, because that is the only shape
	// that shows it: after `echo ${ false;}` the status is the *echo's*, not
	// the substitution's, which is what bash reports too.
	if got := braceRun(t, "y=${ false;}\necho \"st=$?\"\n"); !strings.Contains(got, "st=1") {
		t.Errorf("got %q, want st=1", got)
	}
	if got := braceRun(t, "y=${ true;}\necho \"st=$?\"\n"); !strings.Contains(got, "st=0") {
		t.Errorf("got %q, want st=0", got)
	}
	if got := braceRun(t, "echo ${ false;}\necho \"st=$?\"\n"); !strings.Contains(got, "st=0") {
		t.Errorf("got %q, want the echo's status, which is what it is", got)
	}
}

// The writer is put back afterwards: this runner goes on being used, and a
// substitution that kept the buffer would swallow everything after it.
func TestABracedSubstitutionGivesTheOutputBack(t *testing.T) {
	got := braceRun(t, "y=${ echo caught;}\necho after\n")
	if !strings.Contains(got, "after") {
		t.Errorf("got %q, want what follows to be written", got)
	}
	if strings.Contains(got, "caught") {
		t.Errorf("got %q, want the body's output to have gone to the value", got)
	}
}

// A message from inside the body is placed in the script, not in the body —
// the same offset the subshell form carries, and given back afterwards.
func TestABracedSubstitutionIsPlacedInTheScript(t *testing.T) {
	var errs strings.Builder
	sem := PosixSemantics()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{Location: LocationTightLine},
		Name: "sh", Dialect: &d, Stdout: &strings.Builder{}, Stderr: &errs,
	})
	f, err := syntax.Parse("true\ntrue\ny=${ nosuchcmd;}\nnosuchcmd\n", d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "sh:3:") {
		t.Errorf("said %q, want the body reported at line 3", errs.String())
	}
	if !strings.Contains(errs.String(), "sh:4:") {
		t.Errorf("said %q, want the line after it reported at line 4", errs.String())
	}
}

// braceRun runs a source in a runner that has both spellings of the construct.
//
// The trailing env is what the shell was *born with*, which only the reply
// form's test needs — it is the difference between a name deleted and a name
// hidden, and a deleted one still reads through to the environment. Variadic
// rather than a second helper beside this one: two would be two places for a
// fix to land, and only one of them would get it.
func braceRun(t *testing.T, src string, env ...string) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	// And the pipe spelling, so replysubst_test.go has the same door rather
	// than a second one of its own. Turning it on here is also a control:
	// every row above is written in the blank form, and none of them moves.
	d.ReplySubstitution = true
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
		Stdout: &buf, Stderr: &buf, Env: env,
	})
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestBraceRanges — the alphabetic, stepped and zero-padded forms, whose
// absence left all three as literal text. The rows that reach a measured
// disagreement answer it by name — BraceRangePadsToEndpointWidth and the
// step-sign pair — with bash's answers; the axis tests hold the others.
func TestBraceRanges(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo {a..e}`, "a b c d e"},
		{`echo {e..a}`, "e d c b a"},
		{`echo {a..e..2}`, "a c e"},
		{`echo {1..10..3}`, "1 4 7 10"},
		{`echo {10..1..3}`, "10 7 4 1"},
		{`echo {1..10..-3}`, "1 4 7 10"},
		{`echo {01..03}`, "01 02 03"},
		{`echo {1..03}`, "01 02 03"},
		{`echo {-03..3..3}`, "-03 000 003"},
		{`echo {1..5..0}`, "1 2 3 4 5"},
		{`echo {1..5}`, "1 2 3 4 5"},
		{`echo {a,b}c`, "ac bc"},
		{`echo {a}`, "{a}"},
		{`echo {a..5}`, "{a..5}"},
		{`echo {a..bc}`, "{a..bc}"},
	} {
		var buf strings.Builder
		sem := PosixSemantics()
		sem.BraceExpansion = Yes
		sem.BraceRangePadsToEndpointWidth = Yes
		sem.BraceRangeStepSignHonored = No
		sem.BraceRangeNegativeStepReverses = No
		sem.BraceCharRangeSpansAnyCharacter = No
		sem.BraceRangeZeroStepCountsAsOne = Yes
		sem.BraceRangeMissingEndCountsFromZero = No
		sem.BraceRangeNumberMayCarryAPlus = Yes
		sem.BraceRangeThatCannotBeCounted = BraceRangeFailureKeepsTheWord
		d := syntax.Core()
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
			Stdout: &buf, Stderr: &buf,
		})
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(buf.String()); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The body is a frame a `return` leaves, which is what `return` in one means
// rather than a `return` with nothing to return from.
//
// Measured 2026-09-16 from script files under `env -i`: `v=${ echo hi;
// return 42; }` at the top level of a script is `[hi]` with `$?` 42 in bash
// 5.3.20 and ksh93u+ alike — the two columns that have the spelling — and the
// script carries on. The forked spelling is the control and is not this: the
// same `return` inside `$( … )` is refused in bash, because there the body is
// a shell of its own with no frame in it.
func TestABracedSubstitutionIsSomethingToReturnFrom(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the status is the operand and the script carries on",
			"v=${ echo hi; return 42;}\necho \"[$v] st=$?\"\necho alive\n",
			"[hi] st=42\nalive\n",
		},
		{
			"the body stops where the return is",
			"v=${ echo one; return 3; echo two;}\necho \"[$v]\"\n",
			"[one]\n",
		},
		{
			"with no operand it is the body's own status",
			"false\nv=${ echo hi; return;}\necho \"[$v] st=$?\"\n",
			"[hi] st=0\n",
		},
		{
			"and it leaves the body rather than the function around it",
			"f() { v=${ echo in; return 5;}; echo \"[$v] st=$?\"; echo after; }\nf\necho \"back st=$?\"\n",
			"[in] st=5\nafter\nback st=0\n",
		},
		{
			"the pipe spelling has a frame too",
			"v=${| REPLY=x; return 7;}\necho \"[$v] st=$?\"\n",
			"[x] st=7\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := braceRun(t, c.src); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The control: the forked spelling is not a frame, so a `return` in one has
// nothing to return from and is refused — which is the reading this shell
// was giving the braced spelling as well.
func TestAForkedSubstitutionIsNotSomethingToReturnFrom(t *testing.T) {
	got := braceRun(t, "v=$(echo hi; return 42)\necho \"[$v] st=$?\"\n")
	// The place is judged — the axis behind the refusal is asked, which this
	// bare core leaves unanswered and reports — and the status is 2 rather
	// than the operand. Both halves say the frame was not there.
	if !strings.Contains(got, "st=2") || !strings.Contains(got, "return") {
		t.Errorf("got %q, want the place judged and status 2", got)
	}
}
