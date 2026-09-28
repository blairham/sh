// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A `${` the input runs out of ends **at the end of the input** where the
// grammar says so, once its operand has begun — and is unfinished input where
// it does not.
//
// [Dialect.UnterminatedExpansionOperandIsAValue] is the whole of the
// difference and one column has it; that flag's own comment carries the
// eleven measured rows and the two controls.
//
// The lexer is driven on its own here because the **quoted** spelling needs
// the driver's reading of an unterminated double quote as well: `echo "abc` is
// a word through the shipped binary and a refusal through a bare Runner, so a
// behavioral harness cannot express the quoted grid. What it can express is
// that the expansion no longer refuses and that the remainder arrived as its
// text, which is what these assert.
func TestAnUnterminatedExpansionOperandEndsAtTheEndOfTheInput(t *testing.T) {
	t.Parallel()
	operand := func() Dialect {
		d := Core()
		d.ParamSubstitution = true
		d.BareBraceNestsInAQuotedPatternOperand = true
		d.UnterminatedExpansionOperandIsAValue = true
		// The quoted rows below need this as well, and that is the shape of
		// the measurement rather than scaffolding: every probe is `-c`, and
		// the expansion swallows the closing quote, so the word is left
		// holding an unterminated one. The shell that runs these closes a
		// quote at the end of a command string — `echo "abc` is `abc` there
		// and here — and the two readings have to be in force together for
		// the line to run at all. See [Dialect.CloseQuotesAtEOF].
		d.CloseQuotesAtEOF = RouteFromCommandString
		d.ProgramRoute = RouteFromCommandString
		return d
	}()
	for _, c := range []struct {
		name, src, want string
	}{
		// Unquoted, one row per operator family. None holds a `{`, which is
		// what says the reading is not the bare brace #4936 gave a way to
		// open a level.
		{"a prefix trim", `echo ${s#x`, "s#x"},
		{"a suffix trim", `echo ${s%x`, "s%x"},
		{"a substitution", `echo ${s/x/y`, "s/x/y"},
		{"a value operand", `echo ${s:-x`, "s:-x"},

		// Quoted, which is the shape #4973 reports: the brace opened a level
		// and the remainder — the `]`, the closing quote and the rest of the
		// line — became the operand.
		{"a bare brace in a quoted operand", `echo "[${s#{}]"`, `s#{}]"`},
		{"one behind text", `echo "[${s#a{}]"`, `s#a{}]"`},
		{"one with a body", `echo "[${s#{x}]"`, `s#{x}]"`},
		{
			"a replacement that took the rest of the line",
			`echo "[${a/x/{y}]"; echo AFTER`, `a/x/{y}]"; echo AFTER`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := NewLexer(c.src, operand)
			got := render(l.Tokens())
			if l.Err() != nil {
				t.Fatalf("refused with %v, want the expansion to end at the input", l.Err())
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("tokens = %s, want %s in them", got, c.want)
			}
		})
	}
}

// And the two falsifying halves, because the test above would pass against a
// lexer that had simply stopped refusing.
//
// The first is the flag off: the same input under the same grammar is still
// unfinished input. The second is the flag **on** with no operand begun, which
// is the row that fixes the rule — it is not "input that ran out inside an
// expansion is a value", it is that an operand consumes to the end of the
// input and so no character can be unexpected. The reference refuses `${s`
// and `${#s` too.
func TestAnUnterminatedExpansionIsUnfinishedInputWithoutAnOperand(t *testing.T) {
	t.Parallel()
	off := func() Dialect {
		d := Core()
		d.ParamSubstitution = true
		d.BareBraceNestsInAQuotedPatternOperand = true
		return d
	}()
	on := func() Dialect {
		d := off
		d.UnterminatedExpansionOperandIsAValue = true
		return d
	}()
	for _, c := range []struct {
		name, src string
		d         Dialect
	}{
		{"the flag off, a trim", `echo ${s#x`, off},
		{"the flag off, a quoted brace", `echo "[${s#{}]"`, off},
		{"the flag on, a bare name", `echo ${s`, on},
		{"the flag on, a length", `echo ${#s`, on},
		{"the flag on, a bare name quoted", `echo "${s`, on},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := NewLexer(c.src, c.d)
			l.Tokens()
			if l.Err() == nil {
				t.Errorf("%s: read as a word, want unfinished input", c.src)
			}
		})
	}
}
