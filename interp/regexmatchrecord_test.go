// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `=~` leaves behind in the record a dialect names (#2916).
//
// Two axes, moved across each other, because a record that is only ever
// written one way cannot be told from one nobody consults: the rows that
// answer differently are what say each is read. The subject and the
// expressions are the same throughout, so what moves is the answer and not
// the match.
func TestWhatARegexMatchLeavesInTheRecord(t *testing.T) {
	for _, tc := range []struct {
		name     string
		survives Answer
		omits    Answer
		src      string
		want     string
	}{
		{
			name: "a dense record", survives: No, omits: No,
			src: `[[ abcd =~ (b)(z)?(c) ]]`, want: "4:bc b  c",
		},
		{
			name: "a group that took no part left out", survives: No, omits: Yes,
			src: `[[ abcd =~ (b)(z)?(c) ]]`, want: "3:bc b c",
		},
		{
			// The gap is closed wherever it falls, so this is not a trailing
			// group being dropped: the third group is read back as the
			// second.
			name: "a trailing one, dense", survives: No, omits: No,
			src: `[[ abcd =~ b(z)?c ]]`, want: "2:bc ",
		},
		{
			name: "a trailing one, left out", survives: No, omits: Yes,
			src: `[[ abcd =~ b(z)?c ]]`, want: "1:bc",
		},
		{
			// A group that matched the *empty string* took part, and stays
			// under either answer — which is what says the axis is about
			// participation and not about emptiness.
			name: "an empty group that took part, dense", survives: No, omits: No,
			src: `[[ abcd =~ (b)(z*)(c) ]]`, want: "4:bc b  c",
		},
		{
			name: "an empty group that took part, left out", survives: No, omits: Yes,
			src: `[[ abcd =~ (b)(z*)(c) ]]`, want: "4:bc b  c",
		},
		{
			name: "a failed match empties it", survives: No, omits: No,
			src: `[[ abcd =~ (b)(c) ]]; [[ abcd =~ (x)(y) ]]`, want: "0:",
		},
		{
			name: "a failed match leaves it", survives: Yes, omits: No,
			src: `[[ abcd =~ (b)(c) ]]; [[ abcd =~ (x)(y) ]]`, want: "3:bc b c",
		},
		{
			// The control for that pair: with nothing recorded yet, both
			// answers leave an empty record behind.
			name: "a failed match with nothing before it, empties", survives: No, omits: No,
			src: `[[ abcd =~ (x)(y) ]]`, want: "0:",
		},
		{
			name: "a failed match with nothing before it, leaves", survives: Yes, omits: No,
			src: `[[ abcd =~ (x)(y) ]]`, want: "0:",
		},
		{
			// A negated match still records, which the record has always
			// done: it is the evaluation that writes, before `!` sees the
			// result.
			name: "a negated match still records", survives: Yes, omits: Yes,
			src: `[[ ! abcd =~ (b)(c) ]]`, want: "3:bc b c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.RegexMatchSurvivesAFailedMatch = tc.survives
			sem.RegexMatchOmitsGroupsThatDidNotMatch = tc.omits
			src := tc.src + `; printf '%s:%s' "${#M[@]}" "${M[*]}"`
			out, _ := run(t, src, func(r *Runner) {
				r.Semantics = &sem
				r.SetRegexMatch("M")
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// With no name from the dialect there is no record, so neither axis is
// reached — which is what lets a shell that keeps its captures under another
// shape leave both unanswered.
func TestNoRecordNameAsksNeitherAxis(t *testing.T) {
	sem := permissive()
	sem.RegexMatchSurvivesAFailedMatch = Unspecified
	sem.RegexMatchOmitsGroupsThatDidNotMatch = Unspecified
	out, st := run(t, `[[ abcd =~ (b)(c) ]]; echo "st=$?"`, func(r *Runner) { r.Semantics = &sem })
	if st != 0 || out != "st=0\n" {
		t.Errorf("out %q status %d, want the match answered and nothing asked", out, st)
	}
	// And with a name, an unanswered axis refuses by name rather than
	// guessing at the shape of the record.
	out, st = run(t, `[[ abcd =~ (b)(c) ]]`, func(r *Runner) {
		r.Semantics = &sem
		r.SetRegexMatch("M")
	})
	if st != 2 || out == "" {
		t.Errorf("out %q status %d, want the refusal by name at 2", out, st)
	}
}
