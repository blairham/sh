// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An empty field survives when it is the *inner* of a nesting, and does not
// when it is a word of the command line — #1596.
//
// The two are the same expansion written in two places, and the difference is
// who reads the fields. On the command line an unquoted empty element is no
// field at all, which is unanimous and has nothing to do with splitting. As
// the inner of a nesting it is a value the operator around it is about to
// read, and it is still there.
//
// Measured on zsh 5.9.2, 2026-09-09, with `a=(one "" two)`. The `(@)` row and
// the row without it are both needed and they show the same fact twice: with
// `(@)` the inner stays a list and the join puts a separator on either side of
// the hole; without it the inner joins on IFS first, so the hole shows as the
// doubled space between `one` and `two`. Either way the element is there.
func TestAnEmptyFieldSurvivesAsTheInnerOfANesting(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`a=(one '' two); print -r -- "[${(j:,:)${(@)${a[@]}}}]"`, "[one,,two]\n"},
		{`a=(one '' two); print -r -- "[${(j:,:)${${a[@]}}}]"`, "[one  two]\n"},
		{`a=(one '' two); print -r -- "[${(j:,:)${(@)${a[@]}/x/y}}]"`, "[one,,two]\n"},
		// And the same expansion as a *word*, which still drops it. Without
		// these rows the fix reads as "an empty field always survives", which
		// is a rule this shell measurably does not have.
		{`a=(one '' two); printf '[%s]' ${a[@]}; print`, "[one][two]\n"},
		{`a=(one '' two); printf '[%s]' ${(@)${a[@]}}; print`, "[one][two]\n"},
		// Quoted, it survives as a word too, which is the rule that was
		// already right and must stay so.
		{`a=(one '' two); printf '[%s]' "${a[@]}"; print`, "[one][][two]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The shape zi is written in, and the one that made this worth finding: a
// split, a substitution over it, and a join — three levels, with the hole
// coming from a string that opens with its own separator.
//
// `~/.zi/bin/zi.zsh` builds the alternation of ice names an annex registered
// this way, and `ZI_EXTS[ice-mods]` opens with `|` because the first
// registration appends a separator to an empty string. Dropping the hole made
// the pattern `…|xskip` where zsh has `…|x|skip` — still a *valid* pattern, so
// nothing failed and it matched the wrong things: the ice was taken for a
// plugin id and the loader tried to clone it.
func TestTheHoleSurvivesASubstitutionOverASplit(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`v='|a'; print -r -- "${#${(@)${(@s:|:)v}/a/b}}"`, "2\n"},
		{`v='|a'; print -r -- "[${(j:,:)${(@)${(@s:|:)v}/a/b}}]"`, "[,b]\n"},
		{`v='x||a'; print -r -- "[${(j:,:)${(@)${(@s:|:)v}/a/b}}]"`, "[x,,b]\n"},
		{`v='a|'; print -r -- "[${(j:,:)${(@)${(@s:|:)v}/a/b}}]"`, "[b,]\n"},
		// The line itself, reduced to one ice name.
		{`setopt extendedglob; v="|4-skip"; print -r -- "[${(j:|:)${(@)${(@Akons:|:u)v}/(#s)<->-/}}]"`, "[|skip]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
