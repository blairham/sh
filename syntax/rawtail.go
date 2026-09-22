// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// setRawTails records, on every expansion in a finished word that could be
// divided differently by a run in POSIX mode, the source it would be divided
// from. See [ParamExpr.RawTail] for what the run does with it and
// [Dialect.QuoteProtectsTheClosingBraceInPosixMode] for why it has to.
//
// Called from newWord because that is the one place that knows where the
// *word* ends — the same reason [ParamExpr.FlagsErrTail] is set there. An
// operand's sub-word stands at a single position with nothing between its
// ends, so sourceBetween answers nothing for one and a nested expansion
// carries no tail: the re-read is bounded by the word it was cut from, and a
// word inside a word is already inside one.
//
// The gate is deliberately wider than "the two readings differ on this body".
// Answering that exactly costs a second scan of every double-quoted expansion
// in every program, which is what this design was chosen over — see the
// alternate-spans variant recorded on #2604. A tail set where the readings
// agree costs a slice header at parse and a re-read that lands on the same
// spans, and only in the one dialect that has somewhere to move to.
func (p *Parser) setRawTails(spans []Span, stop Pos) {
	if p.dialect.QuoteProtectsTheClosingBraceInPosixMode == BraceQuoteUnmovedInPosixMode {
		return
	}
	// A tail being divided again has no tail of its own to keep. Its spans
	// carry offsets into that tail rather than into the program, so `stop`
	// names a position in some other string — and sourceBetween's guards
	// catch most of those pairs and not all of them, which is a slice of the
	// wrong text rather than nothing. The re-read word is thrown away when
	// the expansion ends either way, so there is nothing here to record.
	if p.lex.inWordTail {
		return
	}
	for i := range spans {
		s := &spans[i]
		// Inside double quotes only: written without them the quote protects
		// in every column, so there is nothing for the mode to move. The
		// braced form only, because the reading is about where a `}` ends the
		// expansion and the bare form has no brace to end at.
		if s.Kind != ParamExp || s.Param == nil || s.Bare ||
			s.Quoting != DoubleQuoted || !strings.ContainsRune(s.Value, '\'') {
			continue
		}
		tail := p.sourceBetween(s.Pos, stop)
		if tail == "" {
			continue
		}
		s.Param.RawTail = tail
		s.Param.RawTailRead = p.dialect.QuoteProtectsTheClosingBrace
	}
}

// RereadWordTail divides the tail of a word again, under d's reading of
// [Dialect.QuoteProtectsTheClosingBrace].
//
// tail is a [ParamExpr.RawTail]: the source from an expansion's `${` to the
// end of the word it stands in. The spans it returns replace the word's from
// that expansion onward. Everything in front of it is untouched, and is
// untouchable — the two readings scan identically until the first single quote
// inside the braces, which is the first place they can disagree.
//
// It is read as double-quoted **content** rather than as the rest of a word,
// and that is measured rather than convenient. `printf "[%s]" "${v-'a}" x
// "'}"` is one argument in bash under both modes, and `"${v-'a}"; echo
// MIDDLE; :"'}"` is the one argument `V; echo MIDDLE; :'}` with `MIDDLE`
// never printing. So the leftover characters are neither re-tokenized nor
// field-split: the quotes around them are removed, a `'` among them is an
// ordinary character, and a `;` among them is text. A word's extent and a
// statement's are facts about the parse in bash too; only the division inside
// one already-cut word belongs to the run.
//
// Which makes the `"` that closed the word part of this text, with nothing
// left to close — see [Lexer.inWordTail], which is what says that running out
// of a word's own tail is the word ending rather than an unmatched quote.
//
// An error is the scan running out of an expansion — `"${v-'$('}"` never
// closes its `${` under the reading where the quote does not protect — and it
// belongs to the *run* rather than to the file, because the file read cleanly
// under the reading it was parsed with. bash answers it the same way, with a
// run-time `bad substitution` at the word rather than a syntax error at the
// line (#2604).
func RereadWordTail(tail string, at Pos, d Dialect) ([]Span, error) {
	p := &Parser{lex: NewLexer(tail, d), dialect: d}
	p.lex.inWordTail = true
	// dquoteEscapes and not operandEscapes: the text was scanned with the
	// word's own escape set the first time, and the second reading moves
	// where a brace ends an expansion and nothing else. Handing it the
	// operand's wider set would make `\}` an escape in text that was read as
	// two characters.
	spans := p.lex.scanDoubleEscaping(at, false, dquoteEscapeSet)
	if err := p.lex.Err(); err != nil {
		return nil, err
	}
	// Through newWord, so the expansions the second division produced are
	// parsed exactly as a first one's are.
	w := p.newWord(spans, at, at)
	if p.err != nil {
		return nil, p.err
	}
	return w.Spans, nil
}
