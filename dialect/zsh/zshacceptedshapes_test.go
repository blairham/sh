// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Five shapes real zsh accepts and runs were parse errors here, so the script
// died at status 1 instead of running (#3898).
//
// They are four causes and not five bugs: the two dangling-operator rows share
// one, and the other three shapes have one each. syntax's own
// zshacceptedshapes_test.go names the flag each cause belongs to; this file is
// what the reference does.
//
// Measured 2026-09-20, each line its own script file under
// `env -i -u FPATH PATH=/usr/bin:/bin LC_ALL=C zsh case.zsh` with no standard
// input, against zsh 5.9.2:
//
//	v=$(echo hi; echo x &&); print -r -- "v=[$v]"     v=[hi\nx]
//	v=$(echo hi; echo x ||); print -r -- "v=[$v]"     v=[hi\nx]
//	export z=a}; print -r -- "[$z]"                   [a}]
//	case x in (a}) print P;; (*) print D;; esac       D
//	select o in a b c; | :; print T                   the menu, then T
//
// The `}` rows are not even zsh-only: `export z=a}` and `case x in (a})` are
// taken by bash 5.3.20, bash 3.2.57, ksh93u+ and dash 0.5.12 as well, the
// brace being an ordinary character in every column but this one. So this
// preset stood alone against the whole panel on two of the five.
func TestFiveShapesRealZshAcceptsRun(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a dangling && in a substitution", `v=$(echo hi; echo x &&); print -r -- "v=[$v]"`, "v=[hi\nx]\n"},
		{"a dangling || in a substitution", `v=$(echo hi; echo x ||); print -r -- "v=[$v]"`, "v=[hi\nx]\n"},
		{"a close brace in an export operand", `export z=a}; print -r -- "[$z]"`, "[a}]\n"},
		{"a close brace in a case pattern", `case x in (a}) print P;; (*) print D;; esac`, "D\n"},
		{"an empty select body before a bar", `select o in a b c; | :; print T`, "1) a  2) b  3) c  \n?# \nT\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The neighbors each cause reaches, and the rows that must not move with it.
//
// Same measurement run, same arrangement. Every `want` here is what zsh 5.9.2
// printed.
func TestTheFourCausesReachTheirNeighbors(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The and-or cause is about the closing delimiter, so the older
		// substitution spelling takes it too, and an operand that is nothing
		// but the operator is the empty string rather than a refusal.
		{"the backquoted spelling", "v=`echo x &&`; print -r -- \"[$v]\"", "[x]\n"},
		{"nothing but the operator", `v=$( : || ); print -r -- "[$v]"`, "[]\n"},
		// The declaration cause is the operand's shape, so every spelling of
		// the utility takes it and every operand does.
		{"typeset", `typeset z=a}; print -r -- "[$z]"`, "[a}]\n"},
		{"local", `local z=a}; print -r -- "[$z]"`, "[a}]\n"},
		{"readonly", `readonly z=a}; print -r -- "[$z]"`, "[a}]\n"},
		{"the second operand too", `export z=a} w=b}; print -r -- "[$z][$w]"`, "[a}][b}]\n"},
		{"a value that is only the brace", `export z=}; print -r -- "[$z]"`, "[}]\n"},
		// The case cause is the parentheses, so an alternation takes it.
		{"an alternation", `case x in (a}|b) print P;; (*) print D;; esac`, "D\n"},
		{"and it still matches", `case "a}" in (a}) print P;; (*) print D;; esac`, "P\n"},
		// The short-body cause is the joining operators, so `for` and
		// `repeat` take it as `select` does, and both bar spellings do.
		{"a for loop", `for i in a b; | :; print T`, "T\n"},
		{"the other bar", `for i in a b; |& :; print T`, "T\n"},
		{"repeat", `repeat 2; | :; print T`, "T\n"},
		{"and the loop succeeded", `for i in a b; && print x`, "x\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The controls: rows zsh 5.9.2 refuses, which this preset must go on refusing.
//
// They are what make each cause the narrow thing it is rather than a scanner
// that stopped caring. Measured in the same run: every one of these is a parse
// error at status 1 in zsh, as it was and is here.
func TestTheNeighborsThatStayRefused(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		// The brace is text in an assignment, not behind any utility.
		{"an operand that is not an assignment", `export -- a}; print ok`},
		{"a utility that takes no assignment", `alias z=a}; print ok`},
		{"a prefix does not reach the argument", `x=1 print -r -- a}`},
		{"an ordinary argument", `print -r -- a}`},
		// A `case` arm written without parentheses keeps the reserved word.
		{"a bare case arm", `case x in a}) print P;; *) print D;; esac`},
		// And the tokens that are not joining operators, plus the long form,
		// which is not lenient at all.
		{"an ampersand after a short header", `for i in a b; & print x`},
		{"a case terminator after one", `for i in a b; ;; print x`},
		{"the long form before a bar", `for i in a b; do :; done; | :`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseZsh(c.src); err == nil {
				t.Errorf("%q parsed, want the refusal zsh gives", c.src)
			}
		})
	}
}

// The and-or leniency belongs to the and-or list alone, which is the
// discriminating half of #3898's first cause: zsh takes a substitution body
// ending on `&&` or `||` and refuses one ending on `|` or on a stray `;;`.
//
// These two need a **run** rather than a parse, and that is the trap the issue
// warned about from the other side: a substitution's body is not read until the
// word is expanded, so the outer text parses either way and a static check
// passes against the bug. Measured 2026-09-20: both are a parse error at
// status 1 in zsh 5.9.2, and both were here before this change and after it.
func TestASubstitutionBodyStillRefusesTheOtherOperators(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a pipeline never takes it", `v=$(echo x |); print -r -- "[$v]"`},
		{"nor does a case terminator", `v=$(echo x ;;); print -r -- "[$v]"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, st := runZsh(t, t.TempDir(), c.src); st == 0 {
				t.Errorf("%q ran at 0, want the refusal zsh gives", c.src)
			}
		})
	}
}

// Where a refusal *lands*, for the shapes that keep one. The short-body cause
// moves the token a refusal names, and that is the half a parsed/refused table
// cannot see — a row that goes on being refused for a different reason reads
// exactly like a row that did not move.
//
// Measured 2026-09-20, same arrangement as the tables above, against zsh
// 5.9.2. Each line is its own script file and the quoted word is the whole of
// what zsh named:
//
//	while & do :; done      parse error near `&'
//	until & do :; done      parse error near `&'
//	while | do :; done      parse error near `do'
//	while |& do :; done     parse error near `do'
//	while && do :; done     parse error near `do'
//	while || do :; done     parse error near `do'
//
// So the `&` rows name the operator where it stands and the four joining
// operators do not: a loop written in front of one is its left-hand side, and
// `do` is then a right-hand side that cannot begin a command. `&` terminates a
// list rather than joining two commands, which is why it is the row that does
// not move.
//
// The whole panel refuses all six — only zsh has the short form at all — so
// this is a fact about the one column that has the grammar, and syntax's
// TestAShortBodyRefusesTheTokenThatIsThere is the same question asked of a
// grammar with [syntax.Dialect.ShortForm] and without
// [syntax.Dialect.ShortBodyEndsOnAJoiningOperator], where the operator is
// still refused where it stands.
func TestWhereAShortBodyRefusalLands(t *testing.T) {
	for _, c := range []struct{ src, token string }{
		{`while & do :; done`, "&"},
		{`until & do :; done`, "&"},
		{`while | do :; done`, "do"},
		{`while |& do :; done`, "do"},
		{`while && do :; done`, "do"},
		{`while || do :; done`, "do"},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, err := parseZsh(c.src)
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("%q: err = %v, want the refusal zsh gives", c.src, err)
			}
			if se.Token != c.token {
				t.Errorf("%q named %q, want %q — the token zsh names", c.src, se.Token, c.token)
			}
		})
	}
}
