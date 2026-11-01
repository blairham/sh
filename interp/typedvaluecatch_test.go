// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestATypedValueThatEvaluatesAsksNothing pins where the two typed-value
// axes are asked: only where the store has failed. A `read` or `getopts`
// over a typed name whose value evaluates puts no question, so a vector that
// answers neither runs it.
func TestATypedValueThatEvaluatesAsksNothing(t *testing.T) {
	t.Parallel()
	out, _ := run(t, `typeset -i n; read n <<E
3+4
E
echo "[$n]"; getopts x n -x; echo "[$n]"`, func(r *Runner) {
		sem := PosixSemantics()
		sem.ReadTypedValueFailureIsAnAssignmentFailure = Unspecified
		sem.GetoptsTypedValueFailureIsAFreeze = Unspecified
		r.Semantics = &sem
	})
	// The vector leaves other questions unanswered too; what is asserted is
	// that neither of these two is among them, and that the store landed.
	for _, q := range []string{"typed name will not take"} {
		if strings.Contains(out, q) {
			t.Errorf("asked %q: %q", q, out)
		}
	}
	if !strings.HasPrefix(
		strings.TrimPrefix(
			out,
			"sh: a declaration without a value setting the name: the shells disagree here and no dialect was chosen\n",
		),
		"[7]\n",
	) {
		t.Errorf("the value was not stored: %q", out)
	}
}

// TestATypedValueFailureIsAskedWhereItHappens is the other half: a value
// that will not evaluate reaches each axis, and each answer does what it
// says.
func TestATypedValueFailureIsAskedWhereItHappens(t *testing.T) {
	t.Parallel()
	const readSrc = "typeset -i n=5; read n m <<E\n1+ b\nE\necho \"[$?] [$n] [$m]\"; echo after"
	for _, tc := range []struct {
		name   string
		answer Answer
		src    string
		after  bool
	}{
		{"read, the assignment's cost", Yes, readSrc, false},
		{"read, given up at the name", No, readSrc, true},
	} {
		out, _ := run(t, tc.src, func(r *Runner) {
			sem := *r.Semantics
			sem.ReadTypedValueFailureIsAnAssignmentFailure = tc.answer
			r.Semantics = &sem
		})
		if got := strings.Contains(out, "after"); got != tc.after {
			t.Errorf("%s: after=%v in %q", tc.name, got, out)
		}
		if tc.after && !strings.Contains(out, "[1] [5] []") {
			t.Errorf("%s: %q", tc.name, out)
		}
	}
	out, _ := run(t, readSrc, func(r *Runner) {
		sem := *r.Semantics
		sem.ReadTypedValueFailureIsAnAssignmentFailure = Unspecified
		r.Semantics = &sem
	})
	if !strings.Contains(out, "the shells disagree") {
		t.Errorf("a failed store under an unanswered vector asked nothing: %q", out)
	}
}
