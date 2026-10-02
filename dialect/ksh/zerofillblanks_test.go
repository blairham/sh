// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAZeroFillTakesTheLeadingBlanksOff is ksh93's answer to
// interp.Semantics.ZeroFillKeepsLeadingBlanks and
// interp.Semantics.ZeroFillGoesAfterAnIntegersSign: the blanks come off
// before the fill, and a sign is blank-filled however the name is typed.
// Measured 2026-10-02 on ksh93u+ 2012-08-01.
func TestAZeroFillTakesTheLeadingBlanksOff(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`typeset -Z 6 z; for v in "  4" " 4" " -4"; do z=$v; print -r -- "[$z]"; done
typeset -i n=-4; typeset -Z6 n; print -r -- "[$n]"
`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[000004]\n[000004]\n[    -4]\n[    -4]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
