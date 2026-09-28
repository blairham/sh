// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

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
		// **Measured in `alpine:3.20`, not derived from dash.** The two are
		// neighbors on every other arithmetic question and they split here:
		// BusyBox ash has named bases and answers zero, where dash has none
		// and refuses the whole construct. TestDashHasNoNamedBaseToAskAbout
		// is the other half.
		{"8#", "0\n"},
		{"16#", "0\n"},
		{"8#+1", "1\n"},
		// The control: digits in the same spelling, unmoved.
		{"8#7", "7\n"},
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
