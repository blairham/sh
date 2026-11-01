// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is named by the operator behind it: always for digits,
// and for a `{name}` in a `case` subject or pattern, among a loop's items or
// among an array literal's elements, where in a loop's name position the
// `{name}` is named by its text (#6552, #6790). Measured 2026-10-15 and
// 2026-10-16 on zsh 5.9.2 at /opt/homebrew/bin/zsh with
// `-fc '<program>'`; the table is on
// interp.Diagnostics.SyntaxUnexpectedIONumberIsItsOperator.
func TestARefusedIONumberIsNamedByItsOperator(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{"for 2>x\n", ">"},
		{"select 2<x\n", "<"},
		{"case x in 2>x) esac\n", ">"},
		{"case 2>x in x) esac\n", ">"},
		{"for i in a 2>x; do :; done\n", ">"},
		{"case x in {v}<x) esac\n", "<"},
		{"case x in ({v}>x) esac\n", ">"},
		{"case x in a|{v}<x) esac\n", "<"},
		{"case x in {v}>>x) esac\n", ">>"},
		{"case x in {v}<&0) esac\n", "<&"},
		{"case x in {v}<<<x) esac\n", "<<<"},
		{"case x in {v}<>x) esac\n", "<>"},
		{"case {v}<x in x) esac\n", "<"},
		{"for i in a {v}<x; do :; done\n", "<"},
		{"for i in {v}<x; do :; done\n", "<"},
		{"for i in a {v}>>x; do :; done\n", ">>"},
		{"select i in a {v}<x; do :; done\n", "<"},
		{"for i ({v}<x) :\n", "<"},
		{"a=({v}>/dev/null)\n", ">"},
		{"a=(x {v}>/dev/null)\n", ">"},
		{"a=(x {v}>>/dev/null y)\n", ">>"},
		{"typeset a=({v}>/dev/null)\n", ">"},
		{"a+=({v}</dev/null)\n", "<"},
		// In a loop's name position the `{name}` is named itself.
		{"for {v}</dev/null\n", "{v}"},
		{"for {ab}>>x\n", "{ab}"},
		{"select {v}>x\n", "{v}"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		want := "parse error near `" + tc.want + "'"
		if got := diagTable(zsh.Diagnostics()).ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, want)
		}
	}
}
