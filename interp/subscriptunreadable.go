// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"strings"

	"github.com/blairham/sh/syntax"
)

// subscriptExpressionBeforeAnUnreadableByte is the part of a subscript's text
// in front of the first byte the arithmetic has no reading for, in the dialect
// where such a byte ends the expression. See
// Semantics.SubscriptExpressionStopsAtAnUnreadableByte.
//
// Nothing but blanks in front of the byte is the operand expected *at* it,
// which is the reference's sentence: `'2'` is `operand expected at `'2”`.
func (r *Runner) subscriptExpressionBeforeAnUnreadableByte(text string) (string, error) {
	at := unreadableArithByte(text)
	if at < 0 {
		return text, nil
	}
	if strings.TrimSpace(text[:at]) == "" {
		// Refused under either answer, so nothing is asked: what the
		// dialect that stops here adds is only its sentence, and a dialect
		// that has not chosen leaves the byte to the arithmetic's own.
		if r.sem().SubscriptExpressionStopsAtAnUnreadableByte == Yes {
			return "", &syntax.Error{Kind: syntax.ErrArithOperand, Expr: text, Token: text[at:]}
		}
		return text, nil
	}
	if !r.ask(r.sem().SubscriptExpressionStopsAtAnUnreadableByte,
		"a subscript's expression ending at a byte the arithmetic cannot read") {
		return text, nil
	}
	return text[:at], nil
}

// operandExpectedAtTheRest rewords an expression that ran out at the byte
// that ended it: what it ran out *at* is the text the byte began, so `1+'2'`
// is `operand expected at `'2”` and `1*@` is `operand expected at `@'`. Any
// other failure — a parenthesis left open is still `')' expected` — stands.
func operandExpectedAtTheRest(err error, text, expr string) error {
	var se *syntax.Error
	if !errors.As(err, &se) || se.Kind != syntax.ErrArithOperandEnd {
		return err
	}
	return &syntax.Error{Kind: syntax.ErrArithOperand, Expr: text, Token: text[len(expr):]}
}

// unreadableArithByte is where the first byte no arithmetic token can begin
// or continue stands in text, or -1. Letters, digits, the underscore, blanks,
// the operators and brackets, `#` and `.` of a numeral, `$` and a double
// quote are all read by something; what is left is an apostrophe, a
// backslash, `@`, a backquote and the braces, among the rest of the
// punctuation, and every byte past ASCII.
func unreadableArithByte(text string) int {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_', c == ' ', c == '\t', c == '\n':
		case c == '"':
		default:
			if !isArithPunctuation(c) {
				return i
			}
		}
	}
	return -1
}

func isArithPunctuation(c byte) bool {
	switch c {
	case '+', '-', '*', '/', '%', '^', '&', '|', '<', '>', '=', '!', '~',
		'?', ':', '(', ')', '[', ']', ',', ';', '.', '#', '$':
		return true
	}
	return false
}
