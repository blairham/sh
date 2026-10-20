// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A three-word `test` whose first word is `!` is read as a negation of the
// other two here too, ahead of the connective reading.
//
// Measured 2026-09-19 in the digest-pinned Alpine image, BusyBox 1.37.0, each
// probe a script file run by `busybox ash`. The reference column and this one
// agree on all eighteen rows swept there.
//
// **The ordering is shared with dash and the two-word readings are not**,
// which is what says this is the ordering rather than a borrowed answer:
// `[ -a x ]` is `x: unknown operand` here against dash's `-a: unexpected
// operator`, and `[ x -a ]` is `argument expected` here where dash answers 1.
// Every row below is this shell's own two-word answer with a `!` in front of
// it. bash 5.3.20 and zsh 5.9.2 read the connective first and answer 0 for
// the first two; ksh93u+ answers 0 there too, by a reading of its own (#3717).
func TestAThreeWordTestReadsItsLeadingNegationFirst(t *testing.T) {
	for _, tc := range []struct {
		src     string
		want    int
		message string
	}{
		{`[ ! -a x ]`, 2, "x: unknown operand"},
		{`[ ! -o x ]`, 2, "x: unknown operand"},
		{`[ ! -a ! ]`, 2, "!: unknown operand"},
		{`[ ! x -a ]`, 2, "argument expected"},
		{`[ ! x -o ]`, 2, "argument expected"},
		{`[ ! -a -a ]`, 2, "argument expected"},
		{`[ ! -a -o ]`, 2, "argument expected"},

		// The two-word readings those come from, which differ from the other
		// column that shares this ordering.
		{`[ -a x ]`, 2, "x: unknown operand"},
		{`[ x -a ]`, 2, "argument expected"},

		// The binary reading first, the guard unmoved, and the counts on
		// either side.
		{`[ ! = x ]`, 1, ""},
		{`[ x -a y ]`, 0, ""},
		{`[ x -a "" ]`, 1, ""},
		{`[ "" -o x ]`, 0, ""},
		{`[ ! -n x ]`, 1, ""},
		{`[ ! ! -n x ]`, 0, ""},
		{`[ ! x -a y ]`, 1, ""},

		// Four words whose last three are this shape.
		{`[ ! ! -a x ]`, 2, "x: unknown operand"},
		{`[ ! ! ! -a x ]`, 2, "x: unknown operand"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := run(t, tc.src+"\n")
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
