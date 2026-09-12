// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// refusal parses src under this dialect and words the failure the way the
// shell would, so a row below is the line the binary prints.
func refusal(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src+"\n", ksh.Dialect())
	if err == nil {
		return "parsed"
	}
	return ksh.Diagnostics().ParseFailure(err)
}

// This shell has no expansion flag groups, so `${(U)x}` is a parse-time
// refusal — and what it quotes back is the rest of the *word* rather than the
// `(` it refused. Measured 2026-09-12 against ksh93u+ 2012-08-01, `env -i`
// with a scratch HOME, over `-c`. See syntax.Error.FlagGroupWordTail.
func TestARefusedFlagGroupNamesTheRestOfTheWord(t *testing.T) {
	if !ksh.Diagnostics().FlagGroupNamesTheWordTail {
		t.Fatal("FlagGroupNamesTheWordTail = false, want true")
	}
	for _, c := range []struct{ src, want string }{
		{`echo ${(U)x}`, "syntax error at line 1: `x}' unexpected"},
		// The discriminating row: the extent is the word and not the line,
		// so ` after` is not carried with it.
		{`echo ${(U)x} after`, "syntax error at line 1: `x}' unexpected"},
		// And it runs past the closing brace to the end of the word.
		{`echo a${(U)x}b c`, "syntax error at line 1: `x}b' unexpected"},
		// A group whose argument holds the delimiter, and one whose
		// delimiter is escaped: the `)` search honours the backslash.
		{`echo ${(s.:.)x}`, "syntax error at line 1: `x}' unexpected"},
		{`echo ${(ps:\):)x}`, "syntax error at line 1: `x}' unexpected"},
		// The idiom the corpus carries, quotes and all — a tail holding an
		// expansion keeps the quoting it was written with.
		{`echo ${(@f)"$(printf ab)"}`, "syntax error at line 1: `\"$(printf ab)\"}' unexpected"},
		{"echo ${(U)`printf ab`}", "syntax error at line 1: ``printf ab`}' unexpected"},
		{`echo ${(U)x$vy}`, "syntax error at line 1: `x$vy}' unexpected"},
		// A tail with no expansion in it is quoted back with its quotes
		// off, which is measured in all three positions.
		{`echo ${(U)"x"}`, "syntax error at line 1: `x}' unexpected"},
		{`echo ${(U)a"b"c}`, "syntax error at line 1: `abc}' unexpected"},
		{`echo "[${(U)x}]"`, "syntax error at line 1: `x}]' unexpected"},
		{`echo ${(U)x}"q"`, "syntax error at line 1: `x}q' unexpected"},
		// A group holding a nested `(` is one the search cannot read, and
		// the shell goes back to naming the `(` there too.
		{`echo ${(l(3))x}`, "syntax error at line 1: `(' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// The tail is the flag group's alone. Every other thing this shell refuses
// inside a `${…}` while reading it still names the one character, measured in
// the same run — so a rule written over the whole construct would have been
// wrong four ways.
func TestTheOtherBraceRefusalsStillNameOneCharacter(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo ${~x} after`, "syntax error at line 1: `~' unexpected"},
		{`echo ${=x} after`, "syntax error at line 1: `=' unexpected"},
		{`echo ${^x} after`, "syntax error at line 1: `^' unexpected"},
		{`echo ${+x} after`, "syntax error at line 1: `+' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
