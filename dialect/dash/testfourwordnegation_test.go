// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A four-word `test` led by `!` takes the three-word reading of the rest and
// does not negate it again, where the rest is itself a negation.
//
// Measured 2026-09-18 and 2026-09-19 on dash 0.5.12 at /bin/dash, each probe a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on /dev/null. The reference column and this one agree on all of these, and
// on 34 probes around them.
//
// Both readings exit without output, so the status is the whole answer.
//
// The `!`-behind-a-negation rows are the ones this shell is alone on: bash
// 5.3.20, zsh 5.9.2, ksh93u+ and BusyBox ash 1.37.0 all answer the other way,
// measured the same day (#3700).
func TestAFourWordTestBehindANegationNegatesOnce(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		// Four words whose last three are a negation: the answer is what
		// those three words give on their own.
		{`[ ! ! -n x ]`, 1},
		{`[ ! ! -n "" ]`, 0},
		{`[ ! ! -z x ]`, 0},
		{`[ ! ! -z "" ]`, 1},
		{`[ ! ! -d / ]`, 1},
		{`[ ! ! -e /nope ]`, 0},
		{`[ ! ! -n -n ]`, 1},
		{`[ ! ! ! x ]`, 0},
		{`[ ! ! ! "" ]`, 1},
		{`[ ! ! \( \) ]`, 0},

		// The three-word readings those fold onto, which is where each
		// status above comes from.
		{`[ ! -n x ]`, 1},
		{`[ ! -z "" ]`, 1},
		{`[ ! -d / ]`, 1},
		{`[ ! \( \) ]`, 0},

		// Four words whose last three are **not** a negation: the `!`
		// negates, exactly as a recursive reading says.
		{`[ ! x = x ]`, 1},
		{`[ ! x = y ]`, 0},
		{`[ ! x -a y ]`, 1},
		{`[ ! "" -o y ]`, 1},
		{`[ ! \( x \) ]`, 1},
		{`[ ! \( "" \) ]`, 0},
		// The sharpest control: two leading `!`s whose remainder is a
		// string comparison, because `=` takes the first `!` as its left
		// operand. A "two leading `!`s cancel at four words" rule gets
		// this one wrong, and the reference says 0.
		{`[ ! ! = x ]`, 0},
		{`[ ! = x ]`, 1},

		// The counts on either side, which negate as a recursive reading
		// predicts here too.
		{`[ ! ! x ]`, 0},
		{`[ ! ! "" ]`, 1},
		{`[ ! ! x = x ]`, 0},
		{`[ ! ! ! -n x ]`, 1},
		{`[ ! ! ! \( \) ]`, 0},
		{`[ ! ! ! ! -n x ]`, 0},
		{`[ ! x ]`, 1},
		{`[ ! "" ]`, 0},
		{`[ ! ! ]`, 1},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), tc.src+"\n")
			if st != tc.want {
				t.Errorf("%s = %d, want %d (output %q)", tc.src, st, tc.want, out)
			}
			if out != "" {
				t.Errorf("%s said %q, want nothing", tc.src, out)
			}
		})
	}
}
