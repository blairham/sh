// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `f==(a b)` is the array `f=(a b)` where the dialect says so, and a plain
// value of `=` signs stays a value. See Dialect.ArrayLiteralAfterRepeatedEquals.
func TestRepeatedEqualsBeforeAParenthesisIsAnArray(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ArrayLiteral = true
	d.ArrayLiteralAfterRepeatedEquals = true
	for _, c := range []struct{ src, want string }{
		{"f==(a b)", "f=(a b)"},
		{"f===(a b)", "f=(a b)"},
		{"f==a", "f==a"},
	} {
		f, err := syntax.Parse(c.src, d)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got := syntax.Print(f); got != c.want {
			t.Errorf("%q printed %q, want %q", c.src, got, c.want)
		}
	}
	d.ArrayLiteralAfterRepeatedEquals = false
	if _, err := syntax.Parse("f==(a b)", d); err == nil {
		t.Error("without the field `f==(a b)` parsed")
	}
}
