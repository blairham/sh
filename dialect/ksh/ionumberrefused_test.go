// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is named by the operator behind it: always for digits,
// and for a `{name}` in a `case` subject or pattern, where elsewhere the
// `{name}` is the byte 0x04 (#6552). Measured 2026-10-15 on ksh93u+
// 2012-08-01 at /bin/ksh with `-c '<program>'`, the byte read with `od -c`;
// the table is on interp.Diagnostics.SyntaxUnexpectedIONumberIsItsOperator.
func TestARefusedIONumberIsNamedByItsOperator(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"for 2>x\n", ">"},
		{"select 2<x\n", "<"},
		{"case x in 2>x) esac\n", ">"},
		{"case 2>x in x) esac\n", ">"},
		{"for i in a 2>x; do :; done\n", ">"},
		{"f() 2<x\n", "<"},
		{"case x in {v}<x) esac\n", "<"},
		{"case x in ({v}>x) esac\n", ">"},
		{"case x in a|{v}<x) esac\n", "<"},
		{"case x in {v}>>x) esac\n", ">>"},
		{"case x in {v}<&0) esac\n", "<&"},
		{"case x in {v}<>x) esac\n", "<>"},
		{"case x in {v}>|x) esac\n", ">|"},
		{"case x in {v}<<<x) esac\n", "<<"},
		{"case {v}<x in x) esac\n", "<"},
		{"case {v}>>x in x) esac\n", ">>"},
		// Outside a `case` word the `{name}` is the byte.
		{"for {v}</dev/null\n", "\x04"},
		{"select {v}</dev/null\n", "\x04"},
		{"function {v}</dev/null\n", "\x04"},
		{"for i in a {v}<x; do :; done\n", "\x04"},
		{"f() {v}</dev/null\n", "\x04"},
		{"f() {v}>x\n", "\x04"},
		{"f() {v}<&0\n", "\x04"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		want := "syntax error at line 1: `" + tc.want + "' unexpected"
		if got := ksh.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, want)
		}
	}
}
