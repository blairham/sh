// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAFunctionNameMayHoldAnEquals — where what stands before a word's first
// `=` is not a name, the word is a function name and a `(` after it opens a
// definition. Measured 2026-10-04 on bash 5.3.20; see commands.md.
func TestAFunctionNameMayHoldAnEquals(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{`2=() { echo hi; }; 2=`, "hi\n", ""},
		{`set -- z y w; 1=(a b); echo "[$*]"`, "", "syntax error near unexpected token `a'"},
		{`set -- a; 1+=(z)`, "", "syntax error near unexpected token `z'"},
		{`set -- a b c; 2=(); echo "[$*]"`, "", "syntax error near unexpected token `;'"},
		// The controls: an assignment stays one.
		{`a=b=() { :; }`, "", "syntax error near unexpected token `('"},
		{`f=g() { :; }`, "", "syntax error near unexpected token `('"},
	} {
		out, errs, _ := runAlias(t, c.src)
		if out != c.out || !strings.Contains(errs, c.errs) {
			t.Errorf("%s: wrote %q and said %q, want %q and %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
