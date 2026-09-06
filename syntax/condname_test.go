// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// condErr parses src under a dialect with `[[ ]]` and returns the structured
// failure, which is what a dialect words from.
func condErr(t *testing.T, src string) *Error {
	t.Helper()
	_, err := Parse(src, Core())
	if err == nil {
		t.Fatalf("%s: parsed, want a refusal", src)
	}
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("%s: %T, want a syntax error", src, err)
	}
	return se
}

// TestARefusedConditionNamesItsToken — `expected ]]` was one sentence for
// every way a condition can be malformed, and every shell in the panel names
// the *offending* token instead. The parser records the token rather than a
// message, so each dialect words it.
func TestARefusedConditionNamesItsToken(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{`[[ -n x -z "" ]]`, "-z"},
		{`[[ -n x y ]]`, "y"},
		{"[[ -n x\n -z \"\" ]]", "-z"},
		{`[[ -n x ; ]]`, ";"},
		{`[[ -n x | y ]]`, "|"},
		{`[[ a b c d ]]`, "b"},
	} {
		se := condErr(t, tc.src)
		if se.Kind != ErrUnexpected {
			t.Errorf("%s: kind %v, want an unexpected token", tc.src, se.Kind)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%s: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

// TestARefusedConditionCarriesWhereItOpened — one dialect writes a line about
// the *construct* in front of the one about the token, at the `[[`'s line
// rather than the token's, so the position of the `[[` has to travel with the
// error. A condition opened and refused on the same line is the case that
// would pass if the line were simply copied.
func TestARefusedConditionCarriesWhereItOpened(t *testing.T) {
	se := condErr(t, "[[ -n x\n -z \"\" ]]")
	if se.Construct != "[[" {
		t.Errorf("construct = %q, want `[[`", se.Construct)
	}
	if se.ConstructLine != 1 {
		t.Errorf("construct line = %d, want 1 — where the `[[` is", se.ConstructLine)
	}
	if se.Pos.Line != 2 {
		t.Errorf("token line = %d, want 2 — where the token is", se.Pos.Line)
	}
	// Same line for both when it is written on one, which is the case a
	// wrong implementation still gets right.
	se = condErr(t, `[[ -n x ; ]]`)
	if se.ConstructLine != 1 || se.Pos.Line != 1 {
		t.Errorf("one-line condition: construct %d token %d, want 1 and 1",
			se.ConstructLine, se.Pos.Line)
	}
	// And a failure outside a condition carries no construct at all, or
	// every syntax error in the shell would get the extra line.
	se = condErr(t, `echo a (b)`)
	if se.Construct == "[[" {
		t.Error("a failure outside a condition was blamed on `[[`")
	}
}

// TestAClosingBracketIsNotAnOperand — the `]]` was read as a word where an
// operand belonged, so `[[ -n ]]` blamed whatever followed the condition
// rather than the `]]` in front of it. Every shell in the panel names the
// `]]`.
func TestAClosingBracketIsNotAnOperand(t *testing.T) {
	for _, tc := range []struct {
		src, token, arity string
	}{
		{`[[ -n ]]`, "]]", "unary"},
		{`[[ x == ]]`, "]]", "binary"},
	} {
		se := condErr(t, tc.src)
		if se.Kind != ErrCondOperand {
			t.Errorf("%s: kind %v, want the operand refusal", tc.src, se.Kind)
			continue
		}
		if se.Token != tc.token || se.Expected != tc.arity {
			t.Errorf("%s: blamed %q as %q, want %q as %q",
				tc.src, se.Token, se.Expected, tc.token, tc.arity)
		}
	}
	// A *quoted* `]]` is an ordinary word, so the check is on the spelling
	// and not on the text: `[[ -n "]]" ]]` tests a two-character string.
	if _, err := Parse(`[[ -n "]]" ]]`, Core()); err != nil {
		t.Errorf("a quoted `]]` was refused as an operand: %v", err)
	}
}
