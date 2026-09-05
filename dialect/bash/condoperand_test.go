// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A token standing where a conditional operator wanted a word.
//
// bash is the one dialect that words this as a statement about the *operator*
// that was waiting rather than about the token it met, and it says which kind
// of operator — which is the whole of what varies within the sentence. Before
// this it shared one message of our own that matched nobody: `expected an
// operand after ==` (#826).
func TestAConditionalOperandIsNamedTheOperatorsWay(t *testing.T) {
	d := bash.Diagnostics()
	for _, tc := range []struct{ src, want string }{
		// A bare group, which this dialect has nowhere.
		{"[[ $k == (a|b) ]]", "unexpected argument `(' to conditional binary operator"},
		{"[[ $k = (a|b) ]]", "unexpected argument `(' to conditional binary operator"},
		{"[[ $k != (a|b) ]]", "unexpected argument `(' to conditional binary operator"},
		// The unary family says so.
		{"[[ -n (a|b) ]]", "unexpected argument `(' to conditional unary operator"},
		// And it is about the token rather than about groups.
		{"[[ $k == | ]]", "unexpected argument `|' to conditional binary operator"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%s: parsed, want a syntax error", tc.src)
			continue
		}
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}
