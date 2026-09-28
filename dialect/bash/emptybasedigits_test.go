// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

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
func TestAnEmptyBaseIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Refused whatever the base — 5.3's answer, and the third of this
		// shell's three numeral sentences: a digit the base cannot use is
		// `value too great for base`, a base out of range is `invalid
		// arithmetic base`, and this one is its own.
		{"8#", "bash: line 1: 8#: invalid integer constant (error token is \"8#\")\n"},
		{"16#", "bash: line 1: 16#: invalid integer constant (error token is \"16#\")\n"},
		{"8#+1", "bash: line 1: 8#: invalid integer constant (error token is \"8#\")\n"},
		// The controls, both unmoved: digits in the same spelling, and the
		// radix prefix this shell does read as zero.
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
