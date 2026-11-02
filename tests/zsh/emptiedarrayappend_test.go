// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An append to an array that held an element and was emptied starts at the
// first position. zsh cannot empty an array by `unset` of an element, so the
// routes are a slice assigned nothing and an empty literal. Measured 2026-11-02
// on zsh 5.9.2 (`-f`) under `-c`, this script, byte for byte. It is the other
// answer to interp.Semantics.AppendToAnEmptiedArrayStartsPastTheFirstPosition,
// the one ksh93 gives the opposite way (#7393).
func TestAnAppendToAnArrayEmptiedBySlicingStartsAtTheFirstPosition(t *testing.T) {
	t.Parallel()
	src := "e=(7); e[1]=(); e+=(z); typeset -p e\n" +
		"e=(7 8); e[1,2]=(); e+=(y z); typeset -p e\n" +
		"e=(7); e=(); e+=(z); typeset -p e\n"
	want := "typeset -a e=( z )\n" +
		"typeset -a e=( y z )\n" +
		"typeset -a e=( z )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}
