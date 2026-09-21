// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// anonDialect is the core with the nameless function on, which is the one
// grammar where `()` where a command begins has two readings to choose
// between. Named for the flag rather than for a shell, as everything in this
// package is.
// [Dialect.EmptyCompoundBody] with it, because the reading the blank chooses
// is a subshell holding nothing and the core refuses that outright — without
// it the rows below would part company over whether an empty body is legal
// rather than over which command the pair is.
func anonDialect() Dialect {
	d := Core()
	d.AnonymousFunction = true
	d.EmptyCompoundBody = true
	return d
}

// A nameless function's parameter list is the two parentheses **adjacent**,
// and anything at all between them is an empty subshell instead (#3961).
//
// Measured 2026-09-21 on zsh 5.9.2 — the one shell in the panel with the
// construct — from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with standard input on the null device:
//
//	`()` and a newline          parse error near `\n' — a body never came
//	`()` ⏎ `echo $0`            (anon) — the next line *is* the body
//	`() { echo $0; }`           (anon)
//	`( )`                       accepted, and runs nothing
//	`( )` ⏎ `echo $0`           s.sh — a subshell and then a command
//	`( ) { echo $0; }`          parse error near `{'
//	`(` ⏎ `)`                   accepted
//
// The blank was skipped here, so `( ) { echo hi; }` was a function with a
// body where the reference refuses the brace, and `()` alone was accepted in
// silence because the no-body case fell back to reading the pair as the
// empty subshell it is only when something stands between them.
func TestAnAnonymousFunctionsParensMustBeAdjacent(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src string
		// anon is whether the first command is the nameless function.
		anon bool
	}{
		{"adjacent, with a brace body", "() { :; }\n", true},
		{"adjacent, with the body on the next line", "()\n:\n", true},
		{"a blank between them", "( ) \n", false},
		{"two blanks between them", "(  )\n", false},
		{"a newline between them", "(\n)\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src, anonDialect())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0]
			if _, ok := cmd.(*AnonFunc); ok != c.anon {
				t.Errorf("%q came to %T, want anonymous function = %v", c.src, cmd, c.anon)
			}
		})
	}
	// And the pair that says the blank changes what the *next* words are: a
	// subshell is a complete command, so a brace group behind it is a token
	// the grammar has nowhere to put.
	if _, err := Parse("( ) { :; }\n", anonDialect()); err == nil {
		t.Error("`( ) { :; }` parsed; the reference refuses the brace after an empty subshell")
	}
	if _, err := Parse("() { :; }\n", anonDialect()); err != nil {
		t.Errorf("`() { :; }` was refused: %v", err)
	}
}

// And a nameless function with no body at all is refused rather than read as
// the empty subshell, marked as the missing body it is so that the dialect
// which numbers this failure from the parentheses can find it.
//
// `()` alone at the end of a file is “parse error near `\n' “ on zsh 5.9.2
// where this accepted it and ran nothing.
func TestAnAnonymousFunctionNeedsABody(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"()", "()\n", "echo hi; ()\n"} {
		_, err := Parse(src, anonDialect())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: refusal = %v, want a syntax error", src, err)
		}
		if !se.FuncBody {
			t.Errorf("%q: not marked as a missing function body", src)
		}
	}
	// The extent walk reaches a refused node through whatever encloses it,
	// so a nameless function with no body has to answer where it ends. It
	// was unreachable while `()` was accepted, and the first input that made
	// one crashed the parser.
	for _, src := range []string{"for (()", "if (()"} {
		func() {
			defer func() {
				if e := recover(); e != nil {
					t.Fatalf("%q panicked: %v", src, e)
				}
			}()
			_, _ = Parse(src, anonDialect())
		}()
	}
}
