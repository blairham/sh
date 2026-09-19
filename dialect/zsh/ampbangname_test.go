// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The operator that backgrounds a command and lets go of it is written two
// ways in this shell, and a refusal names it by one of them whichever was
// written: `&!` comes back `&|`. This parser quoted the token as it stood
// (#3747).
//
// Measured 2026-09-19 against zsh 5.9.2 at `/opt/homebrew/bin/zsh`, from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on the null device. Every row below is one of those runs.
func TestTheDisowningOperatorIsNamedByItsPipeSpelling(t *testing.T) {
	for _, src := range []string{
		"; &!\n",
		"; &|\n",
		"&!\n",
		"&|\n",
		"true &! &!\n",
		"{ &! }\n",
		"( &! )\n",
		"while &!; do :; done\n",
		"echo x | &!\n",
		"for &!\n",
		"function &!\n",
	} {
		const want = "parse error near `&|'"
		_, err := syntax.Parse(src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}

// The control, and it is what says the fold is a rule about the **token**
// rather than about the two characters: a word spelled the same way is echoed
// back as it was written, quotes, backslashes and all. Same runs, same day.
//
// The word has to stand where an *unexpected token* is named rather than
// where a production has a sentence of its own — after a compound command,
// not after `for`, which words its own refusal — because that is the sentence
// the fold is written in. There the refused word's token is the bare `&!`
// with its quoting held elsewhere, so the class the parser recorded is the
// only thing parting the two readings, and dropping it turns every row here
// into `&|`.
func TestAWordSpelledLikeTheOperatorKeepsItsOwnSpelling(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"{ :; } '&!'\n", "parse error near `'&!''"},
		{"{ :; } \"&!\"\n", "parse error near `\"&!\"'"},
		{"{ :; } \\&\\!\n", "parse error near `\\&\\!'"},
		{"if true; then :; fi '&!'\n", "parse error near `'&!''"},
		{"(:) '&!'\n", "parse error near `'&!''"},
		// The production with its own wording, for the other half of the
		// same claim: a word is quoted back as written there too, and it
		// never reaches the fold at all.
		{"for '&!'\n", "parse error near `'&!''"},
		{"for \\&\\!\n", "parse error near `\\&\\!'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// The other operators this shell has two spellings for, which do **not** fold
// — the measurement that makes this one flag rather than a table. `;|` is not
// written back as `;;&`, and the pipe-both and the plain pair each name
// themselves.
//
// `;;&` names `;;` in both, which is the lexer reading the `&` as a separate
// token rather than a fold, and it is here because a row that already agrees
// is what a table-shaped fix would have broken.
func TestTheOtherPairedOperatorsKeepTheirSpelling(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"; ;|\n", "parse error near `;|'"},
		{"; ;;&\n", "parse error near `;;'"},
		{"; |&\n", "parse error near `|&'"},
		{"; ;&\n", "parse error near `;&'"},
		{"; &&\n", "parse error near `&&'"},
		{"; ||\n", "parse error near `||'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// Where the operator stands somewhere it belongs, nothing about it changes:
// the fold is how the token is *named when it is refused* and not a respelling
// of the grammar. Both spellings parse to the same tree.
func TestTheOperatorItselfIsUnmoved(t *testing.T) {
	for _, src := range []string{"true &!\n", "true &|\n", "echo x &! y\n"} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	bang, err := syntax.Parse("true &!\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := syntax.Parse("true &|\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if why, ok := syntax.SameProgram(bang, pipe); !ok {
		t.Errorf("the two spellings are not the same program: %s", why)
	}
}
