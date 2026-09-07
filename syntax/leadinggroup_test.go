// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The positions where a parenthesised group standing at the *front* of a word
// belongs to the word: a loop's item list, `select`'s, and a `case` arm's
// pattern. `echo (#i)a` and `files=( (#i)a )` were the first two (#839,
// #1149); these are the rest of them (#1161).
//
// Measured 2026-09-06 on zsh 5.9.2, `-n` over a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME and ZDOTDIR. Every row
// below is accepted there and refused by bash 5.3, bash 3.2, dash and ksh93
// alike, which is what makes it one dialect's answer and not core.

// loopGlobQuals is globQuals plus the loop forms these rows need.
func loopGlobQuals() Dialect {
	d := globQuals()
	d.Select = true
	d.ShortForm = true
	d.Foreach = true
	return d
}

func TestAParenWhereALoopItemBeginsBelongsToTheItem(t *testing.T) {
	on := loopGlobQuals()
	off := Core()
	off.Select = true
	for _, src := range []string{
		`for x in (#i)a; do :; done`,
		`for x in a (#i)b; do :; done`,
		`select x in (#i)a; do :; done`,
		`select x in a (#i)b; do :; done`,
	} {
		mustParse(t, src, on, "a group beginning a loop item is part of it")
		mustFail(t, src, off, "and it is not, without the flag")
	}
}

func TestAParenWhereACaseArmsPatternBeginsBelongsToThePattern(t *testing.T) {
	on := loopGlobQuals()
	off := Core()
	for _, src := range []string{
		// The flag with no arm paren in front of it: the group is the
		// pattern's and the arm's `)` is the one behind it.
		`case x in (#i)a) :;; esac`,
		`case x in (#i)a|b) :;; esac`,
		// And with the arm's own paren in front, which is the shape
		// `~/.zi/bin/lib/zsh/install.zsh:1580` is written in.
		`case x in ((#i)a) :;; esac`,
		`case x in ((#i)*.zip) :;; esac`,
		// A later arm reaches the same state, which is a separate read.
		`case x in a) :;; ((#i)b) :;; esac`,
	} {
		mustParse(t, src, on, "a group beginning a case arm's pattern is part of it")
		mustFail(t, src, off, "and it is not, without the flag")
	}
}

// An arm's paren in front of a plain group has no `#` in it at all, which is
// what says the `((` half is not about the flag's spelling: it is the lexer
// reading two parens as an arithmetic command where no command may begin.
func TestAnArmsParenInFrontOfAGroupIsNotAnArithmeticCommand(t *testing.T) {
	on := loopGlobQuals()
	for _, src := range []string{
		`case x in ((a|b)) :;; esac`,
		`case x in ((a)) :;; esac`,
		`case x in ((a)|b) :;; esac`,
		// No pattern language in it whatsoever — the row that separates the
		// arithmetic-command reading from the group reading.
		`case x in ((1)) :;; esac`,
	} {
		mustParse(t, src, on, "an arm's paren in front of a group")
	}
	// The suspension is the arm's and not the whole construct's: an arm's
	// *body* is ordinary commands, and `((1))` in one really is an
	// expression. A flag left on for the body would have read this as a
	// pattern.
	mustParse(t, `case x in a) ((1)); :;; esac`, on,
		"an arithmetic command in an arm's body")
}

// The rows that were already right, kept so the boundary is legible: an arm
// whose leading paren is its own, and a group standing *after* pattern text.
func TestTheOrdinaryCaseArmShapesStillParse(t *testing.T) {
	for _, d := range []Dialect{loopGlobQuals(), Core()} {
		for _, src := range []string{
			`case x in a) :;; esac`,
			`case x in (a) :;; esac`,
			`case x in a|b) :;; esac`,
			`case x in (a|b) :;; esac`,
			`case x in *.zip) :;; esac`,
			`case x in a) :;; (b) :;; esac`,
			`case x in a) :;; b) :;; esac`,
		} {
			mustParse(t, src, d, "an ordinary case arm")
		}
	}
	// And the group after pattern text, which needs the group grammar.
	mustParse(t, `case x in *.(zip|tgz)) :;; esac`, loopGlobQuals(),
		"a group behind pattern text")
}

// **No word in a loop's item list is a reserved word.** Unanimous across the
// panel, and this had both directions wrong at once. Core, not a dialect's.
//
//	for x in a b do :; done      refused by bash 5.3, bash 3.2, dash,
//	                             ksh93 and zsh — `do` is a *word* there, so
//	                             `done` stands where `do` belongs
//	for x in do; do :; done      taken by all five
//	for x in done; do :; done    taken by all five
func TestNoWordInALoopsItemListIsReserved(t *testing.T) {
	for _, d := range []Dialect{Core(), loopGlobQuals()} {
		for _, src := range []string{
			`for x in do; do :; done`,
			`for x in done; do :; done`,
			`for x in then; do :; done`,
			`for x in esac fi else; do :; done`,
			"for x in do\ndo :; done",
			`for x in a b; do :; done`,
			"for x in a b\ndo :; done",
		} {
			mustParse(t, src, d, "a reserved word is an ordinary item")
		}
		// The other direction, and the worse half: a header with no
		// separator is refused everywhere, and accepting it ran the body
		// with `do` bound as a value.
		for _, src := range []string{
			`for x in a b do :; done`,
			`for x in a b then :; done`,
		} {
			mustFail(t, src, d, "the list ends at a separator and nothing else")
		}
	}
	// `select` and `foreach` read the same list, and each had its own copy
	// of the loop before.
	sel := loopGlobQuals()
	mustParse(t, `select x in do; do :; done`, sel, "select's list is the same list")
	mustFail(t, `select x in a b do :; done`, sel, "and it ends the same way")
	mustParse(t, "foreach x in a b end\n:\nend", sel, "foreach's list too")
	mustParse(t, "foreach x in do\n:\nend", sel, "including a reserved word")
}

// The parenthesised list gets both answers as well, which is the fourth
// position: `for x (do)` and `for x (a do)` are taken by the shell that has
// the form, and `for x ((#i)a)` reads the flag.
func TestTheParenthesisedItemListReadsTheSameWords(t *testing.T) {
	on := loopGlobQuals()
	for _, src := range []string{
		`for x (do); do :; done`,
		`for x (done); do :; done`,
		`for x (a do); do :; done`,
		`select x (do); do :; done`,
	} {
		mustParse(t, src, on, "a reserved word is an ordinary item there too")
	}
	// And a group beginning a *later* element of it, which is the position
	// the flag reaches there. The first element cannot be reached the same
	// way — `for x ((#i)a)` is lexed as an arithmetic command before the
	// list's own paren is ever seen, which is a different question and is
	// recorded in the spec as still refused.
	off := Core()
	off.ShortForm = true
	for _, src := range []string{
		`for x (a (#i)b); do :; done`,
		`select x (a (#i)b); do :; done`,
	} {
		mustParse(t, src, on, "a group beginning a later parenthesised item")
		mustFail(t, src, off, "and it does not, without the flag")
	}
	mustFail(t, `for x ((#i)a); do :; done`, on,
		"the list's own opening paren is lexed as an arithmetic command, which "+
			"this change does not reach")
}
