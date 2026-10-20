// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `x` is an attribute of a function: set in silence, listed alone, and no
// line of it after a body. And the function letter follows the first word's
// sign like every other letter. Measured 2026-10-03 on ksh93u+ 2012-08-01
// under `-c`, over two functions with one marked (#5674). See
// interp.Semantics.FunctionAttributeLetters and
// interp.Semantics.FunctionBodyListingWritesItsAttributes.
func TestAFunctionsExportLetterIsItsOwn(t *testing.T) {
	const two = "function pa { echo one; }; function pb { echo two; }; "
	for _, c := range []struct{ src, want string }{
		{`typeset -fx pa; echo $?`, "0\n"},
		{`typeset -fx pa; typeset +fx`, "pa\n"},
		{`typeset -fx pa; typeset -fx`, "function pa { echo one; };"},
		// The bodies are written as they were defined, with no line between
		// them and none after.
		{`typeset -fx pa; typeset -f`, "function pa { echo one; };function pb { echo two; };"},
		{`typeset -fx pa; typeset +f +x pa; typeset +fx; echo end`, "end\n"},
		{`typeset -fx nosuch; echo $?`, "1\n"},
		// A later plus is another minus, the `f` letter included.
		{`typeset -f +f pa`, "function pa { echo one; };"},
		{`typeset -f +x pa; typeset +fx`, "pa\n"},
		{`typeset -fx +f pa; typeset +fx`, "pa\n"},
		// And a leading plus makes the later minus another plus.
		{`typeset +f -f pa`, "pa\n"},
	} {
		if out, st := runKshEmptyArray(t, two+c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
