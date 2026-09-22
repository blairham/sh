// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `-` or `?` behind a `${#` is the name in every dialect. What the flag
// decides is what happens when the *rest* of the expansion cannot be part of
// that reading: the grammar either keeps the name, or falls back and reads the
// `#` as the parameter `$#` with an operator on it.
//
// This is the decision function rather than a whole parse, because the two
// readings differ in the node they build and the flag is the only input that
// separates them.
func TestWhenALengthOverASpecialNameYieldsToTheParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rest        string
		final, fall bool
	}{
		// Nothing left over: the name in both, and so a length in both.
		{"-", false, false},
		{"?", false, false},
		// Left over, and the fallback is the only thing that moves.
		{"-w", false, true},
		{"?w", false, true},
		{"-:-x", false, true},
		{"?:-x", false, true},
		// The boundary: a special name that is not also an operator has no
		// second reading to fall back to, under either value.
		{"$w", false, false},
		{"!w", false, false},
		{"@w", false, false},
		// And an ordinary name, which was never a question.
		{"v", false, false},
		{"vw", false, false},
	} {
		if got := hashIsTheParameter(tc.rest, false, true, false); got != tc.final {
			t.Errorf("${#%s} with the length final: parameter = %v, want %v", tc.rest, got, tc.final)
		}
		if got := hashIsTheParameter(tc.rest, false, false, false); got != tc.fall {
			t.Errorf("${#%s} with the fallback: parameter = %v, want %v", tc.rest, got, tc.fall)
		}
	}
}

// A bare `+` or `=` behind a `${#` is the other half of the same decision, and
// it is the one row of the table that used to be written without an operand:
// neither character can begin a name, so with nothing after it the length
// reading finds none and the whole expansion is refused — in five of the six
// columns. The sixth keeps the parameter reading, which is what the flag says.
//
// With an operand the two readings do not compete at all, so both values of
// the flag answer the same there. See
// [Dialect.ParamLengthBareOperatorIsTheParameter] for the panel (#4166).
func TestWhenABareOperatorBehindALengthYieldsToTheParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rest       string
		strict, zs bool
	}{
		// Bare: the flag is the whole of the difference.
		{"+", false, true},
		{"=", false, true},
		// With an operand: the parameter in both, as it always was.
		{"+w", true, true},
		{"=w", true, true},
		{"=:=x", true, true},
		// The neighbors, which already required an operand and are not
		// moved by this flag in either direction.
		{"%", false, false},
		{"/", false, false},
		{"#", false, false},
		{"%w", true, true},
	} {
		if got := hashIsTheParameter(tc.rest, false, false, false); got != tc.strict {
			t.Errorf("${#%s} with a bare operator refused: parameter = %v, want %v",
				tc.rest, got, tc.strict)
		}
		if got := hashIsTheParameter(tc.rest, false, false, true); got != tc.zs {
			t.Errorf("${#%s} with a bare operator taken: parameter = %v, want %v",
				tc.rest, got, tc.zs)
		}
	}
}
