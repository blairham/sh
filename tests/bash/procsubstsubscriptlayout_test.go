// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A process substitution written in a subscript is text, and the text is
// its body laid out as a command substitution's is: the key a table stores,
// the listing, the trace and the arithmetic refusal all hold `<(a; b)` for a
// written `<(a;b)` (#6737).
//
// Measured 2026-10-16 against GNU bash 5.3.20 from a script file, byte for
// byte.
func TestAProcessSubstitutionInASubscriptIsLaidOut(t *testing.T) {
	src := "declare -A A; A[<(a;b)]=v; declare -p A; echo \"[${A[<(a;b)]}]\"\n" +
		"f(){ a[<(a;b)]=1; }; declare -f f\n" +
		"set -x; a[<(echo 1 >&2)]=x\n"
	want := "declare -A A=([\"<(a; b)\"]=\"v\" )\n" +
		"[v]\n" +
		"f () \n{ \n    a[<(a; b)]=1\n}\n" +
		"+ a[<(echo 1 1>&2)]=x\n" +
		"bash: line 3: <(echo 1 1>&2): arithmetic syntax error: operand expected (error token is \"<(echo 1 1>&2)\")\n"
	if got, _ := posixRouteStatus(t, src, interp.RouteScriptFile); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}
