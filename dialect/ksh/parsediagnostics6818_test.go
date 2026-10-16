// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// kshParseRefusal is what this shell writes for src refused by the parser.
func kshParseRefusal(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed; this shell refuses it", src)
	}
	return ksh.Diagnostics().ParseFailure(err)
}

// An arm's terminator with a separator in front of it is named until the end
// of the `case`, except from inside a later arm's command list; a later
// terminator with no separator brings it back. Measured 2026-10-16 on ksh93u+
// 2012-08-01, `-c` and a script file (#6818).
func TestAnArmTerminatorStaysNamedUntilTheNextArmsList(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"case x in x) : ; ;; y", "`;;' unmatched"},
		{"case x in x) : ; ;; y|z", "`;;' unmatched"},
		{"case x in x) : ; ;; (y", "`;;' unmatched"},
		{"case x in x) : ; ;;\necho", "`;;' unmatched"},
		{"case x in x) : ; ;; y) : ;; z", "`;;' unmatched"},
		{"case x in x) ; ;; y) : ;; z", "`;' unmatched"},
		{"case x in x) : ; ;; y)", "`case' unmatched"},
		{"case x in x) : ; ;; y) : ;", "`case' unmatched"},
		{"case x in x) : ;; y) : ;; z", "`case' unmatched"},
	} {
		if got := kshParseRefusal(t, tc.src); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%q: %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}

// An arithmetic command the input runs out inside is named by its single
// parenthesis, as `$((` is, and a backquote left open inside it is the
// backquote. Measured 2026-10-15 on ksh93u+ 2012-08-01, script files
// (#6754).
func TestAnUnclosedArithmeticCommandIsTheParenthesis(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(( 1 +\n", "syntax error at line 1: `(' unmatched"},
		{"((\n", "syntax error at line 1: `(' unmatched"},
		{"for (( i=0\n", "syntax error at line 1: `(' unmatched"},
		{"echo a\n(( $(echo\n", "syntax error at line 2: `(' unmatched"},
		{"(( `echo\n", "syntax error at line 1: ``' unmatched"},
		{"(( ${\n", "syntax error at line 1: `{' unmatched"},
	} {
		if got := kshParseRefusal(t, tc.src); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A `${ cmd;}` body the input ran out in with a construct still open is
// refused at its closing brace, which is a token of the body here, when the
// substitution runs. Measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`, a
// script file and standard input (#6817).
func TestABodyStillOpenAtItsBraceRefusesTheBrace(t *testing.T) {
	const brace = "syntax error at line 1: `}' unexpected"
	for _, c := range []struct{ src, out, err string }{
		{"echo before; echo ${ if :; then }; echo after", "before\n", brace},
		{"echo ${ while :; do }", "", brace},
		{"echo ${ case a in a) }", "", brace},
		{"echo ${ echo a && }", "", brace},
		{"echo ${ [[ a }", "", brace},
		{"echo b\necho ${ if :; then\n:\n}", "b\n", "syntax error at line 4: `}' unexpected"},
		// The controls: a body that ends, and one never run.
		{"echo ${ if :; then :; fi }; echo after", "\nafter\n", ""},
		{"false && echo ${ if :; then }; echo after", "after\n", ""},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if !strings.HasPrefix(out, c.out) ||
			(c.err == "" && (out != c.out || st != 0)) ||
			(c.err != "" && (!strings.HasSuffix(strings.TrimSuffix(out, "\n"), c.err) || st != 3)) {
			t.Errorf("%q: %q (status %d), want %q then %q", c.src, out, st, c.out, c.err)
		}
	}
}
