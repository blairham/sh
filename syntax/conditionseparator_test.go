// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// A `;` where a command belongs is stepped over by two of the panel, and one
// of them will not step over it where a *condition* begins.
//
// Measured on ksh93u+ 2026-09-12, `-n` over a script file. Every line here is
// taken by zsh, which is the wider value of the same axis:
//
//	if; then :; fi                  `;' unexpected
//	while; do :; done               `;' unexpected
//	until; do :; done               `;' unexpected
//	if :; then :; elif; then :; fi  `;' unexpected
//	if :; ; then :; fi              `;' unexpected
//	if : ; :; then :; fi            runs
//	if false || ; then :; fi        runs
//	if :; then : ; ; fi             runs
//
// So the position is where a *statement of a condition list* begins: not the
// header as a whole, since a `;` terminating a statement of it is fine and so
// is one standing where an and-or's right-hand side belongs; and not the token
// after the keyword, since the second `;` of `if :; ; then` is refused too
// (#2023).

// semicolonDialect is the axis with the companions each of its two values
// travels with, so that a row below is about the separator and not about a
// flag the preset holding this value also has: an and-or whose right-hand
// side was stepped over needs something to stand there, and a condition that
// stepped its only statement over is an empty body.
func semicolonDialect(v SeparatorSkip) Dialect {
	d := Core()
	d.SeparatorWhereACommandBelongs = v
	d.AbsentAndOrOperandIsAnEmptyCommand = v == OneSeparatorExceptAfterABarOrBeforeACondition
	d.EmptyCompoundBody = v == AnySeparatorWhereACommandBelongs
	return d
}

func TestASeparatorMayNotBeginAConditionInTheNarrowerDialect(t *testing.T) {
	refused := []string{
		"if; then :; fi",
		"while; do :; done",
		"until; do :; done",
		"if :; then :; elif; then :; fi",
		"if :; ; then :; fi",
	}
	taken := []string{
		"if : ; :; then :; fi",
		"if false || ; then :; fi",
		"if :; then : ; ; fi",
		"{ : ; ; }",
	}
	narrow := semicolonDialect(OneSeparatorExceptAfterABarOrBeforeACondition)
	wide := semicolonDialect(AnySeparatorWhereACommandBelongs)

	for _, src := range refused {
		_, err := Parse(src, narrow)
		var se *Error
		if !errors.As(err, &se) {
			t.Errorf("%q: %v, want a refusal naming the separator", src, err)
			continue
		}
		if se.Token != ";" {
			t.Errorf("%q: blamed %q, want %q", src, se.Token, ";")
		}
		// The wider value takes every one of them, which is what makes this
		// the dialect's answer and not the position's.
		if _, err := Parse(src, wide); err != nil {
			t.Errorf("%q at the wider value: %v, want it taken", src, err)
		}
	}
	for _, src := range taken {
		if _, err := Parse(src, narrow); err != nil {
			t.Errorf("%q: %v, want it taken — this is not a condition's first token", src, err)
		}
	}
	// And a dialect that steps over no separator at all refuses the taken
	// rows too, at the same token, which is the control the two values sit
	// between.
	for _, src := range append(append([]string{}, refused...), "if false || ; then :; fi") {
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed with no separator skipping at all", src)
		}
	}
}

// A `;` the dialect would not step over, standing where the *next* statement
// of a list begins, was left for whatever the caller wanted a separator
// before — and that took it. `if :; ; then :; fi` parsed in every dialect
// where dash, bash 5.3 and ksh93 all name the second `;`.
func TestASeparatorLeftOverByAListIsRefusedRatherThanAbsorbed(t *testing.T) {
	for _, src := range []string{
		"if :; ; then :; fi",
		"while :; ; do :; done",
		"if :; then : ; ; fi",
		"{ : ; ; }",
		"echo a ; ; echo b",
		"case x in a) : ; ; esac",
		"for i in a; do : ; ; done",
	} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Errorf("%q: %v, want a refusal", src, err)
			continue
		}
		if se.Token != ";" {
			t.Errorf("%q: blamed %q, want %q", src, se.Token, ";")
		}
	}
	// An *empty* list is a different question with a different answer — the
	// dialect that allows an empty body allows `if ; then :; fi` with it —
	// and this must not answer it a second time.
	d := Core()
	d.EmptyCompoundBody = true
	for _, src := range []string{"if ; then :; fi", "while ; do :; done", "{\n}"} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%q: %v, want an empty body taken", src, err)
		}
	}
}
