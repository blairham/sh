// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A value an integer-attributed name will not take is quoted back, the way
// every other arithmetic failure in this shell is (#2420).
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01: `typeset -i b; b=0x` is `0x:
// arithmetic syntax error`, where ours wrote the reason with nothing named at
// all — the assignment route reported the raw error instead of going through
// the wording every other expression goes through.
//
// The blamed text carries no surrounding blanks, because the assignment's
// value has none. The same shell's `$(( 0x ))` keeps them, from
// the same wording and a different text, which is what says this is the
// expression as written rather than a trimmed form of it.
func TestARefusedIntegerValueIsQuotedBack(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Name: "ksh", Dir: t.TempDir()},
		"typeset -i b; b=0x; echo \"[$b]\"\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "0x: arithmetic syntax error"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	if strings.Contains(out, "[") {
		t.Errorf("got %q, want the script to have ended before the echo", out)
	}
}
