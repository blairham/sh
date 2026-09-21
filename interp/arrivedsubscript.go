// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// markArrivedSubscriptQuoting marks the quote characters of a subscript whose
// **brackets arrived already word-expanded**, so that what they hold is read
// as characters of the key rather than as quotation.
//
// One dialect keeps them and two remove them —
// Semantics.ArrivedSubscriptIsAQuotingContext has the rows — and the marks are
// how the keeping is said, because by the time a subscript is read the text is
// the text either way. `(( m[q'r'z] = 42 ))` and `e='m[q'r'z]'; (( $e = 42 ))`
// hand the reader the identical five characters and ksh93 answers `qrz` for
// the first and `q'r'z` for the second, so nothing but provenance can part
// them. syntax.ArithValueMark already means "a byte a value carried, which is
// a character of the key and not syntax", which is exactly the claim, so this
// is that mark put on at a second site rather than a mechanism of its own.
//
// The brackets themselves are left alone, and so is a `$`. A bracket a value
// carried still delimits a subscript here — that is
// Semantics.ArithSubscriptRereadsItsExpandedText's question and it is answered
// elsewhere — and an expansion still standing in the subscript is still
// performed, which is what [Runner.arithSubscriptRead] does one step later.
// Marking either would override an answer that is already given.
//
// That is also why the axis is not consulted again at the key: a subscript
// still holding an expansion is unmarked before it is read a second time, so
// it arrives at quote removal with its quoting intact and is removed exactly
// as bash removes it — which is the measured refinement, that quote removal on
// an arrived subscript happens as part of performing an expansion in it and
// not otherwise.
//
// Two callers, because there are two ways a subscript's brackets arrive
// word-expanded: a builtin operand, which is a whole text of them, and an
// expansion performed at the top of an expression, whose output carried its
// own. One function rather than one each, so the two constructs cannot part —
// the panel does not part them either.
func (r *Runner) markArrivedSubscriptQuoting(text string) string {
	if !arrivedSubscriptCarriesQuoting(text) {
		return text
	}
	if r.sem().SubscriptIsAQuotingContext != Yes {
		// No subscript of this dialect is a quoting context, so an arrived
		// one has no quoting to keep and the question cannot be reached. Not
		// an exemption: it is why zsh's answer to the axis is a pin.
		return text
	}
	if r.ask(r.sem().ArrivedSubscriptIsAQuotingContext,
		"a subscript whose brackets arrived already expanded being a quoting context") {
		return text
	}
	return markArrivedQuoting(text)
}

// arrivedSubscriptCarriesQuoting reports whether text holds a quote character
// or a backslash between brackets, which is the only shape the axis can move.
//
// Asked first so that a subscript with nothing quoted in it — nearly every one
// a script writes — never puts a dialect question, and so that a text with no
// brackets at all never does either.
func arrivedSubscriptCarriesQuoting(text string) bool {
	depth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '\'', '"', '\\', syntax.ArithValueMark:
			if depth > 0 {
				return true
			}
		}
	}
	return false
}

// markArrivedQuoting is that marking, over the bytes between the brackets.
//
// The mark itself is marked where a value really carried one, which is the
// same escape [markArithValue] makes and for the same reason: a NUL that was
// data has to still be one byte of data when the marks come off.
func markArrivedQuoting(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	depth := 0
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case '[':
			depth++
			b.WriteByte(c)
		case ']':
			if depth > 0 {
				depth--
			}
			b.WriteByte(c)
		case '\'', '"', '\\', syntax.ArithValueMark:
			if depth > 0 {
				b.WriteByte(syntax.ArithValueMark)
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
