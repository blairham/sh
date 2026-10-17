// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The blanks written inside an assignment's brackets are part of the
// subscript: a refusal quotes them, and `@` with blanks around it is not the
// whole-array spelling but arithmetic (#6866).
//
// Measured 2026-10-16 against GNU bash 5.3.20 from a script file, byte for
// byte.
func TestTheBlanksInsideAnAssignmentsBracketsAreTheSubscripts(t *testing.T) {
	src := "a=(1)\n" +
		"a[ -9 ]=x\n" +
		"a[ $(echo -9;) ]=x\n" +
		"a[ 1 ]=(p q)\n" +
		"a[ @ ]=x\n"
	want := "bash: line 2: a[ -9 ]: bad array subscript\n" +
		"bash: line 3: a[ $(echo -9) ]: bad array subscript\n" +
		"bash: line 4: a[ 1 ]: cannot assign list to array member\n" +
		"bash: line 5: @ : arithmetic syntax error: operand expected (error token is \"@ \")\n"
	if got, _ := posixRouteStatus(t, src, interp.RouteScriptFile); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
