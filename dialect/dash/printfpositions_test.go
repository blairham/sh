// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestPrintfRefusesArgumentPositions is interp.Semantics.PrintfArgumentPositions
// in a column that has no positions — or, for ksh93, whose own reading is not
// modeled and which holds the refusal meanwhile. Measured 2026-10-02 against
// the shell this dialect claims: `printf '%2$s %1$s\n' a b` is a refusal at
// a non-zero status, where zsh 5.9.2 writes `b a`.
func TestPrintfRefusesArgumentPositions(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, "printf '%2$s %1$s\\n' a b\n")
	if err != nil {
		t.Fatal(err)
	}
	if st == 0 || strings.Contains(out, "b a") {
		t.Errorf("status %d, output %q: want a refusal", st, out)
	}
}
