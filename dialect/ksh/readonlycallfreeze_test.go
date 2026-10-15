// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `readonly` inside a `function`-defined function freezes a local of its own
// and gives the value to the caller (#6466). Measured 2026-10-15 against
// ksh93u+ 2012-08-01 under `env -i PATH=/usr/bin:/bin`, `-c`. See
// interp.Semantics.ReadonlyFreezeStaysWithTheCall.
func TestReadonlyInAKeywordFunctionFreezesOnlyTheCall(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`function f { readonly x; }; f; x=1; echo "x=$x"`, "x=1\n"},
		{`x=0; function f { readonly x; }; f; x=1; echo "x=$x"`, "x=1\n"},
		{`function f { readonly x=5; echo "in=$x"; }; f; echo "out=${x-unset}"; x=1; echo "x=$x"`, "in=\nout=5\nx=1\n"},
		{`function f { readonly x=5 y; echo "in=${x-u} ${y-u}"; }; f; echo "out=${x-u} ${y-u}"; x=1 y=2; echo "$x $y"`, "in=u u\nout=5 u\n1 2\n"},
		{`x=0; function f { readonly x; echo "in=$x"; typeset -p x; }; f; typeset -p x`, "in=\ntypeset -r x\nx=0\n"},
		{`function f { readonly x[1]=5; typeset -p x; }; f; typeset -p x; x[1]=2; echo "${x[1]}"`, "typeset -r x\ntypeset -a x=([1]=5)\n2\n"},
		{`x=(a b); function f { readonly x; typeset -p x; }; f; typeset -p x; x[1]=2; echo "${x[1]}"`, "typeset -r x\ntypeset -a x=(a b)\n2\n"},
		{`x=0; typeset -n r=x; function f { readonly r=4; echo "in=${x-u}"; }; f; echo "out=$x"; x=1; echo $x`, "in=4\nout=4\n1\n"},
		{`function g { typeset x=1; f; echo "g=$x"; x=2; echo "g2=$x"; }; function f { readonly x; }; g; x=3; echo "top=$x"`, "g=1\ng2=2\ntop=3\n"},
		{`function f { typeset x=3; readonly x=5; echo "in=$x"; }; f; echo "out=${x-unset}"`, "in=5\nout=unset\n"},
		{`s=ab; function s.get { readonly x; }; x=${s#a}; echo "x=$x"`, "x=b\n"},
	} {
		if got := kshC(t, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// And where ksh93u+ does freeze: a POSIX-style function has no scope of its
// own, and a name the call already declared is frozen in the call.
func TestReadonlyInAKshFunctionStillFreezes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`f() { readonly x; }; f; x=1; echo "x=$x"`, "x: is read only"},
		{`function f { readonly x; x=6; echo "in=$x"; }; f; echo "out=${x-unset}"`, "x: is read only"},
		{`readonly x=0; function f { readonly x=1; }; f; echo "out=$x"`, "x: is read only"},
		// A reference the call already declared is followed, and so is one
		// reached from a function with no scope: the freeze lands on what it
		// points at.
		{`x=0; function f { typeset -n r=x; readonly r; }; f; x=1; echo "x=$x"`, "x: is read only"},
		{`x=0; typeset -n r=x; f() { readonly r; }; f; x=1; echo "x=$x"`, "x: is read only"},
	} {
		if got := kshC(t, c.src); !strings.Contains(got, c.want) || strings.Contains(got, "x=1") {
			t.Errorf("%s\n got %q, want a refusal", c.src, got)
		}
	}
}
