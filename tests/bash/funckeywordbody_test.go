// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// The `function` keyword's body must be a compound command here, the same as
// the parenthesized form's — the rule this shell had and never applied to the
// keyword (#1833).
//
// Measured 2026-09-11 over a file holding `function a`, the body and a call,
// on bash 5.3.15, bash 3.2.57 and bash-as-sh, all three answering alike:
//
//	body            answer
//	echo B          syntax error near unexpected token `echo', status 2
//	function a echo B (one line)  the same sentence at `echo', status 2
//	(( 1 ))         B's definition runs, status 0
//	( echo B )      status 0
//	{ echo B; }     status 0
//
// The arithmetic and subshell rows are the discriminating half: they are what
// separates "a compound command" from the brace group the shell that
// originated the keyword insists on.
func TestTheKeywordBodyMustBeCompound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"function a\necho B\na\n", "syntax error near unexpected token `echo'"},
		{"function a echo B\na\n", "syntax error near unexpected token `echo'"},
		{"function a\nx=1\na\n", "syntax error near unexpected token `x=1'"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, where bash answers a syntax error", tc.src)
			continue
		}
		if got := bash.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{
		"function a { echo B; }\na\n",
		"function a\n(( 1 ))\na\n",
		"function a\n( echo B )\na\n",
		"function a\nfor i in 1; do echo B; done\na\n",
	} {
		if _, err := syntax.Parse(src, bash.Dialect()); err != nil {
			t.Errorf("%q: a compound body refused: %v", src, err)
		}
	}
}
