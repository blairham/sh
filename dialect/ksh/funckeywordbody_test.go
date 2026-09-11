// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// The `function` keyword's body is a brace group here and nothing else — not
// a simple command, and not any other compound command either.
//
// Measured 2026-09-11, ksh93u+ over a file holding `function a`, the body and
// a call to it:
//
//	body                          answer
//	echo B                        syntax error at line 2: `echo' unexpected
//	(( 1 ))                       syntax error at line 2: `((' unexpected
//	( echo B )                    syntax error at line 2: `(' unexpected
//	for i in 1; do echo B; done   syntax error at line 2: `for' unexpected
//	{ echo B; }                   B, status 0
//
// All four refusals are status 3. This shell read and defined every one of
// them at status 0 before, because the body-shape rule was asked in the
// parenthesized production and nowhere else (#1833) — a wrong *acceptance*,
// which nothing reports.
func TestTheKeywordBodyIsABraceGroupOrNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"function a\necho B\na\n", "syntax error at line 2: `echo' unexpected"},
		{"function a\n( echo B )\na\n", "syntax error at line 2: `(' unexpected"},
		{"function a\nfor i in 1; do echo B; done\na\n", "syntax error at line 2: `for' unexpected"},
		{"function a\nwhile false; do :; done\na\n", "syntax error at line 2: `while' unexpected"},
		{"function a\nif true; then :; fi\na\n", "syntax error at line 2: `if' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, where ksh93 answers a syntax error", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{"function a { echo B; }\na\n", "function a\n{ echo B; }\na\n"} {
		if _, err := syntax.Parse(src, ksh.Dialect()); err != nil {
			t.Errorf("%q: a brace group refused: %v", src, err)
		}
	}
}

// The parenthesized form is not held to that rule, which is what makes the two
// separate flags rather than one. ksh93 takes `f() echo hi` and runs it, and
// objects only to what such a body *redirects*.
func TestTheParenthesizedBodyIsStillWider(t *testing.T) {
	if _, err := syntax.Parse("f() echo hi\nf\n", ksh.Dialect()); err != nil {
		t.Errorf("the parenthesized form was held to the keyword's rule: %v", err)
	}
	out, st := answersRun(t, `f() echo hi; f`)
	if out != "hi\n" || st != 0 {
		t.Errorf("f() echo hi; f = %q status %d, want %q at 0", out, st, "hi\n")
	}
}
