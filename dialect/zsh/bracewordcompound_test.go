// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A word beginning with `{` behind a compound command is refused naming the
// whole word after one closed by a reserved word, and the bare brace after
// every other kind (#6539). Measured 2026-10-15 on zsh 5.9.2 at
// /opt/homebrew/bin/zsh with `-fc '<program>; echo ok'`, and the same with
// `emulate sh` or `emulate ksh` on the line before.
func TestABraceWordBehindACompoundIsNamedByTheCompoundsKind(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if :; then :; fi {foo}\n", "{foo}"},
		{"while false; do :; done {foo}\n", "{foo}"},
		{"for ((i=0;i<1;i++)); do :; done {foo}\n", "{foo}"},
		{"foreach i (a) :; end {foo}\n", "{foo}"},
		{"for i in a; { :; } {foo}\n", "{foo}"},
		{"if :; then :; fi >/dev/null {foo}\n", "{foo}"},
		{"if :; then :; fi {foo\n", "{foo"},
		{"if :; then :; fi {a,b}\n", "{a,b}"},
		{"if :; then :; fi {a}b\n", "{a}b"},
		{"if :; then :; fi { echo x; }\n", "{"},
		{"{ :; } {foo}\n", "{"},
		{"(( 1 )) {foo}\n", "{"},
		{"case x in x) ;; esac {foo}\n", "{"},
		{"f() { :; } {foo}\n", "{"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		want := "parse error near `" + tc.want + "'"
		if got := zsh.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, want)
		}
	}
	// The brace-spelled `if` with no `else` takes a following brace group,
	// which is no refusal at all, and must stay one.
	if _, err := syntax.Parse("if :; { :; } { echo hi; }\n", zsh.Dialect()); err != nil {
		t.Errorf("the brace-spelled if: %v", err)
	}
}
