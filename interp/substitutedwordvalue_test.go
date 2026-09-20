// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// substitutedValues is the grammar these expansions need, named by the
// constructs rather than by a shell.
func substitutedValues(d *syntax.Dialect) {
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// rejoinsOnASpace answers UnsplitAtListJoinsOnIFS with No: a list this engine
// has taken the fields of, and then decided not to keep them, is rejoined
// with a space rather than with IFS.
//
// Stated rather than inherited, because it is the axis these rows depend on
// and the default answer hides what they are for — see the test below.
func rejoinsOnASpace(r *interp.Runner) {
	r.Semantics.UnsplitAtListJoinsOnIFS = interp.No
}

// The word a substitution substitutes is a **value** wherever nothing is
// going to be split, so a list inside it joins the way the list itself joins
// and not with a space put in afterwards.
//
// Measured 2026-09-20 from script files under `env -i`, with
// `set -- 'a:b' c` and `IFS=:`, on bash 5.3.20, bash 3.2.57, ksh93u+,
// zsh 5.9.2 and dash 0.5.12. All five store `a:b:c` for `${u-$*}` and for
// `${u=$*}`; this shell stored `a b c`, because the word was expanded into
// fields and those were rejoined under the *outer* node — which is not the
// `*`, so it rejoined with a space.
//
// Only `*` is here. What `$@` and `${a[@]}` rejoin as in the same position is
// UnsplitAtListJoinsOnIFS, an axis the panel splits on, so it is asked where a
// dialect can answer it — see the bash preset's own case.
//
// **That axis is set here, and the rows about the default word cannot see
// this fault without it.** Answered Yes, the rejoin these rows are about uses
// IFS anyway and every one of them passes with the fix taken out — a pin, not
// a test. rejoinsOnASpace is the answer two of the panel's columns give, and
// it is what makes the space this fault left visible.
func TestAStarInASubstitutedWordJoinsOnIFSWhereNothingIsSplit(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the default word",
			`set -- 'a:b' c; IFS=:; v=${u-$*}; printf "[%s]" "$v"`,
			"[a:b:c]",
		},
		{
			"the colon default word",
			`set -- 'a:b' c; IFS=:; v=${u:-$*}; printf "[%s]" "$v"`,
			"[a:b:c]",
		},
		{
			// The assigning form twice over: what it substitutes and what it
			// left behind are the same value, and both were wrong.
			"the assigning word, and what it stored",
			`set -- 'a:b' c; IFS=:; v=${u=$*}; printf "[%s][%s]" "$v" "$u"`,
			"[a:b:c][a:b:c]",
		},
		{
			"the colon assigning word",
			`set -- 'a:b' c; IFS=:; v=${u:=$*}; printf "[%s][%s]" "$v" "$u"`,
			"[a:b:c][a:b:c]",
		},
		{
			"a star subscript in the word",
			`a=('p:q' r); IFS=:; v=${u-${a[*]}}; printf "[%s]" "$v"`,
			"[p:q:r]",
		},
		{
			// A separator that is not the default is what makes the rows
			// above readable; this one says the join is IFS's first
			// character and not the whole of it.
			"the first character of IFS and not the rest",
			`set -- x y; IFS=:-; v=${u=$*}; printf "[%s]" "$v"`,
			"[x:y]",
		},
		{
			// An IFS that is set and empty joins with nothing, which is the
			// same rule read at its edge.
			"an empty IFS joins with nothing",
			`set -- x y; IFS=; v=${u=$*}; printf "[%s]" "$v"`,
			"[xy]",
		},
		// Controls. Each of these was already right and must stay right: the
		// quoted spelling reaches the join through the quotes rather than
		// through this path, a word with no list in it is untouched, and an
		// ordinary right-hand side never had the fault.
		{
			"quoted, which took the other route",
			`set -- 'a:b' c; IFS=:; v="${u-$*}"; printf "[%s]" "$v"`,
			"[a:b:c]",
		},
		{
			"a plain word is unchanged",
			`IFS=:; v=${u-x y}; printf "[%s]" "$v"`,
			"[x y]",
		},
		{
			"an ordinary right-hand side",
			`set -- 'a:b' c; IFS=:; v=$*; printf "[%s]" "$v"`,
			"[a:b:c]",
		},
		{
			// The value is joined, and the *result* is then split by
			// whatever context it lands in. Two stages, and this row is the
			// only one that can tell them apart: three fields come out of
			// one joined string, not out of the two the list started with.
			"the joined value is then split where it is used",
			`set -- 'a:b' c; IFS=:; printf "[%s]" ${u=$*}`,
			"[a][b][c]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, substitutedValues, rejoinsOnASpace)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
