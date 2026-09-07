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
// division by zero. A subscript holding an *expansion* is the one place the
// quoting is not reproduced here; see the row that says so.
func TestAnArrayLiteralThroughASubscriptIsRefused(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(x y); a[1]=(p q); echo after`, "a[1]: cannot assign list to array member"},
		{`a=(x y); a[1]=(); echo after`, "a[1]: cannot assign list to array member"},
		{`a=(x y); a[1]+=(p); echo after`, "a[1]: cannot assign list to array member"},
		// A subscript holding an expansion is named by what it came to
		// rather than by what was typed, which is this tree's existing gap
		// on `bad array subscript` and not one this construct introduced:
		// the same `a[$i]` is quoted back whole by the shell and evaluated
		// by us on both routes alike.
		{`i=1; a=(x y); a[$i]=(p q); echo after`, "a[1]: cannot assign list to array member"},
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
