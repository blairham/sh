// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// nameRef is the core with the one flag this file is about.
func nameRef() Dialect {
	d := Core()
	d.NameReferenceTest = true
	return d
}

// `-R` is a one-operand test where the dialect has it, and the `-` is the
// start of an ordinary word where it does not.
//
// A flag rather than core, decided by the same column that decided `-v`'s.
// Measured 2026-09-22 under `-c` with `LC_ALL=C`: bash 5.3.20 and ksh93u+
// read it, bash 3.2.57 cannot read the line at all (`conditional binary
// operator expected`, then a syntax error naming the operand), zsh 5.9.2 says
// `unknown condition: -R`, and dash has no `[[ ]]` to put it in.
func TestANameReferenceTestIsAUnaryOperatorWhereTheDialectHasIt(t *testing.T) {
	t.Parallel()
	on, off := nameRef(), Core()
	for _, src := range []string{
		`[[ -R r ]]`,
		`[[ ! -R r ]]`,
		`[[ -R r && -R s ]]`,
		`[[ -R 'a[2]' ]]`,
		`[[ -R $n ]]`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
	}
	// Without the flag it is not an operator, so the condition is two words
	// where one belongs and the parse fails. This is the mutation that
	// matters: #4228 was exactly this refusal reaching a dialect that has
	// the operator.
	for _, src := range []string{`[[ -R r ]]`, `[[ ! -R r ]]`} {
		if _, err := Parse(src, off); err == nil {
			t.Errorf("%s: parsed without the flag", src)
		}
	}
}

// The operand is an ordinary word, exactly as `-v`'s and `-o`'s are, which is
// what says the flag adds an operator and nothing else about how a word is
// read.
func TestANameReferenceTestsOperandIsAnOrdinaryWord(t *testing.T) {
	t.Parallel()
	on := nameRef()
	for _, tc := range []struct{ src, want string }{
		{`[[ -R r ]]`, `[[ -R r ]]`},
		{`[[ -R 'a[2]' ]]`, `[[ -R 'a[2]' ]]`},
		{`[[ ! -R r ]]`, `[[ ! -R r ]]`},
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
	// same as every other unary operator's.
	if _, err := Parse(`[[ -R ]]`, on); err == nil {
		t.Error("`[[ -R ]]` parsed, want a refusal")
	}
	// And quoted it is a word rather than the operator.
	if _, err := Parse(`[[ "-R" r ]]`, on); err == nil {
		t.Error("`[[ \"-R\" r ]]` parsed as the operator, where quoting makes it a word")
	}
}
