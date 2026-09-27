// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runCompoundAssign evaluates src in a dialect that has floats and an integer
// attribute, with the compound-assignment axis set as given. It names the
// axis rather than a shell, as this package's rule requires.
func runCompoundAssign(t *testing.T, src string, converts Answer) string {
	t.Helper()
	d := syntax.Core()
	d.ArithFloat = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.CompoundArithAssignmentConvertsItsValue = converts
	dg := Diagnostics{ArithFloatDigits: 17}
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// What a compound assignment is worth when the name it writes through carries
// the integer attribute and the number it computed is a float.
//
// The two answers differ only there, and the table proves both halves of that
// claim rather than asserting it: the plain `=` row, the integer-value row and
// the no-attribute row are each the same under both answers, and the *store*
// is the same on every row. Only the compound operator's own value moves.
func TestACompoundAssignmentIsWorthWhatTheAxisSays(t *testing.T) {
	for _, tc := range []struct{ name, src, converts, keeps string }{
		{
			"a float added through an integer name",
			`typeset -i n=1; echo "$(( n += 0.5 )) $n"`, "1 1", "1.5 1",
		},
		{
			"subtracted",
			`typeset -i n=1; echo "$(( n -= 0.5 )) $n"`, "0 0", "0.5 0",
		},
		{
			"multiplied",
			`typeset -i n=1; echo "$(( n *= 2.5 )) $n"`, "2 2", "2.5 2",
		},
		{
			"read out of a larger expression",
			`typeset -i n=1; echo "$(( (n += 0.5) + 0 )) $n"`, "1 1", "1.5 1",
		},
		// The controls. Each is the same under both answers, and together
		// they say the rule is keyed on the compound *operator* and not on
		// "an assignment" or on the attribute alone.
		{
			"the plain operator instead",
			`typeset -i n=1; echo "$(( n = n + 0.5 )) $n"`, "1 1", "1 1",
		},
		{
			"an integer value converts to itself",
			`typeset -i n=1; echo "$(( n += 1 )) $n"`, "2 2", "2 2",
		},
		{
			"no attribute on the name",
			`n=1; echo "$(( n += 0.5 )) $n"`, "1.5 1.5", "1.5 1.5",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runCompoundAssign(t, tc.src, Yes); got != tc.converts {
				t.Errorf("converting: %s = %q, want %q", tc.src, got, tc.converts)
			}
			if got := runCompoundAssign(t, tc.src, No); got != tc.keeps {
				t.Errorf("keeping: %s = %q, want %q", tc.src, got, tc.keeps)
			}
		})
	}
}

// And a dialect that never answered it is refused by name rather than given
// one of the two readings — but only where the question really arises, so a
// compound assignment that computes an integer is untouched.
func TestAnUnansweredCompoundAssignmentAxisIsRefusedOnlyWhereItArises(t *testing.T) {
	got := runCompoundAssign(t, `typeset -i n=1; echo "$(( n += 1 )) $n"`, Unspecified)
	if got != "2 2" {
		t.Errorf("an integer value: %q, want %q", got, "2 2")
	}
	got = runCompoundAssign(t, `typeset -i n=1; echo "$(( n += 0.5 ))"`, Unspecified)
	if !strings.Contains(got, "compound assignment") {
		t.Errorf("a float value: %q, want a refusal naming the axis", got)
	}
}
