// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The word a `-` or `+` substitutes into a *pattern* is part of the pattern,
// and it never reaches the filesystem — #1955.
//
// It used to reach it, because the word was expanded the ordinary way: matched
// against the directory and then handed back as a value. That is wrong twice
// over, and the two wrongs hid each other. Measured 2026-09-10 across bash
// 5.3.15, bash 3.2.57 and zsh 5.9.2:
//
//	r=a; case abc in ${r:+a*}) ;;          matches in all three
//	r=a; x=abc; printf %s ${x%${r:+b*}}    is `a` in all three
//
// Both answered the other way here: the listing the word found was escaped as
// a value, so `a*` stopped being a pattern at the moment it was replaced by
// the names it matched.
//
// The rows below are written so that the filesystem cannot supply the answer:
// the word names nothing in the directory, or names something the subject is
// not. A word that were matched again would give the other answer to every
// one of them.
func TestASubstitutedWordInAPatternIsNeverMatchedAgainstTheFilesystem(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The word matches a file, and the file is not the subject: the
		// listing and the pattern give opposite answers, which is what makes
		// each row discriminating. `vis` is in the directory and `vix` is
		// not, so a matched word answers no and a pattern answers yes.
		{
			"`+` in a case arm", `r=a; case vix in ${r:+v*}) printf yes;; *) printf no;; esac`,
			"yes",
		},
		{
			"`-` in a case arm", `unset u; case vix in ${u:-v*}) printf yes;; *) printf no;; esac`,
			"yes",
		},
		{
			"and the colonless spelling", `unset u; case vix in ${u-v*}) printf yes;; *) printf no;; esac`,
			"yes",
		},
		// A trim is the same entry point, and the operand of the outer
		// expansion is where the startup line came from.
		{"a trim's operand", `unset u; x=avix; printf "[%s]" "${x%${u:-v*}}"`, `[a]`},
		{"and the `+` spelling of it", `r=a; x=avix; printf "[%s]" "${x%${r:+v*}}"`, `[a]`},
		// Quoted, the word is text — which is the other half of the rule and
		// is unanimous in the three.
		{
			"a quoted word is text", `r=a; case abc in ${r:+"a*"}) printf yes;; *) printf no;; esac`,
			"no",
		},
		{
			"and the quoting is per span", `r=a; case 'a*' in ${r:+"a*"}) printf yes;; *) printf no;; esac`,
			"yes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// The condition is the fourth route through the same entry point, and it is
// the one the real startup took.
func TestASubstitutedWordInAConditionIsAPattern(t *testing.T) {
	dir := globDir(t)
	brackets := func(d *syntax.Dialect) { d.DoubleBracket = true }
	for _, tc := range []struct{ name, src, want string }{
		{"the word is the pattern", `r=a; [[ vix == ${r:+v*} ]] && printf yes || printf no`, "yes"},
		{"quoted, it is text", `r=a; [[ vis == "${r:+v*}" ]] && printf yes || printf no`, "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runGrammar(t, tc.src, brackets, func(r *Runner) { r.Dir = dir })
			if got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// Where an unmatched pattern is fatal, matching the word was not a wrong
// answer but a stopped script — which is how #1955 was found: one
// `no matches found: |shim-list` on a real startup, from a word the shell it
// is measured against never sends to the filesystem.
//
// The shape is the one a plugin manager writes: a list of its own subcommands
// joined into an alternation, with the leading bar supplied by the `+` word so
// that an empty list leaves no bar behind.
func TestASubstitutedWordInAPatternIsNotFatalWhereAMissWouldBe(t *testing.T) {
	dir := globDir(t)
	fatal := permissive()
	fatal.GlobNoMatchIsError = Yes
	fatal.FatalErrorStatusIsOne = Yes
	grammar := func(d *syntax.Dialect) {
		d.DoubleBracket, d.PatternAlternation = true, true
		// The bar the `+` word supplies stands outside any group until the
		// pattern is assembled, and it is a live one only where the dialect
		// reads it that way — which is the reading #1929 added and the one
		// that sent this word to the filesystem in the first place.
		d.PatternTopLevelAlternation = true
	}
	src := `cmds=zzq; [[ load == (load${cmds:+|$cmds}) ]] && printf yes || printf no; printf after`
	out, st := runGrammar(t, src, grammar, func(r *Runner) { r.Dir = dir; r.Semantics = &fatal })
	if strings.Contains(out, "no matches found") {
		t.Errorf("the word was matched against the filesystem: %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("the script stopped: %q", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
