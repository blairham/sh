// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Inside a `"` run written in an operand that itself stands in double quotes,
// a backslash escapes whatever follows it in bash and ksh93, and only the
// four characters double quotes make special in dash, zsh and BusyBox ash —
// Dialect.NestedQuoteInAQuotedOperandEscapesAnything.
//
// A different question from NestedQuoteResetsOperandEscapes, which asks only
// whether the closing brace stays escapable in that run, and the panel splits
// along a different line. Measured 2026-09-22 with `u` unset:
//
//	printf '[%s]' "${u-"A\pB"}"
//
//	bash 5.3.20, that build as `sh`, bash 3.2.57, ksh93u+   [ApB]
//	dash 0.5.12, zsh 5.9.2, BusyBox ash 1.37.0              [A\pB]
func TestANestedQuotedRunMayEscapeAnythingInAQuotedOperand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, wide, narrow string }{
		{"an ordinary character", `printf "%s" "${u-"A\pB"}"`, "ApB", `A\pB`},
		{"a single quote", `printf "%s" "${u-"A\'B"}"`, "A'B", `A\'B`},

		// The four double quotes already made special, which must not have
		// moved: the widening adds to that set rather than replacing it.
		{"a dollar", `printf "%s" "${u-"A\$B"}"`, "A$B", "A$B"},
		{"a backslash", `printf "%s" "${u-"A\\B"}"`, `A\B`, `A\B`},
		{"a backtick", "printf \"%s\" \"${u-\"A\\`B\"}\"", "A`B", "A`B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, wide := range []bool{false, true} {
				d := Core()
				d.NestedQuoteInAQuotedOperandEscapesAnything = wide
				want := tc.narrow
				if wide {
					want = tc.wide
				}
				if got := operandLiteralIn(t, tc.src, d); got != want {
					t.Errorf("%s with wide=%v: operand text = %q, want %q", tc.src, wide, got, want)
				}
			}
		})
	}
}

// The enclosing quotes are what decides it. Written without them the operand
// is an ordinary word, and there the same nested run keeps the double-quote
// set in every column — `${u-"A\pB"}` unquoted is `A\pB` for all seven,
// measured 2026-09-22. So a dialect that sets the flag must not widen this.
func TestAnUnquotedOperandKeepsTheDoubleQuoteEscapes(t *testing.T) {
	t.Parallel()
	const src = `printf "%s" ${u-"A\pB"}`
	for _, wide := range []bool{false, true} {
		d := Core()
		d.NestedQuoteInAQuotedOperandEscapesAnything = wide
		if got := operandLiteralIn(t, src, d); got != `A\pB` {
			t.Errorf("%s with wide=%v: operand text = %q, want %q", src, wide, got, `A\pB`)
		}
	}
}
