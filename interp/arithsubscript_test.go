// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `a[1]` written inside an expression is read as part of it, where `${a[1]}`
// is substituted into it before it is read. Same element, two routes.
func TestAnArrayElementInsideAnExpression(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(3 4 5); echo $(( a[1] ))`, "4"},
		{`a=(3 4 5); echo $(( a[1] + a[2] ))`, "9"},
		// The subscript is an expression of its own, and a name in it needs
		// no `$` for the same reason the array's does not.
		{`a=(3 4 5); echo $(( a[1+1] ))`, "5"},
		{`a=(3 4 5); i=2; echo $(( a[i] ))`, "5"},
		{`a=(3 4 5); i=1; echo $(( a[i+1] ))`, "5"},
		// Out of range and never-an-array are both zero rather than errors,
		// which is what lets a version check read an element it may not have.
		{`a=(3 4); echo $(( a[9] + 1 ))`, "1"},
		{`echo $(( nosucharray[0] + 1 ))`, "1"},
		// An element that is empty is zero too.
		{`a=(3 "" 5); echo $(( a[1] + 7 ))`, "7"},
		// The subscript belongs to an assignment's target, so this writes.
		{`a=(3 4); (( a[1] = 9 )); echo "${a[1]}"`, "9"},
		{`a=(3 4); (( a[1] += 5 )); echo "${a[1]}"`, "9"},
		{`a=(3 4); i=1; (( a[i] = 7 )); echo "${a[1]}"`, "7"},
		// A subscript is not an operator: `a` alone still reads the scalar.
		{`a=(3 4); echo $(( a ))`, "3"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// The element counts from the dialect's base, the same one `${a[1]}` uses —
// so the two routes to an element cannot disagree.
func TestAnArithmeticSubscriptUsesTheDialectsBase(t *testing.T) {
	base := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.ArrayBaseIsZero = a
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		a         Answer
		src, want string
	}{
		{Yes, `a=(3 4 5); echo $(( a[1] ))`, "4"},
		{No, `a=(3 4 5); echo $(( a[1] ))`, "3"},
		// And by the other route, which must agree.
		{Yes, `a=(3 4 5); echo $(( ${a[1]} ))`, "4"},
		{No, `a=(3 4 5); echo $(( ${a[1]} ))`, "3"},
	} {
		if out, _ := run(t, tc.src, base(tc.a)); strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: %s = %q, want %q", tc.a, tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}
