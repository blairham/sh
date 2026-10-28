// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// An arithmetic command takes no redirection here: the operator is named, the
// number in front of it is not, a `{name}` in front of it is named by the byte
// 0x04, and the here-document family is named `<<` whichever member it was
// (#6528). Measured 2026-10-15 on ksh93u+ 2012-08-01 at /bin/ksh with
// `-c '<program>; echo ok'`; the table is on
// syntax.Dialect.ArithCommandTakesNoRedirection.
func TestARedirectionBehindAnArithmeticCommandIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(( 1 )) </dev/null\n", "`<' unexpected"},
		{"((1))>/dev/null\n", "`>' unexpected"},
		{"(( 1 )) 3</dev/null\n", "`<' unexpected"},
		{"(( 1 )) 2>&1\n", "`>&' unexpected"},
		{"(( 1 )) >>/dev/null\n", "`>>' unexpected"},
		{"(( 1 )) <<<x\n", "`<<' unexpected"},
		{"(( 1 )) 3<<<x\n", "`<<' unexpected"},
		{"(( 1 )) <<-E\nE\n", "`<<' unexpected"},
		{"(( 1 )) {v}</dev/null\n", "`\x04' unexpected"},
		{"(( 1 )) {abc}>&-\n", "`\x04' unexpected"},
		{"if (( 1 )) >/dev/null; then :; fi\n", "`>' unexpected"},
		{"f() (( 1 )) >/dev/null\n", "`>' unexpected"},
		{"echo a\n(( 1 )) >/dev/null\n", "syntax error at line 2: `>' unexpected"},
		// The here-document family is named `<<` wherever it is refused, and
		// so is the `{name}`.
		{"for <<<x\n", "`<<' unexpected"},
		{"for {v}</dev/null\n", "`\x04' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		got := diagTable(ksh.Diagnostics()).ParseFailure(err)
		want := tc.want
		if len(want) < len("syntax") || want[:6] != "syntax" {
			want = "syntax error at line 1: " + want
		}
		if got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, want)
		}
	}
	// The controls: the same redirection behind `[[ … ]]`, a C-style `for`
	// and a builtin, and one written in front of `(( … ))`, are all read.
	for _, src := range []string{
		"[[ -n x ]] </dev/null\n",
		"[[ -n x ]] {v}</dev/null\n",
		"for ((i=0;i<1;i++)); do :; done >/dev/null\n",
		"let 1 >/dev/null\n",
		"</dev/null (( 1 ))\n",
		"(( 1 )) | cat\n",
	} {
		if _, err := syntax.Parse(src, ksh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
