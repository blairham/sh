// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What `8#` — a named base with no digits after it — comes to here.
//
// One column of Semantics.ArithEmptyBaseDigits, which is a **three-valued**
// policy because the panel splits three ways rather than two. The axis's own
// comment holds the whole table and the `010#` probe that pins ksh93's
// reading; this file is this shell's row of it.
//
// Every `want` is the reference's own answer, measured 2026-09-28 from script
// files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and stdin at
// `/dev/null` (#5061).
func TestAnEmptyBaseIsZero(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Zero whatever the base, which is what parts this column from ksh93:
		// there `8#` refuses and `16#` is zero, and here both are zero.
		{"8#", "0\n"},
		{"16#", "0\n"},
		{"9#", "0\n"},
		{"010#", "0\n"},
		// The sign belongs to the digits, so this is one rather than zero.
		{"8#+1", "1\n"},
		// The controls. The same spelling with digits in it, and the radix
		// prefix ArithEmptyRadixDigitsAreZero already owns — both unmoved, so
		// a reading that had broken named bases generally would show here and
		// in none of the rows above.
		{"8#7", "7\n"},
		{"0x", "0\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, "echo $(( "+tc.src+" ))")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want {
				t.Errorf("$(( %s )) = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
