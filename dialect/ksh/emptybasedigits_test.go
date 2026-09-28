// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

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
//
// **The location is the harness's and the sentence is the reference's.** The
// helper invokes the shell by its base name where the measurement above used
// a path, so the reference writes `/opt/homebrew/bin/bash: line 1: …` and
// this writes `bash: line 1: …`. Everything after the location is compared
// byte for byte, which is the part the axis is about.
func TestAnEmptyBaseIsZeroAtTenOrAbove(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Below ten, refused.
		{"8#", "ksh:  8# : arithmetic syntax error\n"},
		{"9#", "ksh:  9# : arithmetic syntax error\n"},
		// At ten and above, zero. Those four rows alone read as a *digit
		// count*, which is the wrong rule.
		{"10#", "0\n"},
		{"16#", "0\n"},
		// **The probe that separates the two readings.** `010#` is base ten
		// written with a leading zero and it refuses, while `10#` above
		// succeeds — so the base itself is read as a C integer constant, in
		// which `010` is eight. Without this row the column could be written
		// as "a one-digit base refuses" and every other row would agree.
		{"010#", "ksh:  010# : arithmetic syntax error\n"},
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
