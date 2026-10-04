// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An offset that expanded to nothing, ahead of a length, in the dialect whose
// offset expression takes a leading colon: the range is then the expression
// `:2`, exactly as the written `${x::2}` is. Measured 2026-10-03 on ksh93u+.
// Without a length behind it the empty offset is zero, and the reading
// without the flag is zero in both places.
func TestABlankSubstringOffsetBeforeALength(t *testing.T) {
	colon := func(d *syntax.Dialect) { d.ParamSubstringOffsetTakesALeadingColon = true }
	plain := func(d *syntax.Dialect) { d.ParamSubstringOffsetTakesALeadingColon = false }

	out, st := runGrammar(t, `x=abcdef; w=; echo "[${x:$w:2}]"; echo after`, colon, nil)
	if !strings.Contains(out, ":2") || strings.Contains(out, "after") || st == 0 {
		t.Errorf("with the flag: got %q status %d, want `:2` refused and the script ended", out, st)
	}
	if out, st := runGrammar(t, `x=abcdef; w=; echo "[${x:$w}][${x:1:$w}]"`, colon, nil); out != "[abcdef][]\n" || st != 0 {
		t.Errorf("no length: got %q status %d, want [abcdef][] at 0", out, st)
	}
	if out, st := runGrammar(t, `x=abcdef; w=; echo "[${x:$w:2}]"`, plain, nil); out != "[ab]\n" || st != 0 {
		t.Errorf("without the flag: got %q status %d, want [ab] at 0", out, st)
	}
}
