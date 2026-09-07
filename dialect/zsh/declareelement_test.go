// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration's subscripted operand writes the element here, and it takes
// one for `export` as well as for `typeset`.
//
// The `export` answer read No before this and was never reachable: the operand
// was glob-matched before the builtin saw it, so the line died as
// `no matches found: a[1]=v` and the name check was never asked (#1203).
func TestASubscriptedOperandWritesTheElement(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`export a[1]=v; echo "st=$? [${a[1]}]"`, "st=0 [v]\n"},
		{`typeset -x a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`typeset -A m; typeset m[k]=v; echo "[${m[k]}]"`, "[v]\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the three things it will not let an element carry, each in its own
// words — measured 2026-09-07 against zsh 5.9.2, where all three end the
// script.
func TestASubscriptedOperandRefusesWhatBelongsToTheName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{
			`readonly a[1]=v; echo after`,
			"zsh:readonly:1: a[1]: can't create readonly array elements\n",
		},
		{
			`typeset -r a[1]=v; echo after`,
			"zsh:typeset:1: a[1]: can't create readonly array elements\n",
		},
		{
			`typeset -i a[1]=5; echo after`,
			"zsh:typeset:1: a[1]: inconsistent array element or slice assignment\n",
		},
		{
			`f() { typeset a[1]=v; }; f; echo after`,
			"f:typeset: a[1]: can't create local array elements\n",
		},
		{
			`f() { local a[1]=v; }; f; echo after`,
			"f:local: a[1]: can't create local array elements\n",
		},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", tc.src, out, st, tc.want)
		}
	}
}

// The store's own complaint about a subscript does not name the builtin where
// the refusals above do — and the first element is number one here, so `a[0]`
// is what asks the question.
func TestTheSubscriptRangeComplaintDoesNotNameTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`export a[0]=v`, `typeset a[0]=v`, `a[0]=v`} {
		out, st := runZsh(t, dir, src)
		if want := "zsh:1: a: assignment to invalid subscript range\n"; out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}
