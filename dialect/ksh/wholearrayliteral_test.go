// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The whole-array spelling with a literal or a compound body on the right is
// refused for its subscript, before anything in the list is expanded, and the
// input ends — over an array, a table, a scalar and an unset name alike. It
// was read as arithmetic (`@: arithmetic syntax error`), `[*]` was globbed
// against the working directory first, and over a table `@` was stored as a
// key (#6821).
//
// Measured 2026-10-16 on ksh93u+ 2012-08-01 under `-c`, each row byte for
// byte apart from the shell's name. The directory holds two files so a
// globbed `*` would show.
func TestAWholeArraySubscriptRefusesALiteral(t *testing.T) {
	for _, src := range []string{
		`a=(1 2); a[@]=(p $(echo VAL >&2)); echo after`,
		`touch f1 f2; a=(1 2); a[*]=(p q); echo after`,
		`a=(1 2); a[@]+=(p q); echo after`,
		`unset a; a[@]=(p q); echo after`,
		`s=x; s[@]=(p q); echo after`,
		`typeset -A h; h[k]=1; h[@]=(p $(echo VAL >&2)); echo after`,
		`typeset -A h; h[k]=1; readonly h; h[@]=(p $(echo VAL >&2)); echo after`,
		`a=(1 2); a[@]=(); echo after`,
		`a=(1 2); a[@]=(x=$(echo VAL >&2)); echo after`,
		`typeset -A h; h[@]=(); echo after`,
	} {
		out, _ := answersRun(t, src)
		sub := "@"
		if strings.Contains(src, "[*]") {
			sub = "*"
		}
		if want := "sh: " + sub + ": invalid subscript in assignment"; strings.TrimSpace(out) != want {
			t.Errorf("%s\n got %q\nwant %q", src, strings.TrimSpace(out), want)
		}
	}
}
