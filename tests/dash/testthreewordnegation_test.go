// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A three-word `test` whose first word is `!` is read as a negation of the
// other two, ahead of the reading that takes the middle word as a connective.
//
// Measured 2026-09-19 on dash 0.5.12 at /bin/dash, each probe a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on
// /dev/null. The reference column and this one agree on every row here, and
// on a sweep of all 1728 three-word and 3000 four-word shapes over a twelve
// word alphabet — 22 rows of the first and 18 of the second moved onto the
// reference with this reading and none moved off it.
//
// bash 5.3.20 and zsh 5.9.2 read the connective first and answer 0 for the
// first two rows; ksh93u+ answers 0 there too, reaching it through a reading
// of its own rather than through this ordering. BusyBox ash 1.37.0 reads the
// negation first as this column does, in its own wording (#3717).
func TestAThreeWordTestReadsItsLeadingNegationFirst(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
		// message is what the refusal has to say, empty where the row is an
		// answer rather than a refusal.
		message string
	}{
		// The shape. Each is the two-word reading of the last two words with
		// a `!` in front of it, and the two-word reading is where the
		// sentence comes from: `[ -a x ]` is `-a: unexpected operator` here
		// on its own.
		{`[ ! -a x ]`, 2, "-a: unexpected operator"},
		{`[ ! -o x ]`, 2, "-o: unexpected operator"},
		{`[ ! -a "" ]`, 2, "-a: unexpected operator"},
		{`[ ! -o "" ]`, 2, "-o: unexpected operator"},
		{`[ ! -a -n ]`, 2, "-a: unexpected operator"},
		{`[ ! -a ! ]`, 2, "-a: unexpected operator"},
		// And the rows where the two-word reading answers rather than
		// refuses, which is this shell's trailing-connective reading:
		// `[ x -a ]` is 1 and `[ x -o ]` is 0, so these are those negated.
		{`[ ! x -a ]`, 0, ""},
		{`[ ! x -o ]`, 1, ""},
		{`[ ! -a -a ]`, 0, ""},
		{`[ ! -a -o ]`, 1, ""},
		{`[ ! -o -a ]`, 0, ""},
		{`[ ! -o -o ]`, 1, ""},

		// The two-word readings those come from, which is where each status
		// and each sentence above is decided.
		{`[ -a x ]`, 2, "-a: unexpected operator"},
		{`[ x -a ]`, 1, ""},
		{`[ x -o ]`, 0, ""},
		{`[ -a -a ]`, 1, ""},
		{`[ -a -o ]`, 0, ""},

		// The binary reading still comes first, which is what keeps this
		// from being a rule about a leading `!`.
		{`[ ! = x ]`, 1, ""},
		{`[ ! = ! ]`, 0, ""},
		{`[ ! != x ]`, 0, ""},
		{`[ ! -eq x ]`, 2, "Illegal number: !"},

		// The guard a script actually writes, unmoved.
		{`[ x -a y ]`, 0, ""},
		{`[ x -a "" ]`, 1, ""},
		{`[ "" -o x ]`, 0, ""},
		{`[ x -a ! ]`, 0, ""},

		// A leading `!` with no connective in the middle, and the counts on
		// either side.
		{`[ ! -n x ]`, 1, ""},
		{`[ ! -z x ]`, 0, ""},
		{`[ ! x ]`, 1, ""},
		{`[ ! "" ]`, 0, ""},
		{`[ ! x -a y ]`, 1, ""},
		{`[ ! "" -o y ]`, 1, ""},

		// Four words whose last three are this shape: the fold takes the
		// three-word reading of the rest, which is now the refusal.
		{`[ ! ! -a x ]`, 2, "-a: unexpected operator"},
		{`[ ! ! -o x ]`, 2, "-o: unexpected operator"},
		{`[ ! ! ! -a x ]`, 2, "-a: unexpected operator"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), tc.src+"\n")
			if st != tc.want {
				t.Errorf("%s = %d, want %d (output %q)", tc.src, st, tc.want, out)
			}
			if tc.message == "" {
				if out != "" {
					t.Errorf("%s said %q, want nothing", tc.src, out)
				}
				return
			}
			if !strings.Contains(out, tc.message) {
				t.Errorf("%s said %q, want %q in it", tc.src, out, tc.message)
			}
		})
	}
}
