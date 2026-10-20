// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// Words after the `function` keyword's name are a list of name references:
// read, checked, and discarded. Only the first word names a function.
//
// Measured 2026-09-11 and 2026-09-12 on ksh93u+ over a file, because the
// blame lands on a later line than the words do and `-c` has no lines:
//
//	function a b { print hi; } ⏎ a ⏎ b
//	    hi, then `b: not found` at 127 — `a` is defined and `b` is not
//	function a b c d { print hi; } ⏎ a       hi
//	function a "b" { print hi; } ⏎ a         hi — the quotes come off
//	function a 1b { print hi; }              invalid reference list
//	function a b=c { print hi; }             invalid reference list
//	function a $foo { print hi; }            invalid reference list
//	function a if { print hi; }              `if' unexpected
//	function a b; { print hi; }              `;' unexpected
//	function a b > out { print hi; }         `>' unexpected
//
// All the refusals are status 3. This shell refused the second word outright
// before, which is the whole line rejected where ksh93 runs it (#2014).
func TestExtraWordsAfterTheKeywordName(t *testing.T) {
	for _, src := range []string{
		"function a b { print hi; }\na\n",
		"function a b c d { print hi; }\na\n",
		"function a \"b\" { print hi; }\na\n",
		"function a b\n{ print hi; }\na\n",
	} {
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
		if !ok {
			t.Errorf("%q: not a definition", src)
			continue
		}
		if fn.Name != "a" || len(fn.AlsoNamed) != 0 {
			t.Errorf("%q: name %q with %d more; want `a` alone",
				src, fn.Name, len(fn.AlsoNamed))
		}
	}
}

func TestTheReferenceListIsRefusedByItsOwnSentence(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"function a 1b { print hi; }\n", "syntax error at line 1: invalid reference list"},
		{"function a b=c { print hi; }\n", "syntax error at line 1: invalid reference list"},
		{"function a $foo { print hi; }\n", "syntax error at line 1: invalid reference list"},
		{"function a b; { print hi; }\n", "syntax error at line 1: `;' unexpected"},
		{"function a b > out { print hi; }\n", "syntax error at line 1: `>' unexpected"},
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
}

// The list stops at the end of the line, which is what moves the blame: the
// words are eaten, no brace group follows them on the next line either, and
// the refusal names what stands where the body belonged.
func TestTheListStopsAtTheEndOfTheLine(t *testing.T) {
	const src = "function a echo B\na\n"
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed, where ksh93 answers a syntax error", src)
	}
	const want = "syntax error at line 2: `a' unexpected"
	if got := ksh.Diagnostics().ParseFailure(err); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// And with a brace group on the next line the same header is status 0.
	if _, err := syntax.Parse("function a echo\n{ print hi; }\na\n", ksh.Dialect()); err != nil {
		t.Errorf("a body on the next line was refused: %v", err)
	}
}
