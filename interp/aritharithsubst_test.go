// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runArithBody runs src with the axis a substitution whose whole body is one
// `(( … ))` turns on, and the grammar the rows need.
func runArithBody(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArithCommand = true
		d.CurrentShellSubstitution = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArithmeticOnlyBodyIsAnArithmeticExpansion = a
		r.Semantics = &sem
	})
}

// A substitution whose whole body is one arithmetic command expands to the
// expression's *value* where the dialect reads it that way, and to what the
// command printed — nothing — where it does not.
func TestASubstitutionWhoseWholeBodyIsArithmeticIsAnExpansion(t *testing.T) {
	for _, src := range []string{
		`echo "[$( (( 1+1 )) )]"`,
		`echo "[${ (( 1+1 )); }]"`,
	} {
		if out, _ := runArithBody(t, Yes, src); !strings.Contains(out, "[2]") {
			t.Errorf("%s: got %q, want the expression's value", src, out)
		}
		if out, _ := runArithBody(t, No, src); !strings.Contains(out, "[]") {
			t.Errorf("%s: got %q, want the command's output", src, out)
		}
	}
}

// The assignment escapes, which no substitution running a command in a copy
// of the shell could allow — and is the row that says the body is not being
// run at all.
func TestAnArithmeticBodysAssignmentReachesTheShellThatWroteIt(t *testing.T) {
	out, _ := runArithBody(t, Yes, `n=0; echo "[$( (( n+=5 )) )]"; echo "n=$n"`)
	if !strings.Contains(out, "[5]") || !strings.Contains(out, "n=5") {
		t.Errorf("got %q, want [5] and n=5", out)
	}
	if out, _ := runArithBody(t, No, `n=0; echo "[$( (( n+=5 )) )]"; echo "n=$n"`); !strings.Contains(out, "n=0") {
		t.Errorf("got %q, want the assignment contained", out)
	}
}

// It is the *whole* body and nothing less: a second command in it, a
// background `&` and the backquoted spelling are all ordinary substitutions
// under either answer.
func TestOnlyAWholeArithmeticBodyIsAnExpansion(t *testing.T) {
	for _, src := range []string{
		`echo "[$( (( 1+1 )); echo hi )]"`,
		`echo "[$( echo hi; (( 1+1 )) )]"`,
	} {
		for _, a := range []Answer{Yes, No} {
			if out, _ := runArithBody(t, a, src); !strings.Contains(out, "[hi]") {
				t.Errorf("%s (%v): got %q, want [hi]", src, a, out)
			}
		}
	}
	for _, src := range []string{
		"echo \"[`(( 1+1 ))`]\"",
		`echo "[$( (( 1+1 )) & )]"`,
	} {
		for _, a := range []Answer{Yes, No} {
			if out, _ := runArithBody(t, a, src); !strings.Contains(out, "[]") {
				t.Errorf("%s (%v): got %q, want []", src, a, out)
			}
		}
	}
}

// The consequence #3364 was filed on, and the two rows that rule out its
// stated cause: an expansion has no status for `set -e` to judge, where a
// substitution holding a subshell or a call still has one.
func TestErrExitDoesNotJudgeAnArithmeticOnlyBody(t *testing.T) {
	run := func(a Answer, src string) (string, int) {
		return runArithBody(t, a, src)
	}
	if out, st := run(Yes, `set -e; x=$( (( 0 )) ); printf survived`); !strings.Contains(out, "survived") || st != 0 {
		t.Errorf("got %q (status %d), want the line to carry on", out, st)
	}
	if out, _ := run(No, `set -e; x=$( (( 0 )) ); printf survived`); strings.Contains(out, "survived") {
		t.Errorf("got %q, want the option to judge a command", out)
	}
	for _, src := range []string{
		`set -e; x=$( ( (( 0 )) ) ); printf survived`,
		`set -e; x=$(f() { (( 0 )); }; f); printf survived`,
	} {
		if out, _ := run(Yes, src); strings.Contains(out, "survived") {
			t.Errorf("%s: got %q, want a substitution with a command in it still judged", src, out)
		}
	}
}
