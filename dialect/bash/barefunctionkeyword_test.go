// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// The `function` keyword with nothing that could be a name after it is
// refused, and the refusal names the *token* — the ordinary unexpected-token
// sentence this shell gives anywhere else, not a wording about a name that
// was wanted (#3732).
//
// Measured 2026-09-19 on bash 5.3.20 at /opt/homebrew/bin/bash, on that same
// binary invoked as `sh`, and on bash 3.2.57 at /bin/bash — all three agree,
// so it is neither the version nor the invocation. Script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device; the
// echoed source line follows each, and the status is 2.
//
//	function                    syntax error near unexpected token `newline'
//	function; echo after        syntax error near unexpected token `;'
//	function & echo after       syntax error near unexpected token `&'
//	function | echo after       syntax error near unexpected token `|'
//	function && echo after      syntax error near unexpected token `&&'
//	function )                  syntax error near unexpected token `)'
//	function <in                syntax error near unexpected token `<'
//
// The keyword *with* a name is not in question and is not touched: `function
// 1abc { :; }` and `function +x { :; }` are both accepted here, which is what
// says the divergence was the keyword standing alone.
func TestTheKeywordWithNoNameNamesTheTokenHere(t *testing.T) {
	if bash.Dialect().BareFunctionKeyword {
		t.Fatal("BareFunctionKeyword is on, so the keyword would stand alone here")
	}
	for _, tc := range []struct{ src, want string }{
		{"function\n", "<shell>: line 1: syntax error near unexpected token `newline'\n<shell>: line 1: `function'\n"},
		{"function; echo after\n", "<shell>: line 1: syntax error near unexpected token `;'\n<shell>: line 1: `function; echo after'\n"},
		{"function & echo after\n", "<shell>: line 1: syntax error near unexpected token `&'\n<shell>: line 1: `function & echo after'\n"},
		{"function | echo after\n", "<shell>: line 1: syntax error near unexpected token `|'\n<shell>: line 1: `function | echo after'\n"},
		{"function && echo after\n", "<shell>: line 1: syntax error near unexpected token `&&'\n<shell>: line 1: `function && echo after'\n"},
		{"function )\n", "<shell>: line 1: syntax error near unexpected token `)'\n<shell>: line 1: `function )'\n"},
		{"function <in\n", "<shell>: line 1: syntax error near unexpected token `<'\n<shell>: line 1: `function <in'\n"},
		// The input running out where the name belonged is the newline this
		// shell appends to what it reads, which is why it names one — see
		// syntax.Dialect.EndOfInputIsANewlineWhereNoneCouldStand, whose other
		// position is a loop's variable.
		{"function", "<shell>: line 1: syntax error near unexpected token `newline'\n<shell>: line 1: `function'\n"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := bash.Diagnostics().ParseDiagnostic("<shell>", "", err, tc.src); got != tc.want {
			t.Errorf("%q said\n%q\nwant\n%q", tc.src, got, tc.want)
		}
	}
}
