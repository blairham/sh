// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What OPTIND says while the scan is still inside a clustered word (#2291).
//
// `-abc` is three options in one word, and the two answers differ on every
// letter but the last: bash, ksh93 and zsh leave OPTIND naming the word until
// its last letter has been read, and dash and BusyBox ash count it at the
// first. The final value is the same either way, which is why this needed a
// case that reads OPTIND *between* two calls — a loop that only prints the
// number at the end cannot tell the two rules apart, and the one in
// TestGetoptsTheUnanimousParts does not.
//
// Both directions and the refusal, because an axis only ever taken cannot be
// told from one that is always taken.
func TestGetoptsClusterOptindAxis(t *testing.T) {
	// Every letter and the number that was current when it was read, so a
	// wrong answer at any position is visible rather than only at the end.
	const src = `set -- -abc rest
while getopts 'abc' o; do printf '[%s:%s]' "$o" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`

	for _, tc := range []struct {
		name    string
		answer  Answer
		want    string
		refused string
	}{
		{
			// bash 5.3.20, bash 3.2.57, ksh93u+ 2012-08-01 and zsh 5.9.2.
			name:   "the word is counted at its last letter",
			answer: No, want: "[a:1][b:1][c:2] end=2\n",
		},
		{
			// dash 0.5.12 and BusyBox ash 1.37.0. Measured 2026-09-16 over a
			// script file with `env -i PATH=/usr/bin:/bin`, and Apple's
			// dash-16 answers the same, so it is the shell and not the build.
			name:   "the word is counted at its first letter",
			answer: Yes, want: "[a:2][b:2][c:2] end=2\n",
		},
		{
			name: "unanswered", answer: Unspecified,
			// The refusal ends the read, so nothing is printed for the
			// letters; the line the loop writes afterwards still arrives.
			want:    " end=1\n",
			refused: "OPTIND counting a clustered word at its first letter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsCountsTheWordAtItsFirstLetter = tc.answer
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				if !strings.Contains(out, tc.refused) {
					t.Fatalf("got %q, want it to carry %q", out, tc.refused)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
