// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// wordForRun is w divided as this run reads it, which is not always how it was
// read.
//
// Where a `}` ends a `${ … }` is a grammar question — see
// [syntax.Dialect.QuoteProtectsTheClosingBrace] — and one dialect's POSIX mode
// moves the answer. The mode is entered while the shell runs, so a word cut
// under one reading can be expanded under the other, and bash divides it again
// when it expands it rather than keeping the division it made. One function
// body, parsed once before the mode was on, answers `[Vx}y]` and then
// `[Vx}yb'}]` as `set -o posix` moves under it.
//
// What comes back is a *word*, not a span slice, because the word is what the
// rest of expansion carries: [Runner.inWord] records it, `${(q)x}` prints its
// quoting run back out of it, and an index into some other slice than the one
// those two hold is the failure mode this shape avoids. A caller replaces its
// own `w` with what this returns and everything downstream agrees.
//
// The common case returns w itself, after one field read and one comparison —
// the dialects with nowhere to move leave here on the first line that matters,
// and the one that has somewhere pays a nil check per span of the words it
// expands. Nothing re-reads while the run's reading is the one the word was
// cut under, which is every run that never enters or leaves the mode.
//
// The division is bounded by the word and cannot reach past it. That is
// measured rather than assumed: `printf "[%s]" "${v-'a}" x "'}"` is one
// argument in both modes, and a `;` swallowed into the leftover never becomes
// a statement, so no word boundary and no statement boundary moves. See
// [syntax.ParamExpr.RawTail].
//
// One thing the division does that bash's does not, left as it is rather than
// papered over. It reads the text under the run's grammar, so a run the parse
// read as plain text becomes an escape where the mode moved *into* the reading
// that has `$'…'` inside a double-quoted brace at all. Measured 2026-09-23
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` from a script file: a body read
// inside POSIX mode and called outside it — `set -o posix; f(){ printf "[%s]"
// "${u:-x$'\x27'}"; }; set +o posix; f` — answers `[x$'\x27']` in bash 5.3.20
// and `[x']` here, because there was no escape in what bash read and nothing it
// re-divides adds one. The opposite crossing is right, and is the row
// syntax.RereadWordTail's cutUnder parameter carries. Separating the two
// questions means separating the lexer's one gate on
// syntax.Dialect.QuoteProtectsTheClosingBrace, which is where #4169 put the
// construct, and that wants a measurement of its own rather than a change made
// in passing.
func (r *Runner) wordForRun(w *syntax.Word) *syntax.Word {
	if w == nil || r.Dialect == nil ||
		r.Dialect.QuoteProtectsTheClosingBraceInPosixMode == syntax.BraceQuoteUnmovedInPosixMode {
		return w
	}
	reading := r.Dialect.QuoteProtectsTheClosingBrace
	for i := range w.Spans {
		e := w.Spans[i].Param
		if e == nil || e.RawTail == "" {
			continue
		}
		// Two reasons to divide the tail again, and they are one mechanism
		// rather than two: the reading moved under the word, or a `$'…'` in a
		// word operand puts its *value* where the escape was written and the
		// division is read off that. See syntax.RereadWordTail's decode
		// parameter (#4207).
		decode := r.dollarSingleValue(e.RawTail)
		if e.RawTailRead == reading && decode == nil {
			continue
		}
		spans, text, err := syntax.RereadWordTail(e.RawTail, w.Spans[i].Pos, *r.Dialect, e.RawTailRead, decode)
		if err != nil {
			// The scan ran out under the reading this run has — which is a
			// real answer and not a fallback: `"${v-'a}"` parsed as `sh` and
			// expanded after `set +o posix` is a run-time bad substitution in
			// bash, with the two other words on the same line expanding
			// normally. Worded through the same formatter an operand's failed
			// second read uses, because it is the same kind of failure.
			if wording := r.diag().SecondReadingBadSubstitution; wording != "" {
				// The column that reaches this at all words it against the
				// **brace** and names the word as it was written, which is a
				// different sentence from the parse-time one the same shell
				// gives the same text — see
				// Diagnostics.SecondReadingBadSubstitution.
				r.diagf("%s\n", Wording(wording, "", r.wordAsTheScanReadIt(w, e.RawTail, text)))
			} else {
				r.diagf("%s\n", r.diag().ParseFailure(err))
			}
			r.expandErr = true
			return w
		}
		if text == e.RawTail && e.RawTailRead == reading {
			// Nothing moved: the run reads the word the way it was cut, and
			// the escape's value is read the way the escape was written. The
			// second division landed on the first one, so the word the rest of
			// expansion carries stays the one it already had.
			continue
		}
		out := make([]syntax.Span, 0, i+len(spans))
		out = append(out, w.Spans[:i]...)
		out = append(out, spans...)
		// Start and Stop are the word's own and do not move: the re-read
		// divides the text the word already held and never reaches past it.
		return &syntax.Word{Spans: out, Start: w.Start, Stop: w.Stop}
	}
	return w
}

// dollarSingleValue is what a `$'…'` stands for, handed to the second division
// as a function, and nil when tail holds no such run at all.
//
// The cheap test is the whole point of the shape: the conversion is one column's
// and one construct's, so every other word leaves here without a scan. What it
// answers is the escape table, which is this package's — see expandDollarSingle
// — while where a `}` ends an expansion is syntax's, and #4207 is the seam
// between them.
func (r *Runner) dollarSingleValue(tail string) func(string) string {
	if !strings.Contains(tail, "$'") {
		return nil
	}
	return r.expandDollarSingle
}

// wordAsTheScanReadIt is w written out with the text the failing division
// actually read in place of the tail it was cut from.
//
// The two are the same string unless a `$'…'` was converted first, and then
// they differ by exactly the conversion — which is the evidence the re-reading
// happened: bash quotes `"${u:-x'}"` back at a word written `"${u:-x$'\x27'}"`,
// with the escape already gone. The head is what the printed word has in front
// of the tail, so a printer that did not reproduce the word leaves the printed
// word alone rather than pasting a converted tail onto a head nobody checked.
func (r *Runner) wordAsTheScanReadIt(w *syntax.Word, tail, text string) string {
	printed := syntax.PrintWord(w)
	if text == tail {
		return printed
	}
	head := strings.TrimSuffix(printed, tail)
	if head == printed {
		return printed
	}
	return head + text
}
