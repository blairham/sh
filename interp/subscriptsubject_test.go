// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The second refusal that quotes a subscript back is a list standing where one
// element's value goes, and it names the same subject: the text as written.
//
// A separate test from the boundary one because it is a separate sentence and
// a separate axis reaching it, and because this one refuses *before* the
// subscript is evaluated at all — `a[1/0]=(p q)` is this sentence and not a
// division by zero, so the written text is the only text there ever is
// (#1373).
func TestARefusedSubscriptedLiteralIsNamedAsWritten(t *testing.T) {
	sem := testSemantics()
	sem.SubscriptedArrayLiteral = SubscriptedArrayLiteralRefused
	for _, tc := range []struct{ src, want string }{
		{`i=1; a=(x y); a[$i]=(p q)`, `a[$i]: cannot assign list to array member`},
		{`i=1; a=(x y); a[$i+0]=(p q)`, `a[$i+0]: cannot assign list to array member`},
		{`i=1; a=(x y); a[$i]+=(p)`, `a[$i]: cannot assign list to array member`},
		// The control: a subscript with no expansion in it reads the same
		// either way.
		{`a=(x y); a[1]=(p q)`, `a[1]: cannot assign list to array member`},
	} {
		out, _ := run(t, tc.src, withSem(sem))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want it to name %q", tc.src, out, tc.want)
		}
	}
}
