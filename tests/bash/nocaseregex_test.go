// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `nocasematch` reaches the regular expression operator as well as the
// pattern one, and it reaches the *whole* expression.
//
// The option was honored in one of the two places bash applies it: `==` folded
// and `=~` did not, so `shopt -s nocasematch; [[ $reply =~ ^y ]]` took the
// other branch with nothing written to standard error (#2622).
//
// Measured 2026-09-13 on bash 5.3.15, and the same answers on bash-as-`sh`
// and on bash 3.2.57. Written as one table so the reach and its edges cannot
// drift apart: a fix that folded only the pattern's literal letters passes
// the first two rows and fails the four bracket ones.
func TestNocasematchFoldsTheRegexOperatorAndTheWholeExpression(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a lower-case expression against an upper-case subject",
			`shopt -s nocasematch; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "yes",
		},
		{
			"and in the other direction",
			`shopt -s nocasematch; [[ abc =~ ^ABC$ ]] && echo yes || echo no`, "yes",
		},

		// The four that say the fold is the compiled expression's and not a
		// pass over its literal characters.
		{
			"a range folds",
			`shopt -s nocasematch; [[ ABC =~ ^[a-c]+$ ]] && echo yes || echo no`, "yes",
		},
		{
			"an enumeration folds",
			`shopt -s nocasematch; [[ ABC =~ ^[abc]+$ ]] && echo yes || echo no`, "yes",
		},
		{
			"a character class folds",
			`shopt -s nocasematch; [[ ABC =~ ^[[:lower:]]+$ ]] && echo yes || echo no`, "yes",
		},
		{
			"and the class of the other case folds with it",
			`shopt -s nocasematch; [[ abc =~ ^[[:upper:]]+$ ]] && echo yes || echo no`, "yes",
		},

		// The fold happens *before* the class is complemented, which a fold
		// bolted on after the match cannot get right: `[^a]` excludes `A` as
		// well, so an upper-case subject fails and an unrelated letter passes.
		{
			"the fold precedes a negation",
			`shopt -s nocasematch; [[ A =~ ^[^a]$ ]] && echo yes || echo no`, "no",
		},
		{
			"which still leaves the rest of the alphabet in",
			`shopt -s nocasematch; [[ b =~ ^[^a]$ ]] && echo yes || echo no`, "yes",
		},

		// The captures are spans of the subject as the script wrote it. A fold
		// that lower-cased the subject before matching passes every row above
		// and answers `abc/a/b` here.
		{
			"the captures keep the subject's own case",
			`shopt -s nocasematch; [[ ABC =~ ^(a)(B)c$ ]]; echo ${BASH_REMATCH[0]}/${BASH_REMATCH[1]}/${BASH_REMATCH[2]}`, "ABC/A/B",
		},

		// The option is what does it, in both directions, and the *other*
		// case option is not it. bash 3.2 differs on the last row and is not
		// the dialect this file grades — see docs/spec/measurements.md.
		{
			"and the option is what decides",
			`[[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},
		{
			"turned back off again",
			`shopt -s nocasematch; shopt -u nocasematch; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},
		{
			"and nocaseglob is not that option",
			`shopt -s nocaseglob; [[ ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},

		// `!~` is not an operator, so the negation a script reaches for is
		// `!`, and it follows the fold rather than escaping it.
		{
			"a negated condition follows the fold",
			`shopt -s nocasematch; [[ ! ABC =~ ^abc$ ]] && echo yes || echo no`, "no",
		},

		// The option leaves an expression that was already going to match
		// alone, so turning it on can only add matches.
		{
			"an exact expression still matches",
			`shopt -s nocasematch; [[ ABC =~ ^ABC$ ]] && echo yes || echo no`, "yes",
		},

		// A quoted right operand is a literal string in this dialect, and the
		// fold applies to the literal it became rather than being skipped
		// with the regular expression reading.
		{
			"a quoted operand is literal and still folds",
			`shopt -s nocasematch; [[ 'A.C' =~ "a.c" ]] && echo yes || echo no`, "yes",
		},
		{
			"and is still literal",
			`shopt -s nocasematch; [[ AXC =~ "a.c" ]] && echo yes || echo no`, "no",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if got := strings.TrimRight(out, "\n"); got != tc.want {
				t.Errorf("%s = %q status %d, want %q", tc.src, got, st, tc.want)
			}
		})
	}
}
