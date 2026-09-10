// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// BareSubscriptIsASubscript, named as an axis and not as a shell.
//
// The grammar flag decides whether the brackets after an unbraced `$name`
// belong to the expansion at all; this decides what they *mean* once they do.
// A grammar with the flag and either answer is a coherent shell, which is why
// there are two answers rather than one and a special case.

// runBare runs src with the brace-less spelling in the grammar and the axis
// set as asked. The array shape is fixed at three elements with distinct
// values so that the readings in play cannot agree by accident: subscript 1
// against a zero base is the second element, and the brackets read as text
// are the first followed by three characters.
func runBare(t *testing.T, src string, subscript Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.BareSubscript = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.BareSubscriptIsASubscript = subscript
		sem.ArrayBaseIsZero = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.ArrayNameWithoutSubscriptIsTheList = No
		r.Semantics = &sem
	})
}

const bareArray = "a=(xx yy zz)\n"

// The axis, both ways, on the shape where the two answers and the two ways of
// getting one of them wrong are four different strings.
func TestABareSubscriptIsReadOrIsText(t *testing.T) {
	for _, tc := range []struct {
		name      string
		subscript Answer
		want      string
	}{
		{"read as a subscript", Yes, "yy"},
		{"read as text", No, "xx[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBare(t, bareArray+`printf "[%s]" "$a[1]"`, tc.subscript)
			if st != 0 || out != "["+tc.want+"]" {
				t.Errorf("= %q (status %d), want [%s]", out, st, tc.want)
			}
		})
	}
}

// The braced spelling is not this axis's to answer: the braces say where the
// expansion ends, so there is nothing for a second reading to be.
func TestABracedSubscriptIgnoresTheAxis(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, st := runBare(t, bareArray+`printf "[%s]" "${a[1]}"`, a)
		if st != 0 || out != "[yy]" {
			t.Errorf("axis %v: = %q (status %d), want [yy]", a, out, st)
		}
	}
}

// Text, and the word's own text: what is written between the brackets is
// expanded, and unquoted metacharacters are live for the glob stage.
func TestBareSubscriptTextIsExpandedAndLive(t *testing.T) {
	t.Run("an expansion between the brackets", func(t *testing.T) {
		out, st := runBare(t, bareArray+"i=2\n"+`printf "[%s]" "$a[$i]"`, No)
		if st != 0 || out != "[xx[2]]" {
			t.Errorf("= %q (status %d), want [xx[2]]", out, st)
		}
	})
	t.Run("live for the glob stage", func(t *testing.T) {
		out, st := runBare(t, "b=(f)\n: > f1\n"+`printf "[%s]" $b[1]`, No)
		if st != 0 || out != "[f1]" {
			t.Errorf("= %q (status %d), want [f1] — the word was a pattern", out, st)
		}
	})
	t.Run("quoted, it is not a pattern", func(t *testing.T) {
		out, st := runBare(t, "b=(f)\n: > f1\n"+`printf "[%s]" "$b[1]"`, No)
		if st != 0 || out != "[f[1]]" {
			t.Errorf("= %q (status %d), want [f[1]]", out, st)
		}
	})
}

// The axis is read when the word expands and not when it was read, which is
// the whole reason it is an axis. A body parsed once and run twice under two
// answers gives two results.
func TestTheAxisIsReadWhenTheWordExpands(t *testing.T) {
	src := bareArray + `f() { printf "[%s]" "$a[1]"; }` + "\nf\nf"
	// One body, parsed once, called twice, with the vector swapped between
	// the two calls — which is what a run-time option does to it. The two
	// calls have to give the two answers: a shell that decided this while
	// reading would give one answer twice.
	answers := []Answer{Yes, No}
	out, st := runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.BareSubscript = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.ArrayNameWithoutSubscriptIsTheList = No
		r.Semantics = &sem
		call := 0
		r.AtEveryFunctionCall(func(r *Runner) func() {
			s := *r.Semantics
			s.BareSubscriptIsASubscript = answers[call%len(answers)]
			call++
			r.Semantics = &s
			return func() {}
		})
	})
	if st != 0 || out != "[yy][xx[1]]" {
		t.Errorf("= %q (status %d), want [yy][xx[1]] — one body, two answers", out, st)
	}
}

// A grammar with the brace-less spelling and no answer is a gap, and it says
// so out loud rather than picking a side.
func TestABareSubscriptWithNoAnswerIsRefused(t *testing.T) {
	out, st := runBare(t, bareArray+`printf "[%s]" "$a[1]"`, Unspecified)
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("= %q (status %d), want the unanswered-axis refusal at 2", out, st)
	}
}
