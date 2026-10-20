// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An indexed array that has never held an element, given a whole-name scalar
// store, lists as that scalar under its `-a` letter. It does so until an
// array write touches it. Measured 2026-10-03 on ksh93u+ 2012-08-01 under
// `-c` (#5643). See interp.Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar.
func TestAScalarStoreIntoAnEmptyArrayListsAsTheScalar(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -a x; x=/y; typeset -p x`, "typeset -a x=/y\n"},
		{`typeset -a x=(); x=/y; typeset -p x`, "typeset -a x=/y\n"},
		{`typeset -a x; x="a b"; typeset -p x`, "typeset -a x='a b'\n"},
		{`typeset -a x; read x <<< /y; typeset -p x`, "typeset -a x=/y\n"},
		{`typeset -a x; for x in /y; do :; done; typeset -p x`, "typeset -a x=/y\n"},
		// Another scalar store, a scalar append and a read keep it.
		{`typeset -a x; x=/y; x=w; typeset -p x`, "typeset -a x=w\n"},
		{`typeset -a x; x=/y; x+=w; typeset -p x`, "typeset -a x=/yw\n"},
		{`typeset -a x; x=/y; echo ${#x[@]} ${x[0]}; typeset -p x`, "1 /y\ntypeset -a x=/y\n"},
		{`typeset -a x; x=/y; typeset -a x; typeset -p x`, "typeset -a x=/y\n"},
		{`function f { typeset -a x; x=/y; typeset -p x; }; f`, "typeset -a x=/y\n"},
		// A function's own local does not end the caller's.
		{`typeset -a x; x=/y; function f { typeset -a x; x[0]=q; }; f; typeset -p x`, "typeset -a x=/y\n"},
		// A function called from one that has its own local reaches the
		// global under static scope, and lists and writes that.
		{`typeset -a x; x=/y; function f { typeset -a x; x[0]=q; g; }; function g { typeset -p x; x[1]=w; }; f; typeset -p x`, "typeset -a x=/y\ntypeset -a x=(/y w)\n"},
		{`typeset -a x; x=/y; function f { typeset -a x; x[0]=q; g; typeset -p x; }; function g { typeset -p x; }; f; typeset -p x`, "typeset -a x=/y\ntypeset -a x=(q)\ntypeset -a x=/y\n"},
		// A local starts with no record of the caller's: it lists as the
		// scalar it was given, across a static call, and an emptied caller
		// does not make the local's listing `([0]=)`.
		{`typeset -a x; x[0]=p; function f { typeset -a x; x=/q; g; typeset -p x; }; function g { typeset -p x; }; f; typeset -p x`, "typeset -a x=(p)\ntypeset -a x=/q\ntypeset -a x=(p)\n"},
		{`typeset -a x; x[0]=p; unset 'x[0]'; function f { typeset -a x; typeset -p x; }; f; typeset -p x`, "typeset -a x\ntypeset -a x=([0]=)\n"},
		// Any array write ends it.
		{`typeset -a x; x=/y; x[1]=z; unset x[1]; typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x; x=/y; unset x[1]; typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x; x=/y; x+=(z); typeset -p x`, "typeset -a x=(/y z)\n"},
		{`typeset -a x; x=/y; set -A x q; typeset -p x`, "typeset -a x=(q)\n"},
		// The controls: an element store, and a name that held an element.
		{`typeset -a x; x[0]=/y; typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x=(a); x=/y; typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x=(a); unset x[0]; x=/y; typeset -p x`, "typeset -a x=(/y)\n"},
		{`typeset -a x; x=/y; unset x; typeset -a x; typeset -p x`, "typeset -a x\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
