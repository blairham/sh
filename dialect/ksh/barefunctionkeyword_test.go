// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// The `function` keyword with nothing that could be a name after it is
// refused here too, and the refusal names the token in this shell's own
// wording rather than saying what was wanted (#3732).
//
// Measured 2026-09-19 on ksh93u+ 2012-08-01 at /bin/ksh, script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device. Status 3 throughout.
//
//	function                    syntax error at line 2: `newline' unexpected
//	function; echo after        syntax error at line 1: `;' unexpected
//	function & echo after       syntax error at line 1: `&' unexpected
//	function | echo after       syntax error at line 1: `|' unexpected
//	function && echo after      syntax error at line 1: `&&' unexpected
//	function )                  syntax error at line 1: `)' unexpected
//	function <in                syntax error at line 1: `<' unexpected
//
// The line the first row names is the one after the text, because the newline
// has been read by the time the keyword is refused — the same placement this
// shell gives a bare `for`, and what a file whose last line has no newline
// says instead is `end of file`.
func TestTheKeywordWithNoNameNamesTheTokenHere(t *testing.T) {
	if ksh.Dialect().BareFunctionKeyword {
		t.Fatal("BareFunctionKeyword is on, so the keyword would stand alone here")
	}
	for _, tc := range []struct{ src, want string }{
		{"function\n", "<shell>: syntax error at line 2: `newline' unexpected\n"},
		{"function; echo after\n", "<shell>: syntax error at line 1: `;' unexpected\n"},
		{"function & echo after\n", "<shell>: syntax error at line 1: `&' unexpected\n"},
		{"function | echo after\n", "<shell>: syntax error at line 1: `|' unexpected\n"},
		{"function && echo after\n", "<shell>: syntax error at line 1: `&&' unexpected\n"},
		{"function )\n", "<shell>: syntax error at line 1: `)' unexpected\n"},
		{"function <in\n", "<shell>: syntax error at line 1: `<' unexpected\n"},
		{"function", "<shell>: syntax error at line 1: `end of file' unexpected\n"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseDiagnostic("<shell>", "", err, tc.src); got != tc.want {
			t.Errorf("%q said %q, want %q", tc.src, got, tc.want)
		}
	}
}
