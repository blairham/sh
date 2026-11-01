// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The whole-array spelling with a literal on the right, `a[@]=(p q)`, is the
// whole name here: the literal replaces the array, and `+=` appends to it. It
// was read as arithmetic and refused with `bad math expression`, and `[*]` was
// globbed against the working directory first (#6821).
//
// Measured 2026-10-16 on zsh 5.9.2 under `-c`, each row byte for byte. The
// directory holds two files so a globbed `*` would show.
func TestAWholeArraySubscriptTakesALiteralAsTheWholeName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`a=(1 2); a[@]=(p $(echo VAL)); echo st=$?; typeset -p a`, "st=0\ntypeset -a a=( p VAL )"},
		{`touch f1 f2; a=(1 2); a[*]=(p q); echo st=$?; typeset -p a`, "st=0\ntypeset -a a=( p q )"},
		{`a=(1 2); a[@]+=(p q); typeset -p a`, "typeset -a a=( 1 2 p q )"},
		{`a=(1 2); a[@]=(); typeset -p a`, "typeset -a a=(  )"},
		{`unset a; a[@]=(p q); typeset -p a`, "typeset -a a=( p q )"},
		{`a=(1 2 3); a[@]=([5]=x y); typeset -p a`, "typeset -a a=( '' '' '' '' x y )"},
		{`a=(1 2); a[@]=(p q) b=3; typeset -p a b`, "typeset -a a=( p q )\ntypeset b=3"},
		// Not over a scalar or a table, and only once the literal has run:
		// the splice's own two refusals.
		{`s=x; s[@]=(p $(echo VAL >&2)); echo after`, "VAL\nzsh:1: s: attempt to assign array value to non-array"},
		{`typeset -A h; h[k]=1; h[*]=(p $(echo VAL >&2)); echo after`, "VAL\nzsh:1: h: attempt to set slice of associative array"},
		{`readonly a=(1 2); a[@]=(p $(echo VAL >&2)); echo after`, "VAL\nzsh:1: read-only variable: a"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
