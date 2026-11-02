// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A negative subscript in a literal that reads its subscripts counts back from
// the end here, which is
// interp.Semantics.NegativeSubscriptInAnIndexedLiteralIsBad answered No. The
// mixed literals zsh refuses or places differently keep their `[k]=v` as text
// here instead, so neither question is reached by them (#7321, #7322).
// Measured 2026-11-02 on ksh93u+ 2012-08-01 from a file, this script, byte for
// byte.
func TestANegativeLiteralSubscriptCountsBackAndAMixedLiteralKeepsItsText(t *testing.T) {
	t.Parallel()
	src := "typeset -a b=([0]=z [1]=w [-1]=x); typeset -p b\n" +
		"b=(7 8 9); b+=(x [-1]=y); typeset -p b\n" +
		"e=; c=(x $e z [4]=y); typeset -p c\n"
	want := "typeset -a b=(z x)\n" +
		"typeset -a b=(7 8 9 x '[-1]=y')\n" +
		"typeset -a c=(x z '[4]=y')\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}
