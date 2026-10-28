// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is named by its digits, and a refused `{name}` in front
// of a redirection operator by the text around it — one byte past the brace
// and one in front of the name unless that is a blank — in the sentence
// without "unexpected token" (#6552) — and an array literal's first element,
// against its `(`, from the front of the assignment. Measured 2026-10-15 and
// 2026-10-16 on bash 5.3.20 with
// `-c '<program>'` and a script file alike; the table is on
// interp.Diagnostics.SyntaxUnexpectedFdVariableReadText.
func TestARefusedIONumberIsNamedAsBashNamesIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"for 2>x\n", "syntax error near unexpected token `2'"},
		{"case x in 2>x) esac\n", "syntax error near unexpected token `2'"},
		{"case 2>x in x) esac\n", "syntax error near unexpected token `2'"},
		{"f() 2>x\n", "syntax error near unexpected token `2'"},
		{"for {v}</dev/null\n", "syntax error near `{v}<'"},
		{"for  {ab}<x\n", "syntax error near `{ab}<'"},
		{"for {v}>>x\n", "syntax error near `{v}>'"},
		{"for {v}<&0\n", "syntax error near `{v}<'"},
		{"for {v}<<<x\n", "syntax error near `{v}<'"},
		{"select {v}>x\n", "syntax error near `{v}>'"},
		{"function {v}</dev/null\n", "syntax error near `{v}<'"},
		{"f() {v}</dev/null\n", "syntax error near `{v}<'"},
		{"case x in {v}<x) esac\n", "syntax error near `{v}<'"},
		{"case x in ({v}>x) esac\n", "syntax error near `({v}>'"},
		{"case x in a|{v}<x) esac\n", "syntax error near `|{v}<'"},
		{"case x in a| {v}<x) esac\n", "syntax error near `{v}<'"},
		{"case x in a) ;;{v}<x) esac\n", "syntax error near `;{v}<'"},
		{"case x in a) ;;\t{v}<x) esac\n", "syntax error near `{v}<'"},
		{"case x in a) ;;\n{v}<x) esac\n", "syntax error near `{v}<'"},
		{"case {v}<x in x) esac\n", "syntax error near `{v}<'"},
		{"for i in a {v}<x; do :; done\n", "syntax error near `{v}<'"},
		{"a=({v}>/dev/null)\n", "syntax error near `a=({v}>'"},
		{"a+=({v}>/dev/null)\n", "syntax error near `a+=({v}>'"},
		{"a[1]=({v}>/dev/null)\n", "syntax error near `a[1]=({v}>'"},
		{"x=1 a=({v}>/dev/null)\n", "syntax error near `a=({v}>'"},
		{"declare a=({v}>/dev/null)\n", "syntax error near `a=({v}>'"},
		{"a=( {v}>/dev/null)\n", "syntax error near `{v}>'"},
		{"a=(x {v}>/dev/null)\n", "syntax error near `{v}>'"},
		{"a=(); for i in {v}>x; do :; done\n", "syntax error near `{v}>'"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := diagTable(bash.Diagnostics()).ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
