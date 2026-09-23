// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// QuotedRunBoundary says whether the span at i begins a new pair of quotes
// rather than continuing the pair the span in front of it was in — and, in
// `known`, whether that could be worked out at all.
//
// It is the difference between `"$e$@"` and `"$e""$@"`, and both a run and a
// printer need it. Those two words are the **same two spans** — a span records
// the quoting it sits in and not where one run of that quoting ends — so the
// boundary is read off the **positions** instead: spans inside one pair of
// quotes are contiguous in the source, and a gap between two of them is the
// closing quote and the opening one.
//
// Two callers, and each was wrong without it. A shell that reads what an empty
// `"$@"` takes with it is reading one quoted *string* rather than the word, so
// the string the list is not in is a quoted null that survives —
// interp.Semantics.EmptyListTakesTheWord, where bash 5.3 answers `"$e$@"` with
// no argument at all and `"$e""$@"` with one. And a printer that joined the two
// runs wrote `"$e""$@"` back as `"$e$@"`, which is that different program
// (#4203).
//
// Written this way rather than as a flag on [Span] because that flag would be a
// byte on a struct whose size is a committed budget rather than a free choice —
// see `syntax/nodesize_test.go`, where raising it is priced at 185KB of resident
// memory per byte and called a decision rather than a fix.
//
// **The two callers want opposite answers where it cannot be worked out**,
// which is why `known` is a result rather than a default taken here. A run
// keeps the word, so *not knowing* has to read as a boundary there: that is
// the answer every column gives, and the worst it costs is a fix not applied.
// A printer writing an extra pair of quotes is a different program, so not
// knowing has to read as no boundary there — which is what it wrote before
// this existed. Only an extent computed too *long* could be wrong either way,
// and the lengths below are exact or short by construction.
func QuotedRunBoundary(spans []Span, i int) (opens, known bool) {
	if i <= 0 || i >= len(spans) {
		// Nothing in front of it, so there is no earlier run to have ended,
		// and that is a fact rather than a gap in what can be measured.
		return false, true
	}
	end, ok := spanSourceEnd(spans[i-1])
	if !ok {
		return false, false
	}
	return spans[i].Pos.Offset > end, true
}

// spanSourceEnd is where a span's source text ends, for the one kind whose
// extent follows from what it holds. See QuotedRunBoundary for why anything
// else answers that it is not known rather than guessing.
//
// A **literal** is deliberately not that kind, and it looked like the easy one.
// Its value is not always its source: a quoted literal's position is the quote
// rather than the text inside it, and a literal read inside a backquote body
// has had that body's escapes resolved, so “ "`echo \\`echo n\\“" “ measures
// two bytes short and a printer trusting it wrote a quote pair that was never
// there. What is left is the kind the axis needs anyway — a list stands beside
// an expansion — and a literal beside a list keeps the word whatever run it is
// in, so nothing is lost by not measuring one.
func spanSourceEnd(s Span) (int32, bool) {
	switch s.Kind {
	case ParamExp:
		// `$name` is the sigil and the name; `${ … }` is the braces and
		// everything between them, which is what a span's value holds for this
		// kind. A subscript the word carried away is not in the value, so a
		// bare `$a[0]` measures short, which reads as a boundary.
		if s.Bare {
			return s.Pos.Offset + int32(1+len(s.Value)), true
		}
		return s.Pos.Offset + int32(3+len(s.Value)), true
	}
	return 0, false
}
