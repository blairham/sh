// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The output format an arithmetic expression may carry — `$(( [#16] 255 ))` is
// `16#FF`, `$(( [##16] 255 ))` is `FF` — which says the base the *result* is
// written in and how its digits are grouped.
//
// The grammar is [syntax.Dialect.ArithOutputFormat] and the node is
// [syntax.ArithOutput]; what is here is the half that needs the semantics
// vector, which is two things. The alphabet the digits come from and the range
// a base is allowed is `IntegerBaseDigits`, shared with the integer attribute
// because the two spell a base the same way and a script reads one back
// through the other. And whether a negative is a bit pattern or a sign in
// front of the magnitude is `IntegerBaseNegativeIsTwosComplement`, for the
// same reason: `typeset -i16 x=-255` and `$(( [#16] -255 ))` are both `-16#FF`
// in the one shell that has either.

// evalArithOutput evaluates an expression under an output format.
//
// The base is checked *before* the expression is evaluated, which is a choice
// and is measured: `$(( [#37] 1/0 ))` in zsh 5.9.2 complains about the base
// and not about the division, and `$(( 0 ? [#37] 1 : 2 ))` complains about it
// from a branch it never takes. The one arrangement it does not reproduce is
// `$(( 1/0 + [#37] 1 ))`, which that shell answers with the division, because
// it reads and evaluates in one pass and had divided before it reached the
// specifier. Two failures in one expression is the only text that can tell
// them apart, so this takes the reading that is right for the three shapes a
// script can actually have.
func (r *Runner) evalArithOutput(x *syntax.ArithOutput) (arithNum, error) {
	if x.Based && !r.validIntegerBase(x.Base) {
		return intNum(0), arithError{
			msg: Wording(r.diag().IntegerBadBase, "invalid base: %[2]s",
				"", itoa(x.Base)),
			complete: true,
		}
	}
	// Set while the operand is evaluated, so an assignment inside it stores
	// the formatted text, and put back afterwards so nothing survives into the
	// next expression. The caller formats the result from the node rather than
	// from here, which is why this does not have to stay set.
	saved := r.arithOutput
	r.arithOutput = x
	v, err := r.evalNum(x.X)
	r.arithOutput = saved
	return v, err
}

// formatArith is the text an arithmetic answer is written as, which is
// formatNum unless the expression carried an output format.
//
// Two callers, and both are places the *answer* is written down: the text an
// arithmetic expansion produces, and the text an assignment inside the
// expression stores. A subscript is deliberately not one of them — measured,
// `typeset -A m; (( [#16] m[255] = 1 ))` in zsh 5.9.2 stores `16#1` under the
// key `255`, so the format reaches the value and not the key.
func (r *Runner) formatArith(n arithNum) string {
	return r.formatUnder(r.arithOutput, n)
}

// formatUnder is formatArith against a format the caller names, for the result
// of a whole expression: the specifier is at the top of the tree there, and
// the field it is otherwise read from has already been put back.
func (r *Runner) formatUnder(f *syntax.ArithOutput, n arithNum) string {
	if f == nil {
		return r.formatNum(n)
	}
	if !f.Based {
		// A specifier with no base — `[#_]` — changes no base and only
		// groups, so the value is written the way it would have been.
		// Measured: `$(( [#_] 3.5 ))` is `3.5` where `$(( [#10] 3.5 ))` is
		// `3`, so a base that is written truncates even when it is ten.
		return groupNumberText(r.formatNum(n), f.Group)
	}
	v := n.asInt()
	if !r.spellsIntegerBase(f.Base) {
		// Base ten is every dialect's plain decimal and marks nothing, which
		// is the same answer the integer attribute gives it.
		return groupNumberText(itoa(v), f.Group)
	}
	text, ok := r.baseRendered(v, f.Base, f.Prefixed, f.Group)
	if !ok {
		return itoa(v)
	}
	return text
}

// groupNumberText puts the separators into text that is already written in
// decimal, which is the `[#_]` spelling and the base-ten one.
//
// Only the digits before any point are grouped: measured,
// `$(( [#_3] 1234567.5 ))` is `1_234_567.5`.
func groupNumberText(text string, group int) string {
	if group <= 0 {
		return text
	}
	sign := ""
	if rest, ok := strings.CutPrefix(text, "-"); ok {
		sign, text = "-", rest
	}
	head, tail := text, ""
	if at := strings.IndexByte(text, '.'); at >= 0 {
		head, tail = text[:at], text[at:]
	}
	if !isAllDigits(head) {
		// An infinity or a NaN is a name rather than a number, and there is
		// nothing in it to group.
		return sign + text
	}
	return sign + groupDigits(head, group) + tail
}
