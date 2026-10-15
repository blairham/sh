// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A valueless declaration operand whose brackets name an element past the
// first makes the name an indexed array, the kind that has held an element
// when it is left holding none; the first element is the name itself and
// declares no array. The same under all three declaration words and in the
// local `readonly` freezes inside a `function`-defined function (#6490).
// Measured 2026-10-15 against ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin`, `-c`.
func TestAValuelessSubscriptPastTheFirstDeclaresAnArray(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset x[1]; typeset -p x; echo "c=${#x[@]} z=${x[0]-U}"`, "typeset -a x=([0]=)\nc=0 z=U\n"},
		{`readonly x[1]; typeset -p x`, "typeset -r -a x=([0]=)\n"},
		{`export x[1]; typeset -p x`, "typeset -x -a x=([0]=)\n"},
		{`typeset -a x; readonly x[1]; typeset -p x`, "typeset -r -a x=([0]=)\n"},
		{`typeset x; export x[1]; typeset -p x`, "typeset -x -a x=([0]=)\n"},
		{`typeset x[1]; x=q; typeset -p x`, "typeset -a x=(q)\n"},
		{`typeset x[0]; typeset -p x; echo "c=${#x[@]}"`, "c=0\n"},
		{`readonly x[0]; typeset -p x`, "typeset -r x\n"},
		{`export x[0]; typeset -p x`, "typeset -x x\n"},
		{`x=s; readonly x[0]; typeset -p x`, "typeset -r x=s\n"},
		{`x=(a b); typeset x[1]; typeset -p x`, "typeset -a x=(a b)\n"},
		// The brackets are read a second time only where the first pass
		// found the first element.
		{`i=1; typeset "c[i++]"; echo "i=$i"`, "i=2\n"},
		{`i=5; readonly "c[i++]"; echo "i=$i"`, "i=6\n"},
		{`c=(1 2 3); i=0; typeset "c[i++]"; echo "i=$i"`, "i=2\n"},
		// The local a keyword function's `readonly` freezes.
		{`x=(a b); function f { readonly x[1]; typeset -p x; }; f; typeset -p x`, "typeset -r -a x=([0]=)\ntypeset -a x=(a b)\n"},
		{`function f { readonly x[1]; echo "a=${x-u} b=${x[0]-u} c=${#x[@]}"; }; f`, "a=u b=u c=0\n"},
		{`k=2; function f { readonly x[k]; typeset -p x; }; f`, "typeset -r -a x=([0]=)\n"},
		{`x=s; function f { readonly x[1]; typeset -p x; }; f; typeset -p x`, "typeset -r -a x=([0]=)\nx=s\n"},
		{`x=(a b); function f { readonly x[0]; typeset -p x; }; f; typeset -p x`, "typeset -r x\ntypeset -a x=(a b)\n"},
		{`typeset -A m=([k]=v); function f { readonly m[j]; typeset -p m; }; f; typeset -p m`, "typeset -r m\ntypeset -A m=([k]=v)\n"},
		{`i=1; function f { readonly x[i++]; }; f; echo "i=$i"`, "i=2\n"},
		{`function f { readonly x[1]; x[2]=3; }; f; echo "st=$?"`, "st=1\nksh: f: line 1: x: is read only\n"},
	} {
		if got := kshC(t, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// And the refusal of brackets that will not evaluate names the builtin, in the
// local's path as in the others.
func TestAValuelessBadSubscriptNamesTheBuiltin(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly 'x[a b]'; echo after`, "ksh: readonly: a b: arithmetic syntax error\n"},
		{`export 'x[a b]'; echo after`, "ksh: export: a b: arithmetic syntax error\n"},
		{`function f { readonly 'x[a b]'; echo in; }; f; echo after`, "after\nksh: f[1]: readonly: a b: arithmetic syntax error\n"},
	} {
		if got := kshC(t, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
