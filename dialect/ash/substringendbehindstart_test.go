// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// A negative substring length whose end falls behind the offset hands back
// everything from the offset, in silence and at 0.
//
// This is the third answer on the axis and neither of the other two: bash and
// zsh each refuse it, in different words and at different costs, and an empty
// result — the obvious reading of "not refused" — is what this shell gives
// only when the end lands exactly at the start. Measured 2026-09-27 on
// BusyBox v1.37.0 in the pinned image (#4832).
func TestAnEndBehindTheOffsetIsTheRest(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abcdef; echo "[${v:1:-9}]"`, "[bcdef]"},
		{`v=abcdef; echo "[${v:2:-10}]"`, "[cdef]"},
		{`v=abcdef; echo "[${v:0:-7}]"`, "[abcdef]"},
		{`v=abcdef; echo "[${v:4:-3}]"`, "[ef]"},
	} {
		out, st := run(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// The two controls, and they are what keep the rows above from reading as
	// "a negative length is ignored here": an end exactly at the start is
	// empty, and one inside the value is the unanimous answer.
	for _, tc := range []struct{ src, want string }{
		{`v=abcdef; echo "[${v:0:-6}]"`, "[]"},
		{`v=abcdef; echo "[${v:1:-2}]"`, "[bcd]"},
	} {
		out, st := run(t, tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The axis.
func TestTheEndBehindTheStartAxis(t *testing.T) {
	if got := ash.Semantics().SubstringEndBehindTheStart; got != interp.SubstringEndBehindStartIsTheRest {
		t.Errorf("SubstringEndBehindTheStart = %v, want the rest of the value", got)
	}
}
