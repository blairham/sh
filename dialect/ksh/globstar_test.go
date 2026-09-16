// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `set -o globstar` is the walk and not a note about one.
//
// The name moved in #3128 and nothing read it, so `set -o globstar` was
// status 0, `set -o` said `globstar on`, and a script asking for `**` got the
// pattern back — the same promise the refusal was, one level quieter (#3152).
//
// Every row is printed one word per `[…]`. A table built with `echo` looks
// identical in every column of the panel, because `echo` joins with spaces
// and where the words divide is the whole question.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01. Four of the rows are bash's
// answers too and three are this shell's alone; the ones that are its alone
// say so.
func TestGlobstarBuildsTheWalk(t *testing.T) {
	const tree = "mkdir -p p/q/w r; : > topf; : > p/pf; : > p/q/qf; : > p/q/w/leaf; : > r/x; ln -s r s\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			// Off, `**` is an ordinary `*`, which is what it was under the
			// option too until this was built.
			"off it is one level", `printf "[%s]" p/**`, `[p/pf][p/q]`,
		},
		{
			// On, and this shell's own: no `[p/]` in front of them, because
			// nothing listed the directory the walk started in.
			"on it crosses and names no spelled start",
			"set -o globstar\n" + `printf "[%s]" p/**`,
			`[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// And its own again: the zero-level match is the `*` component's
			// listing, `topf` and all — a plain file no walk could have
			// stood in.
			"a listed start is reported whole",
			"set -o globstar\n" + `printf "[%s]" */**`,
			`[p][p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf][r][r/x][s][topf]`,
		},
		{
			"with a component behind it",
			"set -o globstar\n" + `printf "[%s]" **/qf`, `[p/q/qf]`,
		},
		{
			// A link is named by `**/` and never entered, which is bash's
			// answer as well.
			"a link is named and not entered",
			"set -o globstar\n" + `printf "[%s]" **/x`, `[r/x]`,
		},
		{
			// This shell's alone, and the one that is a miss rather than a
			// short answer: a pattern holding a `**` reads no directory
			// through a link, the one the walk starts in included.
			"and a walk will not begin behind one",
			"set -o globstar\n" + `printf "[%s]" s/**`, `[s/**]`,
		},
		{
			// With no `**` in the word the same link is read like any other
			// directory, with the option still on.
			"while a pattern with no crossing reads it",
			"set -o globstar\n" + `printf "[%s]" s/*`, `[s/x]`,
		},
		{
			// A run of `**` is one component rather than a cross product.
			"a run is one component",
			"set -o globstar\n" + `printf "[%s]" p/**/**`,
			`[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// The letter is the same option. It was refused by the
			// validating pass while this shell's own usage line advertised
			// it one row down.
			"the letter moves the same state",
			"set -G\n" + `printf "[%s]" p/**`,
			`[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// And it is in `$-` while it is on, which is the half a status
			// cannot report.
			"and shows in the dash parameter",
			"set -G\n" + `case "$-" in *G*) printf "[G]" ;; *) printf "[no G]" ;; esac` +
				"\nset +G\n" + `case "$-" in *G*) printf "[G]" ;; *) printf "[no G]" ;; esac`,
			`[G][no G]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tree+tc.src+"\n")
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
