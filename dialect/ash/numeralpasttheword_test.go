// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// An integer numeral larger than the machine word, which this shell answers
// by letting the unsigned word go round — see
// interp.Semantics.ArithNumeralPastTheWord.
//
// Here and not in the suite because this column's reference lives in a
// container and its tier holds only what the reference on this machine can be
// held against; and here rather than only in `interp` because the assertion
// is what *this shell* answers, where the substrate's tests name the axis and
// run it at all three readings.
//
// The measured column is the pinned Alpine image's BusyBox v1.37.0, probed
// 2026-09-16 with a script file under `env -i` (#3202). Its answers are
// bash's to the byte, which is the easy thing to assume about this column and
// the thing that has been wrong seven times: dash is the other POSIX shell
// here and it clamps instead.
func TestANumeralPastTheWordGoesRoundTheUnsignedWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"at the unsigned maximum",
			`echo $((18446744073709551615))`,
			"-1",
			"paired with the row below: a reader that stopped at the largest unsigned value would answer -1 for both",
		},
		{
			"one past the unsigned maximum",
			`echo $((18446744073709551616))`,
			"0",
			"the row that says the reading is modular",
		},
		{
			"one past the signed word",
			`echo $((10000000000000000000))`,
			"-8446744073709551616",
			"the numeral the issue was filed on; dash answers 9223372036854775807 here",
		},
		{
			"several words past",
			`echo $((99999999999999999999999))`,
			"200376420520689663",
			"the accumulator keeps going round rather than stopping at the first wrap",
		},
		{
			"hexadecimal",
			`echo $((0xffffffffffffffffff))`,
			"-1",
			"the base does not change the reading",
		},
		{
			"the same digits out of a variable",
			`n=10000000000000000000; echo $((n))`,
			"-8446744073709551616",
			"dash is the only column whose two number readers part, and this one's do not",
		},
		{
			"and the numeral is still an operand",
			`echo $((10000000000000000000 - 1))`,
			"-8446744073709551617",
			"the wrapped value goes on into the expression rather than being a special case of its own",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0 — %s", tc.src, out, st, tc.want, tc.why)
			}
		})
	}
}
