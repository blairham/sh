// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// isSet is the core with the one flag this file is about.
func isSet() Dialect {
	d := Core()
	d.ParameterIsSetTest = true
	return d
}

// `-v` is a one-operand test where the dialect has it, and the `-` is the
// start of an ordinary word where it does not.
//
// A flag rather than core, and by exactly one column: bash 3.2 is the only
// panel shell with `[[ ]]` that lacks the operator, and it does not merely
// answer differently — `conditional binary operator expected`, then `syntax
// error near `x”. dash has no `[[ ]]` at all. That is the same head count
// that made `-o` core, failing by one.
func TestAnIsSetTestIsAUnaryOperatorWhereTheDialectHasIt(t *testing.T) {
	on, off := isSet(), Core()
	for _, src := range []string{
		`[[ -v x ]]`,
		`[[ ! -v x ]]`,
		`[[ -v x && -v y ]]`,
		`[[ -v 'a[2]' ]]`,
		`[[ -v $n ]]`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
	}
	// Without the flag it is not an operator, so the condition is two words
	// where one belongs and the parse fails.
	for _, src := range []string{`[[ -v x ]]`, `[[ ! -v x ]]`} {
		if _, err := Parse(src, off); err == nil {
			t.Errorf("%s: parsed without the flag", src)
		}
	}
}

// The operand is an ordinary word, exactly as `-o`'s is, which is what says
// the flag adds an operator and nothing else about how a word is read.
func TestAnIsSetTestsOperandIsAnOrdinaryWord(t *testing.T) {
	on := isSet()
	for _, tc := range []struct{ src, want string }{
		{`[[ -v x ]]`, `[[ -v x ]]`},
		{`[[ -v 'a[2]' ]]`, `[[ -v 'a[2]' ]]`},
		{`[[ ! -v x ]]`, `[[ ! -v x ]]`},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		if got := Print(f); got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
		}
		if _, err := Parse(Print(f), on); err != nil {
			t.Errorf("%s: printed form does not parse: %v", tc.src, err)
		}
	}
	// A missing operand is a syntax error rather than a bare-word test, the
	// same as every other unary operator's. Measured: bash and ksh93 refuse
	// `[[ -v ]]` while parsing and zsh calls it an unknown condition, so all
	// three refuse it.
	if _, err := Parse(`[[ -v ]]`, on); err == nil {
		t.Error("`[[ -v ]]` parsed, want a refusal")
	}
	// And quoted it is a word rather than the operator, which is the rule
	// every operator in the table already has.
	if _, err := Parse(`[[ "-v" x ]]`, on); err == nil {
		t.Error("`[[ \"-v\" x ]]` parsed as the operator, where quoting makes it a word")
	}
}
