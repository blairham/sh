// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"strings"

	"github.com/blairham/sh/syntax"
)

// rereadBraceOutput hands the words brace expansion produced back to the rest
// of word expansion as shell **text**, in the dialect that reads them that
// way. See [Semantics.BraceOutputRereadAsText], which holds the measurements.
//
// The whole word is re-read rather than the produced fragment alone, because
// that is where the two readings differ: what a group produced runs *into* the
// text on either side of it, so `$var{x,y}` is the two names `$varx` and
// `$vary` rather than one name with a character behind it.
//
// **The axis is asked only where the two readings part**, which is a small
// minority of the braces anybody writes: `a{b,c}d` is `abd acd` either way,
// because text re-entering a word as characters and text re-entering it as the
// spans it was cut into are the same word wherever the characters are inert.
// So the re-read is computed and compared, and a question whose two answers
// are the same answer never refuses a script.
func (r *Runner) rereadBraceOutput(w *syntax.Word, words []*syntax.Word) []*syntax.Word {
	// A word the braces left alone is the word the parse cut, and re-reading
	// it could only give that word back — which is also what keeps every word
	// in every program that has no brace in it out of this path.
	if len(words) == 1 && words[0] == w {
		return words
	}
	// The run-time switch and the dialect's own answer about braces are read
	// first, for the reason expandWordEscaped reads them first: what brace
	// expansion produced is put back or refused by that outer question, so a
	// re-reading of output nobody is going to use must not be the thing that
	// refuses the script.
	if r.sem().BraceExpansion != Yes || r.noBraceExpand {
		return words
	}
	d := syntax.Dialect{}
	if r.Dialect != nil {
		d = *r.Dialect
	}
	out := make([]*syntax.Word, 0, len(words))
	var failedText string
	var failure error
	for _, bw := range words {
		text := braceOutputText(bw)
		spans, err := syntax.RereadWord(text, w.Start, d)
		if err != nil {
			// A word that will not read at all is a difference by itself:
			// the other reading expands it. The complaint waits for the axis,
			// since a dialect that takes the output as spans never meets it.
			failedText, failure = text, err
			break
		}
		out = append(out, &syntax.Word{Spans: spans, Start: bw.Start, Stop: bw.Stop})
	}
	if failure == nil && sameWords(out, words) {
		return words
	}
	if !r.askBrace(r.sem().BraceOutputRereadAsText,
		"brace expansion's output re-entering the word as shell text") {
		return words
	}
	if failure != nil {
		// The produced text will not read, which is a run-time failure
		// belonging to the word: the line is abandoned at status 1 and the
		// next one runs. The words are handed back in their span form so the
		// caller has words at all; nothing expands them, because expandErr is
		// what a simple command reads to know its word list failed.
		r.refuseBraceOutput(failedText, failure)
		return words
	}
	return out
}

// refuseBraceOutput reports produced text that will not read as a word.
func (r *Runner) refuseBraceOutput(text string, err error) {
	if wording := r.diag().BraceOutputBadSubstitution; wording != "" {
		r.diagf("%s\n", Wording(wording, "", unclosedOpener(text, err), unreadTail(text, err)))
	} else {
		r.diagf("%s\n", r.diag().ParseFailure(err))
	}
	r.expandErr = true
}

// unreadTail is the produced text from the construct that never closed to the
// end of the word, which is what the one column that words this names: for
// `x{Z..a}y` it is the backtick and the `y` behind it, and it is neither the
// whole word nor the opener alone.
func unreadTail(text string, err error) string {
	at := failedAt(err)
	if at < 0 || at >= len(text) {
		return text
	}
	return text[at:]
}

// unclosedOpener is the character that tail opened with, which the same
// sentence names between quotes of its own.
func unclosedOpener(text string, err error) string {
	tail := unreadTail(text, err)
	if tail == "" {
		return ""
	}
	return tail[:1]
}

// failedAt is the offset the re-read gave up at, or -1 when the error carries
// no position in the text.
func failedAt(err error) int {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return -1
	}
	return int(se.Pos.Offset)
}

// sameWords reports whether the two readings produced the same words, which is
// what says the axis has nothing to decide about this one.
//
// Adjacent literal spans of one quoting are a single span for this comparison:
// substituting an alternative between the text on either side of its group
// leaves `a{b,c}d` as three literal spans where re-reading `abd` cuts one, and
// a word divided differently into pieces that join back up is not a different
// word.
func sameWords(a, b []*syntax.Word) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameSpans(joinedLiterals(a[i].Spans), joinedLiterals(b[i].Spans)) {
			return false
		}
	}
	return true
}

// sameSpans compares two span lists by everything an expansion reads off one.
func sameSpans(a, b []syntax.Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.Quoting != y.Quoting || x.Value != y.Value ||
			x.Bare != y.Bare || x.Backquoted != y.Backquoted ||
			x.CurrentShell != y.CurrentShell || x.ReplyValue != y.ReplyValue ||
			x.Translated != y.Translated {
			return false
		}
	}
	return true
}

// joinedLiterals runs adjacent literal spans of one quoting together.
func joinedLiterals(spans []syntax.Span) []syntax.Span {
	out := make([]syntax.Span, 0, len(spans))
	for _, s := range spans {
		if n := len(out); n > 0 && s.Kind == syntax.Literal &&
			out[n-1].Kind == syntax.Literal && out[n-1].Quoting == s.Quoting &&
			out[n-1].Translated == s.Translated {
			out[n-1].Value += s.Value
			continue
		}
		out = append(out, s)
	}
	return out
}

// braceOutputText renders a word brace expansion produced back to the shell
// text it stands for, which is the string the re-read is of.
//
// An unquoted literal span is written **as it stands**, and that is the whole
// mechanism rather than an optimization: [syntax.Span] holds a literal's text
// with no escapes resolved, so an unquoted one is already the source the file
// wrote — and a character a range counted is put in the same way, unescaped,
// which is what makes bash's backslash reach quote removal and its backtick
// open a substitution. Everything else is written back through the printer,
// since a quoted run and a substitution carry delimiters the span no longer
// holds.
//
// The braces of a `${x}` are kept, which is not a nicety: the printer drops a
// pair a name does not need, and `${var}` written back as `$var` beside a
// produced `x` is the name `varx` — the very reading this axis is about.
func braceOutputText(w *syntax.Word) string {
	var b strings.Builder
	var run []syntax.Span
	flush := func() {
		if len(run) > 0 {
			b.WriteString(syntax.PrintWordWith(&syntax.Word{Spans: run}, bracesAsWritten))
			run = nil
		}
	}
	for _, s := range w.Spans {
		if braceable(s) {
			flush()
			b.WriteString(s.Value)
			continue
		}
		run = append(run, s)
	}
	flush()
	text := b.String()
	// The last character of the produced text is read twice over, and both
	// readings are the re-reading shell being permissive about a string it
	// made rather than a file somebody wrote. Measured 2026-09-22 on bash
	// 5.3.20, where `printf '[%s]' ab{Z..a}` answers the eight elements
	// abZ, "ab[", ab, "ab]", ab^, ab_, ab-backtick and aba:
	//
	//   - a **backtick** at the end opens nothing and stays a character,
	//     where the same character with a `y` behind it is a substitution
	//     nothing closes. Escaping it is how that is spelled to a lexer.
	//   - a **backslash** at the end escapes nothing and leaves a *quoted*
	//     empty string, which is the empty element in that row and in
	//     `{Z..a}`'s own. Quoted is the half that matters: `printf '[%s]'
	//     {,}` is a single `[]` in the same shell, because two produced words
	//     with no text at all are no fields at all, where this one is a field
	//     that is empty. Two empty quotes are how that is spelled to a lexer.
	//
	// Nothing but a produced character can be either: one the file wrote is a
	// substitution the parse already took, or an escape the literal span
	// still carries in front of what it escapes.
	if last := len(text) - 1; last >= 0 && !endsEscaped(text[:last]) {
		switch text[last] {
		case '`':
			return text[:last] + "\\`"
		case '\\':
			return text[:last] + "''"
		}
	}
	return text
}

// bracesAsWritten is the one arrangement this rendering needs: the spelling
// the file used, rather than the shortest spelling that parses to the same
// spans.
var bracesAsWritten = syntax.Layout{ParameterBracesAsWritten: true}

// endsEscaped reports whether the character after text would be escaped by a
// backslash run the text ends with — an odd run escapes, an even one is
// backslashes standing for themselves.
func endsEscaped(text string) bool {
	n := 0
	for n < len(text) && text[len(text)-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}
