// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// A range whose *second* endpoint is not there counts from zero.
//
// Measured on ksh93u+, 2026-09-14: `{1..}` is `1 0`, `{5..}` counts all the
// way down and `{-1..}` is `-1 0`. It is the second endpoint alone — a
// missing first endpoint or a missing step is no range at all and leaves the
// word exactly as written, which is what keeps this from being "an empty
// component is zero" and is the half bash and zsh each answer differently
// (#1691). This dialect left every one of them as written before.
func TestAMissingSecondEndpointCountsFromZero(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a missing second endpoint", `printf '[%s]' @{1..}@`, "[@1@][@0@]"},
		{"counting down to it", `printf '[%s]' {5..}`, "[5][4][3][2][1][0]"},
		{"a negative first endpoint", `printf '[%s]' {-1..}`, "[-1][0]"},
		{"already there", `printf '[%s]' {0..}`, "[0]"},
		// The padding this dialect strips is stripped here too, so the zero
		// endpoint is not a second reading of the width.
		{"leading zeros are still stripped", `printf '[%s]' {08..}`, "[8][7][6][5][4][3][2][1][0]"},
		// A step beside it is the same reading, counting from 1 to 0 by 2.
		{"with a step", `printf '[%s]' {1....2}`, "[1]"},
		// And the three shapes it does not reach.
		{"a missing first endpoint is not a range", `printf '[%s]' @{..3}@`, "[@{..3}@]"},
		{"a missing step is not a range", `printf '[%s]' @{1..2..}@`, "[@{1..2..}@]"},
		{"a letter is not a number", `printf '[%s]' @{a..}@`, "[@{a..}@]"},
		// A step of zero is a walk that never arrives, and this dialect
		// declines to read it as one where bash counts `1 2`.
		{"a step of zero is not read as one", `printf '[%s]' @{1..2..0}@`, "[@{1..2..0}@]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
