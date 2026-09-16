// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The braces of a `{ … }` run written immediately after `$$` are characters,
// so the pair is not a brace expansion (#3091).
//
// The lexical half is [syntax.Dialect.PidBraceGroupIsText]; this is the other
// answer the same rule carries, and the two want opposite things from the
// same byte. Measured 2026-09-15 on zsh 5.9.2, each probe in a script file of
// its own and printed one argument per `[%s]`:
//
//	printf '[%s]' $${a,b}       →  [<pid>{a,b}]        one word
//	printf '[%s]' $${a{b,c}d}   →  [<pid>{abd}][<pid>{acd}]
//
// So the *outer* pair alone is inert and a pair nested inside it still
// expands. A reading that emitted the whole run as one literal span would
// pass the first row and fail the second, which is why the second is here.
//
// ksh93u+ is the one other column that reaches a verdict of its own: it
// expands `$${a,b}` into two words. The three bash columns and dash print
// `{a,b}` as this does, by a different route — bash reads the `${` as the
// start of a parameter expansion and steps over the group, and dash has no
// brace expansion at all — so they agree with the answer without sharing the
// rule, and BusyBox ash is dash's case again.

func pidBraceGrammar(d *syntax.Dialect) {
	d.PidBraceGroupIsText = true
}

func TestThePidBracePairIsNotABraceExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the outer pair is text",
			`printf '[%s]' $${a,b}; echo`,
			"[@{a,b}]\n",
		},
		{
			"a pair nested inside it still expands",
			`printf '[%s]' $${a{b,c}d}; echo`,
			"[@{abd}][@{acd}]\n",
		},
		{
			"a blank inside leaves one word",
			`printf '[%s]' $${a b}; echo`,
			"[@{a b}]\n",
		},
		{
			"what the run holds still expands",
			`v=V; printf '[%s]' $${a${v}b}; echo`,
			"[@{aVb}]\n",
		},
		{
			// The exception, and the reason this row is here rather than in
			// a comment: the outer pair is a *list* nowhere and a range
			// wherever one is written. Measured on the shell this comes
			// from, along with `$${1..5..2}` for the step and `$${a..c}`
			// for letters; `$${a,1..3}` and `$${1..2 3}` stay text.
			"a range in the outer pair still expands",
			`printf '[%s]' $${1..3}; echo`,
			"[@1][@2][@3]\n",
		},
		{
			"a comma in the outer pair is not a range",
			`printf '[%s]' $${a,1..3}; echo`,
			"[@{a,1..3}]\n",
		},
		{
			// A run does not nest: the second `$$` opens nothing, so the
			// inner pair is the ordinary list it looks like.
			"a second `$$` inside one opens nothing",
			`printf '[%s]' $${a$${b,c}d}; echo`,
			"[@{a@bd}][@{a@cd}]\n",
		},
		{
			"the control: the same group one character along",
			`printf '[%s]' $$x{a,b}; echo`,
			"[@xa][@xb]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// `$$` is this process's own id — the runner is in-process —
			// so the expected text carries an `@` where it lands and the
			// number is put in here. What is being asserted is the braces
			// around it and how many words there are, and spelling the pid
			// into each row would make both harder to read.
			out, _ := runGrammar(t, tc.src, pidBraceGrammar, func(r *Runner) {
				// The scan resumes inside a group that did not expand,
				// which is how the pair nested in a run is reached at all.
				// The shell this is measured from answers yes; the default
				// preset leaves the axis open and refuses the line.
				sem := *r.Semantics
				sem.BraceRescanEntersFailedGroup = Yes
				r.Semantics = &sem
			})
			want := strings.ReplaceAll(tc.want, "@", strconv.Itoa(os.Getpid()))
			if out != want {
				t.Errorf("%s: %q, want %q", tc.src, out, want)
			}
		})
	}
}
