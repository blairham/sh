// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// **Which of the two refusals a bad name-reference target takes is decided by
// the route, not by the word.**
//
// This shell read the word: an empty operand took the builtin's ordinary
// `not a valid identifier` and every other word took the `n` letter's own
// `invalid variable name for name reference`. That agrees with bash wherever
// the two happen to line up, which is most places — fourteen target shapes and
// seven aiming routes were already right — and is wrong on the two routes where
// they do not.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree, over eleven
// words — the empty one, `12345`, `/`, `1bad`, `a b`, `-x`, `.b`, `a[`, `0`,
// `@` and `+`:
//
//	declare -n r=WORD            the `n` letter's sentence for every word
//	                             **except** the empty one
//	WORD held, then declare -n r the `n` letter's sentence for every word,
//	                             the empty one included
//	declare -n r; typeset -n r+=WORD
//	                             the builtin's sentence for every word
//
// So the empty exception belongs to the written route alone. An **adopted**
// empty drew the builtin's sentence here and should draw the letter's; every
// **appended** word but the empty one drew the letter's and should draw the
// builtin's (#4178).
func TestABadReferenceTargetTakesTheRoutesWording(t *testing.T) {
	const letter = "invalid variable name for name reference"
	const builtin = "not a valid identifier"
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref11.sub` grades: an adopted empty value.
			"an adopted empty value takes the letter's sentence",
			"r=''\ndeclare -n r", letter, builtin,
		},
		{
			// And an adopted non-empty one always did, which is the control
			// that says the route and not the word is what moved.
			"an adopted bad word takes it too",
			"r='a b'\ndeclare -n r", letter, builtin,
		},
		{
			// The written route keeps its exception: the empty word there
			// takes the builtin's sentence.
			"a written empty word takes the builtin's sentence",
			"declare -n r=''", builtin, letter,
		},
		{
			// And a written non-empty one takes the letter's.
			"a written bad word takes the letter's sentence",
			"declare -n r='a b'", letter, builtin,
		},
		{
			// The appending route takes the builtin's sentence whatever the
			// word is — this one was the letter's here.
			"an appended bad word takes the builtin's sentence",
			"declare -n r\ntypeset -n r+='12345'", builtin, letter,
		},
		{
			// A second appended word, to say it is the route rather than
			// something about digits.
			"and so does another appended word",
			"declare -n r\ntypeset -n r+='/'", builtin, letter,
		},
		{
			// The appended empty already took the builtin's sentence and
			// still does, so the change is an extension of that row rather
			// than a swap of it.
			"an appended empty word is unchanged",
			"declare -n r\ntypeset -n r+=''", builtin, letter,
		},
		{
			// A route this must not have moved: assigning through a
			// reference with nothing to point at aims it, and a bad word
			// there is the builtin's sentence in both shells.
			"assigning through an unaimed reference is unchanged",
			"declare -n r\nr='a b'", builtin, letter,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
