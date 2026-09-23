// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestATokenWhereARegexBelongsIsAnUnexpectedToken — the one operand read as a
// regular expression words an operator token standing in its place as **a token
// the grammar did not want** rather than as an argument the operator would not
// take. The closer is the exception, and every other operator keeps the
// argument form — so it is this operator and not this position.
//
// Measured 2026-09-23 against bash 5.3.15 in the digest-pinned image the suite
// is graded in. This shell wrote the argument form for all of them, and for a
// `)` did not refuse at all: it read `)x` as the operand and reported an invalid
// expression at run time with the script carrying on, where the reference ends
// it while reading (#4173).
func TestATokenWhereARegexBelongsIsAnUnexpectedToken(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The token reading, for the operand `=~` reads as an expression.
		{"[[ x =~ ) ]]", "syntax error in conditional expression: unexpected token `)'"},
		{"[[ x =~ & ]]", "syntax error in conditional expression: unexpected token `&'"},
		{"[[ x =~ < ]]", "syntax error in conditional expression: unexpected token `<'"},
		{"[[ x =~ ; ]]", "syntax error in conditional expression: unexpected token `;'"},
		// A closer that closes nothing ends the word, so what follows a partial
		// operand is refused the same way.
		{"[[ x =~ a) ]]", "syntax error in conditional expression: unexpected token `)'"},
		{"[[ x =~ (x)) ]]", "syntax error in conditional expression: unexpected token `)'"},
		// The exceptions. The construct's own closer keeps the argument form,
		// and so does every other operator's operand — which is what parts the
		// two readings.
		{"[[ x =~ ]]", "unexpected argument `]]' to conditional binary operator"},
		{"[[ x == ) ]]", "unexpected argument `)' to conditional binary operator"},
		{"[[ -n ) ]]", "unexpected argument `)' to conditional unary operator"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out := condDiagnostic(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("said %q, want a line ending %q", out, tc.want)
			}
		})
	}
	// And a balanced group is still the expression's, which is the control that
	// says none of this reaches an operand that parses.
	for _, src := range []string{"[[ x =~ (x) ]]", "[[ x =~ ((x)) ]]", "[[ a =~ |a ]]", "[[ a =~ a|b ]]"} {
		out, errs := runBashSplit(t, src+"\necho after\n")
		if out != "after\n" || errs != "" {
			t.Errorf("%s: out %q errs %q, want it to parse and run", src, out, errs)
		}
	}
}
