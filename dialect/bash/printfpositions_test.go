// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

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

// TestPrintfIntoAnArrayIsOneElement is interp.Semantics.PrintfVTakesAnElementPerPass
// in bash: a format reused into an array leaves the whole text in element 0.
// Measured 2026-10-02 on bash 5.3.20: `declare -a foo; printf -v foo '%s' a b
// c; declare -p foo` is `declare -a foo=([0]="abc")`.
func TestPrintfIntoAnArrayIsOneElement(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"declare -a foo; printf -v foo '%s' a b c; declare -p foo\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "declare -a foo=([0]=\"abc\")\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
