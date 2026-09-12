// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An array literal written through a subscript is refused and the script ends.
//
// Measured 2026-09-07 in bash 5.3.15 and 3.2.57, which give the same sentence
// at status 1 and run nothing after it. The set below is the one that says the
// refusal is about the *list* and not about the name: an array, a scalar, a
// declared table and a name holding nothing are all refused alike, and the
// subscript is quoted back as it was written rather than as it evaluated —
// which is also why an unevaluable one is still this refusal and not a
// division by zero, and why a subscript holding an *expansion* is named by
// the expansion (#1373).
func TestAnArrayLiteralThroughASubscriptIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(x y); a[1]=(p q); echo after`, "a[1]: cannot assign list to array member"},
		{`a=(x y); a[1]=(); echo after`, "a[1]: cannot assign list to array member"},
		{`a=(x y); a[1]+=(p); echo after`, "a[1]: cannot assign list to array member"},
		// A subscript holding an expansion is named by what was *typed*, and
		// it is the row that says the subject is the source text: `a[1]`
		// would be the value it came to, and both are plausible.
		{`i=1; a=(x y); a[$i]=(p q); echo after`, "a[$i]: cannot assign list to array member"},
		{`i=1; a=(x y); a[$i+0]=(p q); echo after`, "a[$i+0]: cannot assign list to array member"},
		{`s=abc; s[2]=(p q); echo after`, "s[2]: cannot assign list to array member"},
		{`declare -A h; h[k]=(p q); echo after`, "h[k]: cannot assign list to array member"},
		{`unset a; a[3]=(p q); echo after`, "a[3]: cannot assign list to array member"},
		{`a=(x y); a[1/0]=(p q); echo after`, "a[1/0]: cannot assign list to array member"},
	} {
		out, st := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s:\n  said %q\n  want it to contain %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: the rest of the line ran; the refusal should end the script", tc.src)
		}
		if st != 1 {
			t.Errorf("%s: status %d, want 1", tc.src, st)
		}
	}
}

// The same subject on the boundary refusal, which is the other sentence that
// quotes a subscript back.
//
// Measured 2026-09-07 and re-measured 2026-09-12 in bash 5.3.15 and 3.2.57:
// `i=-9; a=(x); a[$i]=q` is `a[$i]: bad array subscript`, not `a[-9]`. The
// rows with no expansion in them are the controls — they read the same either
// way, which is why three corpus cases written with a numeral passed while
// this did not (#1373).
func TestABoundarySubscriptIsNamedAsWritten(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`i=-9; a=(x); a[$i]=q`, "a[$i]: bad array subscript"},
		{`i=-9; a=(x); a[$i+0]=q`, "a[$i+0]: bad array subscript"},
		{`i=-9; a=(x); a[${i}]=q`, "a[${i}]: bad array subscript"},
		{`i=-9; a=(x); a[$i]+=q`, "a[$i]: bad array subscript"},
		{`a=(x); a[$(echo -9)]=q`, "a[$(echo -9)]: bad array subscript"},
		// Controls: no expansion, so the source and the value are one text.
		{`x=1; a[x-2]=v`, "a[x-2]: bad array subscript"},
		{`a=(p q); a[-3]=v`, "a[-3]: bad array subscript"},
		// A subscript that arrived as a *string* is named by its value, which
		// is what the same shell does: the operand was expanded before any of
		// it was source text.
		{`i=-9; a=(x); typeset "a[$i]"=q`, "a[-9]: bad array subscript"},
		// And an expression that will not evaluate is quoted back expanded.
		{`i=1; a[$i/0]=x`, "1/0"},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s:\n  said %q\n  want it to contain %q", tc.src, out, tc.want)
		}
	}
}
