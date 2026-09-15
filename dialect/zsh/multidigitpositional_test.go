// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A run of digits after an unbraced `$` is one positional parameter here, end
// to end through this dialect.
//
// Measured 2026-09-15 with `set -- 1 2 3 4 5 6 7 8 9 ten eleven twelve` on zsh
// 5.9.2 against bash 5.3, bash 3.2, bash as `sh`, ksh93, dash and BusyBox ash:
// `$10` is `ten` here and `10` in the other six, which read `$1` and leave the
// `0` in the word. That is the grammar split `syntax.MultiDigitPositional`
// exists for, and it is silent in both directions — `10` is a perfectly
// ordinary word, so a script that means the tenth parameter and runs under the
// wrong reading prints something plausible rather than failing (#2879).
func TestAPositionalPastTheNinthNeedsNoBraces(t *testing.T) {
	const set = `set -- 1 2 3 4 5 6 7 8 9 ten eleven twelve; `
	for _, tc := range []struct{ name, src, want string }{
		{"the tenth", set + `echo "[$10]"`, "[ten]\n"},
		{"the eleventh", set + `echo "[$11]"`, "[eleven]\n"},
		{"three digits", set + `echo "[$12][$123]"`, "[twelve][]\n"},
		{"past the end is empty", set + `echo "[$99]"`, "[]\n"},
		// The run is read as a number, which is what decides the
		// leading-zero spellings rather than a rule about the digits.
		{"a leading zero", set + `echo "[$01][$09][$010]"`, "[1][9][ten]\n"},
		{"every digit zero is the shell", `echo "[$00]"`, "[sh]\n"},
		// The half the flag does not move: the run still stops at the first
		// character that is not a digit.
		{"a letter ends the run", set + `echo "[$1a]"`, "[1a]\n"},
		{"one digit is unchanged", set + `echo "[$1][$9]"`, "[1][9]\n"},
		// Falls out of this flag and BareSubscript together, with no rule of
		// its own: `$#10` is the length of what `$10` holds.
		{"a length reads the whole run", set + `echo "[$#10]"`, "[3]\n"},
		{"a subscript is still text", set + `echo "[$10[2]]"`, "[ten[2]]\n"},
		// The control. `${10}` is the tenth in every shell in the panel, so
		// the braced spelling must not be what this flag decides.
		{"the braced spelling is unchanged", set + `echo "[${10}][${11}]"`, "[ten][eleven]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
