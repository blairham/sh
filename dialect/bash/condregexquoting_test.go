// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestABracketExpressionTakesTheQuotingOffAndNothingMore — a character the
// script quoted in a `=~` operand is the character wherever it stands, and
// **inside a bracket expression that is all it is**: the two members that are
// still syntax there are not protected from being syntax.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. The rows are written as the script writes
// them, because the split is between a quote the script wrote and a backslash
// a value carries — the controls at the bottom are the same expressions held
// in a parameter, where the backslash is an ordinary member and stays one.
//
// Before this, every quoted character reached the matcher with a backslash in
// front of it. Outside a bracket that is the right spelling; inside one the
// backslash is a member in its own right, so the set gained a `\` and lost the
// character (#4173).
func TestABracketExpressionTakesTheQuotingOffAndNothingMore(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A quoted `.` is the character: the set holds it and not the
		// backslash that quoted it.
		{"a quoted dot is the dot", "[[ . =~ [\\.] ]]; echo $?\n", "0\n"},
		{"and not the backslash", "[[ \\\\ =~ [\\.] ]]; echo $?\n", "1\n"},
		// A quoted `]` still closes the set, which is what says the quoting
		// comes off rather than protecting the member.
		{"a quoted closer still closes", "[[ ']' =~ [\\]] ]]; echo $?\n", "0\n"},
		{"and the set holds nothing else", "[[ \\\\ =~ [\\]] ]]; echo $?\n", "1\n"},
		{
			"so a set after it is a member and a literal",
			"[[ 'a]' =~ [\\a\\]] ]]; echo \"$? [${BASH_REMATCH[0]}]\"\n",
			"0 [a]]\n",
		},
		{"which is not the member on its own", "[[ a =~ [\\a\\]] ]]; echo $?\n", "1\n"},
		{"nor the closer on its own", "[[ ']' =~ [\\a\\]] ]]; echo $?\n", "1\n"},
		// A quoted `]` first in a negated set is the member POSIX says it is.
		{"a quoted member in a negated set", "[[ \\\\ =~ [^]\\.] ]]; echo $?\n", "0\n"},
		// And the constructs a set holds are read after the quoting comes
		// off, not through it: the marks between the characters of an
		// equivalence class would otherwise hide it.
		{
			"an equivalence class written with quotes",
			"[[ abc =~ [\\[=a=\\]].. ]]; echo $?\n",
			"0\n",
		},
		{
			"and one beside a quoted bracket",
			"[[ aXa =~ [[=a=]].[\\[[=A=][=a=]] ]]; echo $?\n",
			"0\n",
		},
		// The controls. Held in a parameter the backslash is a member of the
		// set in its own right, which is POSIX's rule and the reference's.
		{"a value's backslash is a member", "r='[\\.]'\n[[ \\\\ =~ $r ]]; echo $?\n", "0\n"},
		{"and the set is not the closer's", "r='[\\]]'\n[[ ']' =~ $r ]]; echo $?\n", "1\n"},
		// And the control that says a quote outside a bracket is unchanged:
		// there the character still reaches the matcher protected.
		{"outside a bracket a quoted dot is still literal", "[[ axb =~ a\\.b ]]; echo $?\n", "1\n"},
		{"and matches itself", "[[ 'a.b' =~ a\\.b ]]; echo $?\n", "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}
