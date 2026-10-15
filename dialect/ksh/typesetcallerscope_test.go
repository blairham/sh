// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A POSIX-style function has no scope of its own and runs in its caller's, so
// `typeset` in one called from a `function`-defined function declares a local
// of that call (#6488). Measured 2026-10-15 against ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin`, `-c`. See
// interp.Semantics.TypesetLocalNeedsKeywordFunction. kshC writes the standard
// output ahead of the standard error.
func TestTypesetInAPosixFunctionDeclaresIntoTheKeywordCaller(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`function g { h; echo "g=${x-u}"; x=2; echo "g2=$x"; }; h() { typeset -r x; }; g; x=3; echo "top=$x"`, "g=u\ntop=3\nksh: g: line 1: x: is read only\n"},
		{`function g { h; echo "g=${x-u}"; }; h() { typeset -r x=5; }; g; echo "top=${x-u}"`, "g=5\ntop=u\n"},
		{`function g { h; x=2; echo "g=$x"; }; h() { readonly x; }; g; x=3; echo "top=$x"`, "top=3\nksh: g: line 1: x: is read only\n"},
		{`function g { typeset x=1; h; echo "g=${x-u}"; x=2; }; h() { typeset -r x=5; }; g; echo "top=${x-u}"`, "g=5\ntop=u\nksh: g: line 1: x: is read only\n"},
		{`function g { k; echo "g=${x-u}"; }; k() { h; echo "k=${x-u}"; }; h() { typeset x=5; }; g; echo "top=${x-u}"`, "k=5\ng=5\ntop=u\n"},
		{`function o { g; echo "o=${x-u}"; }; function g { h; echo "g=${x-u}"; }; h() { typeset x=5; }; o; echo "top=${x-u}"`, "g=5\no=u\ntop=u\n"},
		{`x=0; function k { echo "k=${x-u}"; }; function g { h; k; echo "g=${x-u}"; }; h() { typeset x=5; }; g; echo "top=$x"`, "k=0\ng=5\ntop=0\n"},
		{`function g { h; echo "g=${x-u}"; }; h() { integer x=3+4; }; g; echo "top=${x-u}"`, "g=7\ntop=u\n"},
		{`function g { h; typeset -p a; }; h() { typeset -a a=(p q); a[2]=r; }; g; echo "top=${a[*]-u}"`, "typeset -a a=(p q r)\ntop=u\n"},
		{`function g { h; echo "g=${x-u}"; }; h() { readonly x=5; }; g; echo "top=${x-u}"; x=1; echo "top2=$x"`, "g=u\ntop=5\ntop2=1\n"},
		{`h() { typeset x=5; }; h; echo "top=${x-u}"`, "top=5\n"},
	} {
		if got := kshC(t, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
