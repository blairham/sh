// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// This shell has no nested expansions, so `${${v}}` is a parse-time refusal —
// and the token it quotes back is a `!`, a character the input does not hold
// anywhere. Measured 2026-09-14 against ksh93u+ 2012-08-01, `env -i` with a
// scratch HOME, over `-c`, a script file and standard input alike. See
// syntax.Error.NestedInTheNamePosition and
// interp.Diagnostics.NestedNameIsBlamedOnTheBang.
func TestASecondExpansionInTheNamePositionIsBlamedOnABang(t *testing.T) {
	if !ksh.Diagnostics().NestedNameIsBlamedOnTheBang {
		t.Fatal("NestedNameIsBlamedOnTheBang = false, want true")
	}
	want := "syntax error at line 1: `!' unexpected"
	for _, src := range []string{
		`echo ${${v}}`,
		// The quoting makes no difference, inside or around.
		`echo "${${v}}"`,
		// Nor does a length in front of it, an operator after it, a
		// subscript on it, or literal text on either side.
		`echo ${#${v}}`,
		`echo ${${v}:-x}`,
		`echo ${${v}[2]}`,
		`echo A${${v}}B`,
		// Nor an empty inner, a flag group inside it, or a third level.
		`echo ${${}}`,
		`echo ${${(U)v}}`,
		`echo "${${${v}}}"`,
		// And it is the refusal of the *word*, not of the command: the
		// assignment form is the same sentence.
		`x=${${v}}`,
	} {
		if got := refusal(t, src); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// The `!` is the `${` standing in the name position and nothing wider. Every
// neighbor here holds the same characters somewhere else and is blamed on
// the character that really stood there, measured in the same run — so a rule
// written over "a `$` in a `${…}`" would have been wrong three ways.
func TestTheBangIsTheNamePositionAndNotAnyNestedDollar(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A `$` that opens no brace is the `$` parameter with a second one
		// after it, and that second `$` is what is named.
		{`echo ${$$}`, "syntax error at line 1: `$' unexpected"},
		// A `${` that is not in the name position — there is a name in
		// front of it — is named for its `$`.
		{`echo ${x${v}}`, "syntax error at line 1: `$' unexpected"},
		// A flag group is the other refusal of this construct and keeps its
		// own rule: the rest of the word.
		{`echo ${(U)a}`, "syntax error at line 1: `a}' unexpected"},
		// And the one-character refusals this construct already had.
		{`echo ${~x} after`, "syntax error at line 1: `~' unexpected"},
		{`echo ${}`, "syntax error at line 1: `}' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
