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
//
// A flag group's refusal is carried on the node rather than raised, because
// this shell writes it when the word expands — see
// syntax.Dialect.FlagGroupRefusedAtExpansion. The sentence is the same
// sentence either way, which is the whole point of carrying the failure
// rather than rebuilding one, so the rows below are unchanged by the move.
func refusal(t *testing.T, src string) string {
	t.Helper()
	f, err := syntax.Parse(src+"\n", ksh.Dialect())
	if err == nil {
		if carried := carriedRefusal(f); carried != nil {
			err = carried
		}
	}
	if err == nil {
		return "parsed"
	}
	return ksh.Diagnostics().ParseFailure(err)
}

// carriedRefusal is the first deferred parse failure in a one-command file,
// which is every row here.
func carriedRefusal(f *syntax.File) error {
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range pipe.Cmds {
			cmd, ok := c.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, w := range cmd.Args {
				for _, sp := range w.Spans {
					if sp.Param != nil && sp.Param.RefusedAtExpansion != nil {
						return sp.Param.RefusedAtExpansion
					}
				}
			}
		}
	}
	return nil
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
		// delimiter is escaped: the `)` search honors the backslash.
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

// The tail is not the rest of the word unconditionally: it stops at an
// operator character even inside the quotes, and at a `(` standing where a
// function definition's would. Measured 2026-09-12 against ksh93u+
// 2012-08-01, `env -i` with a scratch HOME, over `-c`.
func TestARefusedFlagGroupsTailStopsAtAnOperator(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "${(U)a|b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a;b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a&b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a>b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a)b}"`, "syntax error at line 1: `a' unexpected"},
		{`echo "${(U)a(b}"`, "syntax error at line 1: `a' unexpected"},
		// The corpus row this reaches: the assignment form's tail is cut at
		// the blank rather than carried to the closing brace.
		{`printf "[%s]" "${(A)u=x y}"`, "syntax error at line 1: `u=x' unexpected"},
		// And the discriminating pair. A `(` after a name character is where
		// a definition's would be and cuts the tail; one after anything else
		// is a pattern group, and its `)` closes it rather than ending the
		// tail — so neither "every `(` cuts" nor "balanced parens are kept"
		// is the rule.
		{`echo "${(U)a-(b)c}"`, "syntax error at line 1: `a-(b)c}\"\"' unexpected"},
		{`echo "${(U)a@(b)c}"`, "syntax error at line 1: `a@(b)c}\"\"' unexpected"},
		// Nor does the scan reach inside a substitution the tail carries:
		// both of these hold a blank and both are quoted back whole.
		{`echo ${(@f)"$(printf "a b")"}`, "syntax error at line 1: `\"$(printf \"a b\")\"}' unexpected"},
		{"echo \"${(U)`printf a b`}\"", "syntax error at line 1: ``printf a b`}\"\"' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// A tail the shell cannot flatten keeps the quotes it was written with, and
// the closing quote of the word it stands in is written a *second* time. The
// characters that do that are the two that begin an expansion, the three
// that make a pattern and the one that begins a brace range — measured a
// character at a time in the same run, because "any punctuation" would have
// been wrong for six of them.
func TestARefusedFlagGroupsTailDoublesTheClosingQuote(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "${(U)a[2]}"`, "syntax error at line 1: `a[2]}\"\"' unexpected"},
		{`echo "${(U)a$p}"`, "syntax error at line 1: `a$p}\"\"' unexpected"},
		{`echo "${(U)a*b}"`, "syntax error at line 1: `a*b}\"\"' unexpected"},
		{`echo "${(U)a?b}"`, "syntax error at line 1: `a?b}\"\"' unexpected"},
		{`echo "${(U)a{b}}"`, "syntax error at line 1: `a{b}}\"\"' unexpected"},
		// The word's closing quote, wherever it stands — not two characters
		// bolted on the end.
		{`echo x"${(U)a$p}"`, "syntax error at line 1: `a$p}\"\"' unexpected"},
		{`echo "${(U)a$p}"x`, "syntax error at line 1: `a$p}\"x\"' unexpected"},
		// The unquoted word is the discriminator: the same tail, the same
		// subscript, and nothing is doubled — so it is the word's quote
		// being written twice rather than a marker the subscript leaves.
		{`echo ${(U)a[2]}`, "syntax error at line 1: `a[2]}' unexpected"},
		{`echo ${(U)a$p}`, "syntax error at line 1: `a$p}' unexpected"},
		// A tail whose quotes are balanced has no closing one to double.
		{`echo ${(U)x}"q"`, "syntax error at line 1: `x}q' unexpected"},
		// And the characters that do *not* do it, each of which would have
		// been swept up by a wider rule.
		{`echo "${(U)a#b}"`, "syntax error at line 1: `a#b}' unexpected"},
		{`echo "${(U)a%b}"`, "syntax error at line 1: `a%b}' unexpected"},
		{`echo "${(U)a!b}"`, "syntax error at line 1: `a!b}' unexpected"},
		{`echo "${(U)a=b}"`, "syntax error at line 1: `a=b}' unexpected"},
		{`echo "${(U)a~b}"`, "syntax error at line 1: `a~b}' unexpected"},
		{`echo "${(U)a]b}"`, "syntax error at line 1: `a]b}' unexpected"},
		{`echo "${(U)a@b}"`, "syntax error at line 1: `a@b}' unexpected"},
		{`echo "${(U)a/b}"`, "syntax error at line 1: `a/b}' unexpected"},
		{`echo "${(U)a,b}"`, "syntax error at line 1: `a,b}' unexpected"},
		{`echo "${(U)a^b}"`, "syntax error at line 1: `a^b}' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// And *when* it is written: this shell refuses a flag group when the word
// expands, where it refuses every other unreadable expansion while it reads
// the line. Measured 2026-09-18 on ksh93u+ 2012-08-01 from a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on
// /dev/null — `echo one; echo "${(f)x}"; echo two` writes `one` and then the
// refusal at 3, and `echo one; if false; then echo "${(f)x}"; fi; echo two`
// writes both and exits 0, where `${%%%}` in either line is refused before
// `one` runs. See syntax.Dialect.FlagGroupRefusedAtExpansion (#3013).
func TestAFlagGroupIsRefusedWhenTheWordExpands(t *testing.T) {
	if !ksh.Dialect().FlagGroupRefusedAtExpansion {
		t.Error("FlagGroupRefusedAtExpansion = false, want true")
	}
	f, err := syntax.Parse("echo one; echo \"${(f)x}\"; echo two\n", ksh.Dialect())
	if err != nil {
		t.Fatalf("parse: %v, want the line to stand so the commands in front of the word can run", err)
	}
	if carriedRefusal(f) == nil {
		t.Error("nothing carried, want the refusal held for the expansion")
	}
	// Every other unreadable expansion still gives up while it is read.
	if _, err := syntax.Parse("echo one; echo ${%%%}; echo two\n", ksh.Dialect()); err == nil {
		t.Error("parsed, want a refusal that is not a flag group still raised at the read")
	}
}
