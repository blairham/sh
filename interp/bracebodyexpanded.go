// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// alternativesAfterExpansion is the reading where a group's **body** is read
// once its own expansions have run, so a comma an expansion produced is a
// list separator and not a character. See
// [Semantics.BraceBodyReadAfterExpansion], which holds the measurements.
//
// The group's *delimiters* are still the written braces — this is asked only
// once findBraceFrom and matchBraceAcross have already matched a pair — and
// so is every brace the body itself was written with. What the expansions
// produced is neither: its commas divide the body and everything else in it
// is inert, which is the third kind of span this reading needs and the reason
// it cannot be had by expanding the body and reading the text back whole.
//
// Three results rather than two. The third says the group was **abandoned**:
// the produced text holds a `{`, which this reading does not carry, and the
// word is left exactly as it was written. See
// producedBraceAbandonsTheGroup.
//
// Asked only where the two readings can part, which is a body holding
// something to expand: `{a,b}` is a list under both and must not be turned
// into a question. And only on the word road — `endpoints` is false for the
// count a redirection target takes, which is the same gate
// [Semantics.BraceRangeEndpointsExpanded] stands behind and the same measured
// answer, since the one column that reads a body this way brace-expands an
// argument and not a target.
func (r *Runner) alternativesAfterExpansion(w *syntax.Word, open, close cursor, endpoints bool) ([][]syntax.Span, bool, bool) {
	if !endpoints || r.sem().BraceExpansion != Yes || r.noBraceExpand {
		return nil, false, false
	}
	answer := r.sem().BraceBodyReadAfterExpansion
	if answer == No {
		// The common answer, and the one that must cost nothing: three of
		// the four columns that expand braces at all find them in the word
		// the parse cut.
		return nil, false, false
	}
	body := sliceSpans(w.Spans, next(open), close)
	if !bodyHoldsAnExpansion(body) {
		return nil, false, false
	}
	resolved, inert, ok := r.resolveBraceBody(w, body)
	if answer != Yes {
		// Unanswered, and this is the one state the inert test is for. The
		// two readings give the same words wherever nothing an expansion
		// produced is brace syntax, a pattern, a separator or anything else
		// a later stage reads — `a=1; echo {$a,2}` is `1 2` either way — so
		// a vector that has not chosen must expand it rather than refuse it.
		// Where they *can* part, the axis refuses by name.
		//
		// The body is resolved either way, so its expansions have run once
		// by the time the question is put. That is deliberate and is the
		// same choice braceFanRepeatsTheWork makes: a refusal abandons the
		// word, and running the work once before refusing is better than
		// running it once to compare and again to use.
		if !ok || !inert {
			r.askBrace(answer, braceBodyAxis)
			return nil, false, false
		}
		return r.braceBodyAlternatives(w, open, resolved)
	}
	if !ok {
		return nil, false, true
	}
	return r.braceBodyAlternatives(w, open, resolved)
}

// braceBodyAxis is the axis's name where a refusal has to say it.
const braceBodyAxis = "a brace group's body being read after the expansions written in it"

// bodyHoldsAnExpansion reports whether a group's body has anything to run.
//
// A body of literals alone — quoted or not — reads the same way under both
// answers, since there is no produced text for the two to disagree about:
// `x{'a b',c}` is `xa b xc` in every column that expands braces. The gate is
// the span's *kind* rather than whether it is brace syntax, because a quoted
// literal is written text that stands as the parse cut it.
func bodyHoldsAnExpansion(body []syntax.Span) bool {
	for _, s := range body {
		if s.Kind != syntax.Literal {
			return true
		}
	}
	return false
}

// braceBodyAlternatives splits a resolved body, or says what the group comes
// to where it holds no alternative at all.
func (r *Runner) braceBodyAlternatives(w *syntax.Word, open cursor, resolved []syntax.Span) ([][]syntax.Span, bool, bool) {
	if alts, ok := alternativesInBody(resolved); ok {
		return alts, true, false
	}
	// No comma anywhere in the resolved body, written or produced, so the
	// body is a range or it is nothing. Both are read off the text the
	// expansions came to, which is the same question
	// BraceRangeEndpointsExpanded is asked about a range whose endpoints are
	// written as expansions — put here as well, so that the axis still
	// decides a range's ordering in a dialect that reads bodies this way.
	pos := w.Spans[open.span].Pos
	text := resolvedBraceText(resolved)
	if r.askBrace(r.sem().BraceRangeEndpointsExpanded,
		"a brace range's endpoints expanding before the range is read") {
		alts, outcome := r.braceRange(text)
		switch outcome {
		case braceRangeCounted:
			return r.rangeSpans(alts, pos), true, false
		case braceRangeCollapsed:
			return collapsedRange(text, pos), true, false
		}
	}
	// A body that is neither a list nor a range still had its expansions
	// run, and what they left does not go back into the word as text a later
	// stage may split or match: `e="a b"; echo {$e}` is the single field
	// `{a b}` where `echo x$e` is two, and `e="*"; echo {$e}` is `{*}` with
	// the directory full of matches. So the group comes back as the one
	// inert alternative the braces still stand around, exactly as a range
	// that failed to form does above.
	return [][]syntax.Span{{{
		Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
		Value: "{" + text + "}", Pos: pos,
	}}}, true, false
}

// resolveBraceBody runs the body's expansions once and puts what they
// produced back beside the written spans, as the two kinds of span this
// reading splits over.
//
// A **written** span stands as the parse cut it, whether it is brace syntax
// or behind quotes — so a written `{a,b}` nested in the body is still a group
// and a written `"b,c"` is still one alternative. What an expansion
// **produced** is cut into a span per comma, which is structural, and an
// inert span for everything between them, which is neither split nor matched
// nor expanded again: `e="a b,c"; echo {$e}` is the two fields `a b` and `c`,
// and `e="a*,z"` is `a* z` where the written `{a*,z}` is `aa ab z`.
//
// The provenance is per character rather than per alternative, which is
// measured: with `e=a`, `{p,$e*}` is `p aa ab` — the `*` is written, so it
// matches — and with `e=*`, `{p,a$e}` is `p a*`, where the same two
// characters came the other way round.
//
// The second result says every produced run means the same thing under both
// readings — see producedRunIsInert, which is what keeps an unanswered vector
// from refusing `a=1; echo {$a,2}`. The third is false where the produced
// text holds a `{`; see producedBraceAbandonsTheGroup.
func (r *Runner) resolveBraceBody(w *syntax.Word, body []syntax.Span) ([]syntax.Span, bool, bool) {
	out := make([]syntax.Span, 0, len(body))
	inert := true
	for _, s := range body {
		if s.Kind == syntax.Literal {
			out = append(out, s)
			continue
		}
		// One span at a time rather than the run they sit in, so that
		// nothing about the sub-word changes what an expansion comes to:
		// every span here is an expansion rather than a literal, so no
		// sub-word can open with a `~` that the body as written did not
		// open with, and splitting is off on this road either way.
		text := strings.Join(r.expandWordNoSplit(&syntax.Word{
			Spans: []syntax.Span{s}, Start: w.Start, Stop: w.Stop,
		}), "")
		if producedBraceAbandonsTheGroup(text) {
			return nil, false, false
		}
		if !r.producedRunIsInert(text) {
			inert = false
		}
		out = append(out, producedBraceSpans(text, s.Pos)...)
	}
	return out, inert, true
}

// producedRunIsInert reports whether text an expansion produced means the
// same thing under both readings of a group's body.
//
// It is the comparison [Runner.rereadBraceOutput] makes for the other end of
// brace expansion, put where this end can afford it: a vector that has not
// answered the axis must expand a word the two readings agree about rather
// than refuse it, and most bodies holding an expansion are such a word —
// `a=1; echo {$a,2}` is `1 2` whichever way the body is read.
//
// The set is written out rather than its complement, for the reason
// inertInAWord's is: a character nobody has thought about lands on the side
// that asks the question. Four kinds of character are out on their own:
//
//   - a comma and a brace, which are the syntax this axis is about;
//   - a `.`, because two of them are a range and a range is read off the
//     resolved text under one reading and off the written spans under the
//     other — `e=..3; echo {1$e}` is `1 2 3` one way and `{1..3}` the other;
//   - anything the current IFS would split on, since the produced text is a
//     field of its own under one reading and inert data under the other.
//
// Everything left is [inertInAWord]'s set, which already excludes a pattern's
// metacharacters, a quote, a `$` and a tilde.
func (r *Runner) producedRunIsInert(text string) bool {
	if strings.ContainsAny(text, ",{}.") {
		return false
	}
	ifs, set := r.ifs()
	if !set {
		ifs = " \t\n"
	}
	for i := 0; i < len(text); i++ {
		if strings.IndexByte(ifs, text[i]) >= 0 {
			return false
		}
	}
	for _, c := range text {
		if !inertInAWord(c) {
			return false
		}
	}
	return true
}

// producedBraceAbandonsTheGroup is why a produced `{` stops this reading
// rather than being one more inert character, and why it takes the rest of
// the word with it.
//
// Measured 2026-09-27 against `/bin/ksh` — `Version AJM 93u+ 2012-08-01` —
// each case in a directory of its own holding `aa`, `ab` and `zz`:
//
//	e="}"      echo {a,b$e,c}    a b} c          a produced `}` is a character
//	e="{"      echo {a,b$e,c}    {a,b{,c}        and a produced `{` is not
//	e="{z}"    echo {a,b$e,c}    {a,b{z},c}      balanced, and still not
//	e="{z,y}"  echo {a,b$e,c}    {a,b{z,y},c}
//	e="}x{"    echo {a,b$e,c}    {a,b}x{,c}
//	e="a{b"    echo {x,$e}       {x,a{b}
//	e="{z,y}"  echo {$e}         {{z,y}}
//
// So it is the `{` itself and not the balance: a group whose produced text
// holds one is left as it was written, commas and all. The mirror rows say
// the note is on the **body** and on nothing else — with `e="{"`,
// `echo {a,b}$e` is `a{ b{` and `echo {a,b}{c,d$e}` is `a{c,d{} b{c,d{}`,
// both groups read and only the one with the produced brace abandoned.
//
// And it ends the word's scan rather than only its group, which is the row
// that says so: `echo {c,d$e}{a,b}` is the single field `{c,d{}{a,b}`, where
// a scan that carried on past the abandoned group would have found the
// `{a,b}` behind it and made two. That is the same shape
// [Semantics.BraceRescanEntersFailedGroup] already gives this column for a
// group that was never closed.
//
// This is written here rather than in the issue because the two shapes at the
// top of it were reported as fitting no reading at all (#4703), and what they
// fit is this one predicate.
func producedBraceAbandonsTheGroup(text string) bool {
	return strings.ContainsRune(text, '{')
}

// producedBraceSpans cuts text an expansion produced into the spans the split
// reads: a comma of its own, which is brace syntax, and an inert run for
// everything between them.
//
// Single-quoted for the inert runs, which is the same spelling
// [Runner.rangeSpans] uses for a counted range's elements and means the same
// thing — data in a span rather than text the word still has to read. An
// empty produced value therefore leaves no span at all rather than an empty
// quoted one: an alternative with nothing in it is the same alternative
// however it came to be empty, and which of those the shells keep is a
// question of its own that this reading must not answer by accident.
func producedBraceSpans(text string, pos syntax.Pos) []syntax.Span {
	var out []syntax.Span
	for {
		i := strings.IndexByte(text, ',')
		if i < 0 {
			break
		}
		if i > 0 {
			out = append(out, syntax.Span{
				Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
				Value: text[:i], Pos: pos,
			})
		}
		out = append(out, syntax.Span{
			Kind: syntax.Literal, Quoting: syntax.Unquoted, Value: ",", Pos: pos,
		})
		text = text[i+1:]
	}
	if text != "" {
		out = append(out, syntax.Span{
			Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
			Value: text, Pos: pos,
		})
	}
	return out
}

// resolvedBraceText is a resolved body's text, written spans and produced
// ones alike, which is what a range is read from.
func resolvedBraceText(resolved []syntax.Span) string {
	var b strings.Builder
	for _, s := range resolved {
		b.WriteString(s.Value)
	}
	return b.String()
}
