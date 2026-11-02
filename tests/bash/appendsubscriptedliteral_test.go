// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An appending literal's bare elements go after the last element even where
// the literal holds a subscripted one, so a subscript behind them overwrites
// what the literal itself appended rather than what the array held. Measured
// 2026-11-01 on bash 5.3.20 from a file, this script, byte for byte; bash
// 3.2.57 places the bare elements the same way. It is the other answer to
// interp.Semantics.AppendingSubscriptedLiteralStoresFromTheFirstPosition, the
// one zsh gives the opposite way (#7079).
func TestAnAppendingLiteralWithASubscriptStillAppends(t *testing.T) {
	t.Parallel()
	src := "b=(7); b+=(9 [2]=y); declare -p b\n" +
		"b=(7 8 9); b+=(x [4]=y); declare -p b\n" +
		"b=(7 8 9); b+=(x y [4]=z); declare -p b\n" +
		"b=(7 8 9); b+=([4]=z x); declare -p b\n"
	want := `declare -a b=([0]="7" [1]="9" [2]="y")` + "\n" +
		`declare -a b=([0]="7" [1]="8" [2]="9" [3]="x" [4]="y")` + "\n" +
		`declare -a b=([0]="7" [1]="8" [2]="9" [3]="x" [4]="z")` + "\n" +
		`declare -a b=([0]="7" [1]="8" [2]="9" [4]="z" [5]="x")` + "\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}
