// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// TestACondtionThatNeverBeganAndRanOut — the five sites with the input ending
// there, which is the shape this dialect is exact about.
//
// ksh93 calls the `[[` unmatched for all of them, the same line it writes for
// a condition that *was* read and never closed — measured 2026-09-15 on
// ksh93u+ over `-c`.
//
// The neighbouring shape, a `]]` standing where a condition could have begun,
// is **not** this: that shell reads the closer as an ordinary word there and
// blames whatever follows, which is #2964 and a change to what the parser
// consumes. These rows are the ones a wording can answer.
func TestACondtionThatNeverBeganAndRanOut(t *testing.T) {
	for _, src := range []string{"[[ ", "[[ !", "[[ a &&", "[[ a ||", "[[ ("} {
		t.Run(src, func(t *testing.T) {
			_, err := syntax.Parse(src, ksh.Dialect())
			if err == nil {
				t.Fatalf("%s: parsed, want a refusal", src)
			}
			got := ksh.Diagnostics().ParseDiagnostic("ksh", "-c", err, src)
			if want := "ksh: syntax error at line 1: `[[' unmatched\n"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
