// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An appending literal that holds any `[k]=v` element is a run of element
// stores into the array the name holds: the bare elements in front of the
// first subscripted one start at the first position and overwrite, where an
// ordinary append starts after the last element. Measured 2026-11-01 on zsh
// 5.9.2 from a file (`-f`), this script, byte for byte. The bare elements used
// to go after the last element here, so `b+=(x y [5]=z)` lost the `y` under
// the `z` (#7079).
//
// The fourth row and the last two are the controls: a literal that opens with
// a subscript, one that holds nothing but a subscript, and one with no
// subscript at all answer the same under both readings. The fifth row is a
// produced array, which is stored into the same way.
func TestAnAppendingLiteralWithASubscriptStoresFromTheFirstPosition(t *testing.T) {
	t.Parallel()
	src := "b=(7); b+=(9 [3]=y); typeset -p b\n" +
		"b=(7 8 9); b+=(x [5]=y); typeset -p b\n" +
		"b=(7 8 9); b+=(x y [5]=z); typeset -p b\n" +
		"b=(7 8 9); b+=([5]=z x); typeset -p b\n" +
		"set -- 7 8 9; argv+=(x [5]=y); print -r -- \"$# $*\"\n" +
		"b=(7 8 9 10); b+=(x [2]=y z); typeset -p b\n" +
		"b=(7 8); b+=([1]=y); typeset -p b\n" +
		"b=(7); b+=(9); typeset -p b\n"
	want := "typeset -a b=( 9 '' y )\n" +
		"typeset -a b=( x 8 9 '' y )\n" +
		"typeset -a b=( x y 9 '' z )\n" +
		"typeset -a b=( 7 8 9 '' z x )\n" +
		"5 x 8 9  y\n" +
		"typeset -a b=( x y z 10 )\n" +
		"typeset -a b=( y 8 )\n" +
		"typeset -a b=( 7 9 )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}

// It is the name already holding an array that starts the run over. Over a
// scalar the value stays first and the bare elements follow it, and an unset
// name starts at the first position whichever reading it is given. Measured
// in the same run.
func TestAnAppendingLiteralWithASubscriptOverAScalarKeepsTheValueFirst(t *testing.T) {
	t.Parallel()
	src := "s=abc; s+=(x [3]=y); typeset -p s\n" +
		"unset u; u+=(x [3]=y); typeset -p u\n"
	want := "typeset -a s=( abc x y )\n" +
		"typeset -a u=( x '' y )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}
