// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A redirection written in front of a **parenthesized** command belongs to it
// here, and in front of nothing else.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`: `>/dev/null ( echo hi )` and `>/dev/null
// (( 1 ))` both run, and the brace group, the loops, the `if`, the `case` and
// the `[[` are all refused — zsh takes one before every compound it has, and
// bash and dash take none (#3560).
func TestARedirectionMayPrecedeAParenthesizedCommandHere(t *testing.T) {
	d := ksh.Dialect()
	if got, want := d.RedirectionBeforeACompound, syntax.RedirectionMayPrecedeAParenthesizedCommand; got != want {
		t.Errorf("RedirectionBeforeACompound = %v, want %v", got, want)
	}
	for _, src := range []string{">/dev/null ( echo hi )", ">/dev/null (( 1 ))", "3>/dev/null ( echo hi )"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
	for _, src := range []string{
		">/dev/null { echo hi; }",
		">/dev/null while false; do :; done",
		">/dev/null if true; then echo hi; fi",
		">/dev/null for i in a; do echo hi; done",
		">/dev/null case a in a) echo hi;; esac",
		// And the assignment prefix takes it back, as it does in the other
		// column that takes one.
		"v=x >/dev/null ( echo hi )",
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err == nil {
			t.Errorf("%q parsed, want it refused", src)
		}
	}
}
