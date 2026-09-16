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
	return braceRunWith(t, src, nil, env...)
}

// braceRunWith is braceRun with an answer of its own put on the vector, for
// the tests that are about an axis rather than about the construct. One
// function under both, because a second copy of the runner setup is a second
// place for a flag the construct needs to be turned on.
func braceRunWith(t *testing.T, src string, set func(*Semantics), env ...string) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	if set != nil {
		set(&sem)
	}
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

// The body is a variable scope as well as a frame in one of the two columns
// that have the spelling, so a declaration written inside one is local to the
// body there and writes the shell's own name in the other.
//
// The axis is answered both ways on every row, because the correction is a
// disagreement rather than a fix: see
// Semantics.CurrentShellSubstitutionBodyIsAScope for the measurements. Rows
// name the axis and never a shell.
func TestABracedSubstitutionBodyIsAScopeOrIsNot(t *testing.T) {
	for _, c := range []struct{ name, src, scoped, shared string }{
		{
			name:   "a declaration in the body",
			src:    "x=outer\nv=${ typeset x=in; printf %s \"$x\";}\nprintf '[%s][%s]\\n' \"$v\" \"$x\"\n",
			scoped: "[in][outer]\n",
			shared: "[in][in]\n",
		},
		{
			name:   "a plain assignment, which is not a declaration",
			src:    "x=outer\nv=${ x=in; printf %s \"$x\";}\nprintf '[%s][%s]\\n' \"$v\" \"$x\"\n",
			scoped: "[in][in]\n",
			shared: "[in][in]\n",
		},
		{
			name:   "a declaration of a name the shell does not hold",
			src:    "v=${ typeset fresh=in; printf %s \"$fresh\";}\nprintf '[%s][%s]\\n' \"$v\" \"${fresh-none}\"\n",
			scoped: "[in][none]\n",
			shared: "[in][in]\n",
		},
		{
			name:   "a scope inside a call, which nests",
			src:    "x=outer\nf() { typeset x=fn; v=${ typeset x=in; printf %s \"$x\";}; printf '[%s][%s]\\n' \"$v\" \"$x\"; }\nf\nprintf '[%s]\\n' \"$x\"\n",
			scoped: "[in][fn]\n[outer]\n",
			shared: "[in][in]\n[outer]\n",
		},
		{
			name:   "a declaration in the pipe spelling's body",
			src:    "x=outer\nv=${| typeset x=in; REPLY=$x;}\nprintf '[%s][%s]\\n' \"$v\" \"$x\"\n",
			scoped: "[in][outer]\n",
			shared: "[in][in]\n",
		},
		{
			name:   "the body's own value, which the scope does not touch",
			src:    "v=${ printf out;}\nprintf '[%s]\\n' \"$v\"\n",
			scoped: "[out]\n",
			shared: "[out]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, c.scoped}, {No, c.shared}} {
				got := braceRunWith(t, c.src, func(s *Semantics) {
					s.CurrentShellSubstitutionBodyIsAScope = a.answer
					// The declaration word declares a local in any function
					// here, which is a question of its own and is answered
					// so these rows reach the one they are about.
					s.TypesetLocalNeedsKeywordFunction = No
				})
				if got != a.want {
					t.Errorf("scope=%v: got %q, want %q", a.answer, got, a.want)
				}
			}
		})
	}
}

// And `local` is legal in a body that is a scope, which is a consequence of
// the axis rather than a second decision: the word is refused for having no
// function to be local to, and a body with a scope has one. The forked
// spelling is the control and keeps the refusal under both answers.
func TestLocalInABracedSubstitutionFollowsTheScope(t *testing.T) {
	const src = "x=outer\nv=${ local x=in; printf %s \"$x\";}\nprintf '[%s][%s]\\n' \"$v\" \"$x\"\n"
	const forked = "x=outer\nv=$(local x=in; printf %s \"$x\")\nprintf '[%s][%s]\\n' \"$v\" \"$x\"\n"
	scoped := braceRunWith(t, src, func(s *Semantics) {
		s.CurrentShellSubstitutionBodyIsAScope = Yes
		s.LocalOutsideAFunctionIsAnError = Yes
	})
	if scoped != "[in][outer]\n" {
		t.Errorf("scoped: got %q, want the declaration local to the body", scoped)
	}
	shared := braceRunWith(t, src, func(s *Semantics) {
		s.CurrentShellSubstitutionBodyIsAScope = No
		s.LocalOutsideAFunctionIsAnError = Yes
	})
	if !strings.Contains(shared, "local") {
		t.Errorf("shared: got %q, want the word refused for having no scope", shared)
	}
	for _, a := range []Answer{Yes, No} {
		got := braceRunWith(t, forked, func(s *Semantics) {
			s.CurrentShellSubstitutionBodyIsAScope = a
			s.LocalOutsideAFunctionIsAnError = Yes
		})
		if !strings.Contains(got, "local") {
			t.Errorf("forked, scope=%v: got %q, want the word refused there whatever the axis says", a, got)
		}
	}
}
