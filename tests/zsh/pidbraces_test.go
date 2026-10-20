// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A `{ … }` run written immediately after `$$` under the whole preset
// (#3091, the last construct of #3040).
//
// The grammar is syntax.Dialect.PidBraceGroupIsText and the expansion half is
// syntax.Span.PidBrace; this is the two of them together on the dialect that
// actually sets the flag, plus the sentence the refusal gets — which no test
// in either package can reach, because a diagnostic is worded here.
//
// The pid is stripped from each word rather than printed, since it is not the
// same number twice. What is left is the braces and the word count.
func TestAPidBraceRunUnderThisDialect(t *testing.T) {
	const show = "show() { printf 'n=%s' \"$#\"; for a in \"$@\"; do printf ' [%s]' \"${a#$$}\"; done; print; }\n"
	for _, tc := range []struct{ name, src, want string }{
		{"a blank does not end the word", "show $${a b}", "n=1 [{a b}]\n"},
		{"nor a semicolon", "show $${a;b}", "n=1 [{a;b}]\n"},
		{"nor a redirection operator", "show $${a>b}", "n=1 [{a>b}]\n"},
		{"the braces are not a list", "show $${a,b}", "n=1 [{a,b}]\n"},
		{"a pair nested inside still is", "show $${a{b,c}d}", "n=2 [{abd}] [{acd}]\n"},
		{"a range in the outer pair still fires", "show $${1..3}", "n=3 [1] [2] [3]\n"},
		{"a comma is no range", "show $${a,1..3}", "n=1 [{a,1..3}]\n"},
		{"the run ends at its match", "show $${a} b", "n=2 [{a}] [b]\n"},
		{"text between leaves an ordinary list", "show $$x{a,b}", "n=2 [xa] [xb]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, show+tc.src+"\n")
			if out != tc.want || st != 0 {
				t.Errorf("%s: got %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// TestAnUnmatchedPidBraceGetsThisDialectsSentence. The run swallows every
// line after it looking for the match, and the refusal that follows is worded
// the way this shell words an unclosed `${` — measured 2026-09-15, `echo ${a`
// and `echo $${a` over the same two-line script both answer `closing brace
// expected` at line 3, the end of the input. The whole rendered sentence is
// asserted, line and all: this shell puts the line inside it.
func TestAnUnmatchedPidBraceGetsThisDialectsSentence(t *testing.T) {
	const src = "echo $${a\necho two\n"
	const want = "s.sh:3: closing brace expected\n"
	_, err := syntax.Parse(src, zsh.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses an unmatched run")
	}
	d := zsh.Diagnostics().ForScript()
	if got := d.ParseDiagnostic("s.sh", "", err, src); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := d.SyntaxStatus(), 1; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// And the control beside it: the same sentence for the construct this one
// borrows its wording from, so a row that passed because every refusal here
// says the same thing would be visible.
func TestTheUnmatchedRunAndTheUnmatchedExpansionAgree(t *testing.T) {
	d := zsh.Diagnostics().ForScript()
	const other = "echo ${a\necho two\n"
	_, err := syntax.Parse(other, zsh.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses an unclosed `${`")
	}
	if got, want := d.ParseDiagnostic("s.sh", "", err, other), "s.sh:3: closing brace expected\n"; got != want {
		t.Errorf("the `${` row moved: got %q, want %q", got, want)
	}
	// A refusal this shell words differently, so the two rows above are not
	// simply every diagnostic it has.
	const third = "case a in ) echo m;; esac\n"
	_, err = syntax.Parse(third, zsh.Dialect())
	if err == nil {
		t.Fatal("parsed; this shell refuses an empty pattern list")
	}
	if got := d.ParseDiagnostic("s.sh", "", err, third); !strings.Contains(got, "parse error near") {
		t.Errorf("the control row = %q, want a different sentence", got)
	}
}
