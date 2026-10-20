// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A name on the left of an assignment may carry more than one subscript here,
// and the later ones reach *into* the value the earlier one named.
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME,
// read back with `typeset -p`. The value it builds is one this store already
// had — `a[1]=([2]=v)` has always built it — so what these rows pin is the
// walk and the spellings that reach it. #2491 filed the shape as needing a
// value kind, a subscript reader and a listing; two of the three were there.
func TestASecondSubscriptBuildsANestedCompound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a[1][2]=v; typeset -p a`, "typeset -a a=([1]=([2]=v) )\n"},
		{`a[1][2]=v; a[1][3]=w; typeset -p a`, "typeset -a a=([1]=([2]=v [3]=w) )\n"},
		{`a[1][2]=v; a[2][0]=w; typeset -p a`, "typeset -a a=([1]=([2]=v) [2]=(w) )\n"},
		{`a[1][2][3]=v; typeset -p a`, "typeset -a a=([1]=([2]=([3]=v) ) )\n"},
		{`a=(x y); a[1][2]=v; typeset -p a`, "typeset -a a=(x ([0]=y [2]=v) )\n"},
		{`a[1]=""; a[1][2]=v; typeset -p a`, "typeset -a a=([1]=([0]='' [2]=v) )\n"},
		{`a=(p q r); a[1][0]=v; typeset -p a`, "typeset -a a=(p (v) r)\n"},
		{`s=abc; s[1][2]=v; typeset -p s`, "typeset -a s=(abc ([2]=v) )\n"},
		{`typeset -A m; m[k][2]=v; typeset -p m`, "typeset -A m=([k]=([2]=v) )\n"},
		{`typeset -A m; m[k][x]=v; typeset -p m`, "typeset -A m=([k]=(v) )\n"},
		{`a[1][2]+=v; typeset -p a`, "typeset -a a=([1]=([2]=v) )\n"},
		{`a[1][2]=v; a[1][2]+=Q; typeset -p a`, "typeset -a a=([1]=([2]=vQ) )\n"},
		{`i=1; j=2; a[$i][$j]=v; typeset -p a`, "typeset -a a=([1]=([2]=v) )\n"},
		{`a[1][2]=v; echo "n=${#a[@]}"`, "n=1\n"},
		// The whole of it is one element however deep the nesting goes, and
		// the value it names is not the element's own reading: `${a[1]}` is
		// the nested array's first element, which here is not there.
		{`a[1][2]=v; echo "[${a[1]}]"`, "[]\n"},
	} {
		out, st := runKshChain(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Every declaration utility takes the chain as an operand, and lands in the
// same place the plain assignment does.
func TestADeclarationsOperandTakesTheChainHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset a[1][2]=v; typeset -p a`, "typeset -a a=([1]=([2]=v) )\n"},
		{`export a[1][2]=v; typeset -p a`, "typeset -x -a a=([1]=([2]=v) )\n"},
		{`readonly a[1][2]=v; typeset -p a`, "typeset -r -a a=([1]=([2]=v) )\n"},
		{`typeset -x a[1][2]=v; typeset -p a`, "typeset -x -a a=([1]=([2]=v) )\n"},
		{`f(){ typeset a[1][2]=v; typeset -p a; }; f`, "typeset -a a=([1]=([2]=v) )\n"},
		{`typeset a[1][2]=v; echo "n=${#a[@]}"`, "n=1\n"},
		// The value goes through the name's attribute, which is where an
		// element assignment's value meets it wherever it is written.
		{`typeset -i a[1][2]=5+5; typeset -p a`, "typeset -a -i a=([1]=([2]=10) )\n"},
	} {
		out, st := runKshChain(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The words of a nested *literal* are not values the name's attribute
// reaches, where the same attribute arriving over one afterwards does fold —
// and folds into the nesting rather than flattening it.
//
// The pair is the test. Taking the element's scalar reading instead folded a
// whole array into one number and lost every other element it held, silently.
func TestAnIntegerAttributeAndANestedArray(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i a; a[1]=(5+5); typeset -p a`, "typeset -a -i a=([1]=(5+5) )\n"},
		{`typeset -i a; a[1]=(2+2 3+3); typeset -p a`, "typeset -a -i a=([1]=(2+2 3+3) )\n"},
		{`a[1]=(5+5); typeset -i a; typeset -p a`, "typeset -a -i a=([1]=(10) )\n"},
	} {
		out, st := runKshChain(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A chain is a thing a *write* may name and not a thing any operand may be:
// `unset a[1][2]` is refused here, so the reader is asked for the declaration
// utilities and not for `unset`, which would otherwise take the last subscript
// for the whole name and remove an element nobody named.
func TestUnsetDoesNotTakeAChain(t *testing.T) {
	out, st := runKshChain(t, `unset a[1][2]; echo st=$?`)
	if st != 0 || !strings.Contains(out, "a[1][2]") || !strings.Contains(out, "st=1") {
		t.Errorf("`unset a[1][2]` = %q (status %d), want the operand refused at 1", out, st)
	}
}

func runKshChain(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
