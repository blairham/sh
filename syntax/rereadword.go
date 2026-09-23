// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// RereadWord reads text as the source of one word, for a run that has to read
// a word it *produced* rather than one the file held.
//
// It is the second of the two re-readings a run can be asked for, and it is
// the wider one. [RereadWordTail] divides a word the file holds a second time,
// under the other reading of where a `}` ends an expansion; this reads text
// the file never held at all — what brace expansion produced, in the one shell
// that hands its output back to word expansion as ordinary shell text. So the
// `x` that `$var{x,y}` produced continues the parameter's name there, and the
// backslash a character range counted is an unquoted backslash like any other.
// See interp.Semantics.BraceOutputRereadAsText, which is the axis, and holds
// the measurements.
//
// The whole of text is one word by construction — it is what one word of the
// program came to — so an operator in it is a character of the word rather
// than a token, and what a word scan stops at is put back as literal text.
// That is the rule an expansion's operand is read under too, for the same
// reason: everything in the text belongs to the one word it was cut from.
//
// An error is the **run's** rather than the file's, exactly as
// [RereadWordTail]'s is: the program read cleanly under the characters it
// actually holds, and it is what the produced text spells that will not read.
// `x{Z..a}y` is the worked example — the backtick the range counted opens a
// command substitution that the `y` behind it never closes, and bash answers
// it with a run-time `bad substitution` at status 1 with the next line still
// running.
func RereadWord(text string, at Pos, d Dialect) ([]Span, error) {
	p := &Parser{lex: NewLexer(text, d), dialect: d}
	w := &Word{Start: at, Stop: at}
	// Whatever the lexer stepped over between two tokens is text here rather
	// than a separator, and the gap is put back by offset rather than guessed
	// at. See Parser.wordFrom, which puts an operand's back the same way.
	last := 0
	gap := func(upto int) {
		if upto > last && last >= 0 && upto <= len(text) {
			w.Spans = append(w.Spans, Span{Kind: Literal, Value: text[last:upto], Pos: at})
		}
	}
	for {
		t := p.lex.Next()
		if err := p.lex.Err(); err != nil {
			return nil, err
		}
		if t.Kind == TokEOF {
			gap(len(text))
			break
		}
		gap(int(t.Pos.Offset))
		last = int(t.End.Offset)
		if t.Kind != TokWord {
			w.Spans = append(w.Spans, Span{Kind: Literal, Value: t.Text, Pos: at})
			continue
		}
		// Through newWord, so the expansions the second reading produced are
		// parsed exactly as a first one's are — which is the whole point of
		// the re-read, since `$varx` is a different name from `$var`.
		nested := p.newWord(placeLinesIn(t.Spans, at), at, at)
		if p.err != nil {
			return nil, p.err
		}
		w.Spans = append(w.Spans, nested.Spans...)
	}
	return w.Spans, nil
}
