// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

// A redirection target beginning with a substitution that runs against the
// operator does not parse, and the script ends there. Measured 2026-10-02 on
// ksh93u+ 2012-08-01 over a script file `: OP SUBST` then `echo next`
// (#5514). See syntax.Dialect.ProcessSubstitutionAgainstTheRedirectionIsRefused.
func TestASubstitutionAgainstItsRedirectionDoesNotParse(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{": <> >(cat)\necho next\n", "", "s.sh: syntax error at line 1: `>(' unexpected\n"},
		{": > <(echo)\necho next\n", "", "s.sh: syntax error at line 1: `<(' unexpected\n"},
		{": < <(echo)\necho next\n", "next\n", ""},
	} {
		out, errs, _ := runKshScript(t, c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%q\n got %q, %q\nwant %q, %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
