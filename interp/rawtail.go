// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

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
func (r *Runner) wordForRun(w *syntax.Word) *syntax.Word {
	if w == nil || r.Dialect == nil ||
		r.Dialect.QuoteProtectsTheClosingBraceInPosixMode == syntax.BraceQuoteUnmovedInPosixMode {
		return w
	}
	reading := r.Dialect.QuoteProtectsTheClosingBrace
	for i := range w.Spans {
		e := w.Spans[i].Param
		if e == nil || e.RawTail == "" || e.RawTailRead == reading {
			continue
		}
		spans, err := syntax.RereadWordTail(e.RawTail, w.Spans[i].Pos, *r.Dialect)
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
				r.diagf("%s\n", Wording(wording, "", syntax.PrintWord(w)))
			} else {
				r.diagf("%s\n", r.diag().ParseFailure(err))
			}
			r.expandErr = true
			return w
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
