// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A negative length whose end falls behind an offset at or past the end of
// the list is still refused: there is nothing to slice, and the length is read
// and held to the offset all the same. Measured 2026-10-03 on zsh 5.9.2.
func TestANegativeLengthPastTheEndIsStillRefused(t *testing.T) {
	for _, c := range []struct{ src, err string }{
		{`a=(ax bx cx); print -r -- "[${a[@]:3:-1}]"; print after`, "substring expression: 2 < 3"},
		{`b=(); print -r -- "[${b[@]:0:-1}]"; print after`, "substring expression: -1 < 0"},
		{`set -- a b c; print -r -- "[${@:4:-1}]"; print after`, "substring expression: 2 < 3"},
	} {
		out, _ := runZshOnPath(t, t.TempDir(), c.src)
		if !strings.Contains(out, c.err) || strings.Contains(out, "after") {
			t.Errorf("%s: %q, want %q and nothing after it", c.src, out, c.err)
		}
	}
	// The control: an offset past the end with a positive length, and a negative one ending exactly at the offset, are empty.
	if out, st := runZshOnPath(t, t.TempDir(), `a=(ax bx cx); print -r -- "[${a[@]:3:1}][${a[@]:2:-1}]"`); out != "[][]\n" || st != 0 {
		t.Errorf("control: %q at %d", out, st)
	}
}
