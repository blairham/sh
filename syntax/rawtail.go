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
//
// cutUnder is the reading the word was cut with — [ParamExpr.RawTailRead] —
// and decode is what a `$'…'` written in one of the tail's **word** operands
// stands for, and nil where the run has no such conversion to offer. It is the
// second thing the one column that reaches here does to an already-cut word,
// and it is a callback because the two halves live in two packages: the escape
// table belongs to the run — interp's expandDollarSingle applies it — and the
// scan for the closing brace belongs here. bash puts the escape's *value* back
// into the operand's text and reads the division off that, so a value that
// happens to be a quote character hides the `}` behind it and a value that
// holds a `}` ends the expansion early (#4207).
//
// A pattern or a replacement operand is deliberately not converted, and that
// is measured rather than a simplification. Under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` from a script file, bash 5.3.20 and bash 3.2 both answer `[x]` to
// `v="'"; printf '[%s]' "${v/$'\x27'/x}"` and `[']` to `v=Q; printf '[%s]'
// "${v/Q/$'\x27'}"` — the produced quote does not reach the scan there —
// while the word operand of the same expansion refuses. The split is the one
// Lexer.braceOperandIsAPattern makes, read off the parsed operator instead of
// off the text a scan has consumed; the two cannot be one function, because
// that one runs before there is an operator to read.
//
// What comes back beside the spans is the text the division was read from,
// which is the tail itself when nothing was converted. The one column that
// reaches the failure quotes that text back in its diagnostic — with the
// escape already converted, which is the evidence the re-reading is happening
// at all — so the caller needs it rather than the word as written.
//
// Positions inside a converted tail are the honest ones the text has and not
// the ones the file has: an escape is longer than its value, so everything
// behind the first conversion sits that much earlier than it was written. The
// failure this reaches names the word rather than a column, so nothing yet
// reads one; a reader of the returned spans' positions is reading offsets into
// the returned text.
func RereadWordTail(tail string, at Pos, d Dialect, cutUnder BraceQuotePolicy, decode func(string) string) ([]Span, string, error) {
	if decode == nil {
		spans, err := readWordTail(tail, at, d)
		return spans, tail, err
	}
	// Located under the reading the word was *cut* with and divided under the
	// one the run has, which is two readings of one word and is measured. With
	// `unset u` and a function body holding `printf "[%s]" "${u:-x$'\x27'}"`,
	// bash 5.3.20 answers `[x']` when the body is read outside POSIX mode and
	// called inside it — the escape it read is converted, and the `'` the
	// conversion produced then fails to protect the brace the mode has moved.
	// Reading the body inside the mode answers `[x$'\x27']` instead: there was
	// no escape there to convert. So which runs the operand holds belongs to
	// the parse and where the `}` lands belongs to the run.
	located := d
	located.QuoteProtectsTheClosingBrace = cutUnder
	spans, err := readWordTail(tail, at, located)
	if err != nil {
		return spans, tail, err
	}
	text := dollarSingleValuesInWordOperands(tail, spans, decode)
	if text == tail {
		// Nothing converted, so the division the run wants is the one it would
		// have read without any of this.
		spans, err = readWordTail(tail, at, d)
		return spans, tail, err
	}
	spans, err = readWordTail(text, at, d)
	return spans, text, err
}

// dollarSingleValuesInWordOperands is tail with every `$'…'` standing in the
// **word** operand of the expansion tail opens with replaced by what decode
// says it stands for.
//
// The operand is bounded to that one expansion's own, which is the bound
// setRawTails already draws and for its reason: a nested expansion carries no
// tail, because a word inside a word is already inside one. A `$'…'` written
// past the closing `}` is not converted either — it is the enclosing word's
// text rather than an operand's, and one written inside double quotes is a
// dollar and a quote there in every column.
//
// Where the operand *starts* is the one thing neither the spans nor the body
// says outright: an operand's sub-word is parsed on its own, so its spans count
// from the operand rather than from the program, and reading the offset off the
// body would mean a second copy of the parser's operator table — the table that
// knows `::=` is three bytes and `:^^` is three others. So the offset is found
// instead of computed: the one shift at which every `$'…'` the operand holds
// lands on its own text, byte for byte. A body no shift explains is left alone,
// which is the honest answer for a node this package did not parse.
func dollarSingleValuesInWordOperands(tail string, spans []Span, decode func(string) string) string {
	if len(spans) == 0 || spans[0].Kind != ParamExp || spans[0].Param == nil {
		return tail
	}
	e := spans[0].Param
	if !takesAWordOperand(e.Op) || e.Arg == nil {
		return tail
	}
	// The tail opens with the `${` the body stands inside, which is what makes
	// the body a slice of it rather than a string to go looking for.
	const braces = len("${")
	if len(tail) < braces+len(spans[0].Value) {
		return tail
	}
	body := tail[braces : braces+len(spans[0].Value)]
	if body != spans[0].Value {
		return tail
	}
	type run struct {
		off   int
		value string
	}
	var want []run
	for _, a := range e.Arg.Spans {
		if a.Kind == Literal && a.Quoting == DollarSingleQuoted {
			want = append(want, run{int(a.Pos.Offset), a.Value})
		}
	}
	if len(want) == 0 {
		return tail
	}
	written := func(r run) string { return "$'" + r.value + "'" }
	start := -1
	for k := 0; k+want[len(want)-1].off <= len(body); k++ {
		lands := true
		for _, r := range want {
			at, text := k+r.off, written(r)
			if at+len(text) > len(body) || body[at:at+len(text)] != text {
				lands = false
				break
			}
		}
		if lands {
			start = k
			break
		}
	}
	if start < 0 {
		return tail
	}
	var b strings.Builder
	b.WriteString(tail[:braces])
	cut := 0
	for _, r := range want {
		at := start + r.off
		if at < cut {
			// Two runs claiming the same bytes is not a shape the parser
			// produces. Dropping the second keeps this a rewrite of the body
			// rather than a scramble of it if one ever does.
			continue
		}
		b.WriteString(body[cut:at])
		b.WriteString(decode(r.value))
		cut = at + len(written(r))
	}
	b.WriteString(body[cut:])
	b.WriteString(tail[braces+len(body):])
	return b.String()
}

// takesAWordOperand reports whether op's operand is read in the quoting that
// encloses the whole expansion rather than on its own terms. See
// [Lexer.braceOperandIsAPattern], which is the same split made on the text a
// scan has consumed so far.
func takesAWordOperand(op ParamOp) bool {
	switch op {
	case ParamDefault, ParamAssign, ParamAssignAlways, ParamError, ParamAlternate:
		return true
	}
	return false
}

// readWordTail is one reading of text as a word's tail, which is the whole of
// [RereadWordTail] where nothing is converted first.
func readWordTail(tail string, at Pos, d Dialect) ([]Span, error) {
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
