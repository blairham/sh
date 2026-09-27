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
// Three results rather than two, and the third is the one the caller cannot
// get from the other two: the group was **abandoned**, because the produced
// text holds a `{` that this reading will not carry, and the word is left
// exactly as it was written with nothing further in it read. See
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
		// The common answer, and the one that must cost nothing: bash 5.3,
		// bash 3.2 and zsh all find a group's commas in the word the parse
		// cut, and dash and BusyBox ash have no braces to find.
		return nil, false, false
	}
	body := sliceSpans(w.Spans, next(open), close)
	if !bodyHoldsAnExpansion(body) {
		return nil, false, false
	}
	resolved, produced, outcome := r.resolveBraceBody(w, body)
	if outcome == braceBodyNotAGroupHere {
		// Neither reading has a group here, so the word goes on to the one
		// that finds its braces where the parse cut them and finds none.
		return nil, false, false
	}
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
		if outcome != braceBodyResolved || produced.partsTheReadings(rangeShaped(body)) {
			r.askBrace(answer, braceBodyAxis)
			return nil, false, false
		}
		return r.braceBodyAlternatives(w, open, resolved)
	}
	if outcome == braceBodyAbandoned {
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

// braceBodyOutcome is what resolving a group's body came to. Three answers
// rather than two, because "this reading does not apply" and "the word is
// left as written" are different things to the caller: one falls through to
// the reading that finds the braces in the word the parse cut, and the other
// ends the scan.
type braceBodyOutcome uint8

const (
	// braceBodyResolved is a body whose expansions ran and whose produced
	// text is beside the written spans.
	braceBodyResolved braceBodyOutcome = iota
	// braceBodyNotAGroupHere is a body holding an expansion that yields
	// *fields of its own*, which no reading of a body can carry: the group's
	// braces end up in different words. See resolveBraceBody.
	braceBodyNotAGroupHere
	// braceBodyAbandoned is a body whose produced text holds a `{`. See
	// producedBraceAbandonsTheGroup.
	braceBodyAbandoned
)

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
// **An expansion that yields fields of its own is not this reading's**, and
// that is the one place the body is read rather than resolved. A body's
// expansions are unsplit — `e="a b,c"; echo {$e}` is two fields and not three
// — but a *list* still divides the word, so the group's two braces land in
// different words and there is no group left to read. Measured 2026-09-27 on
// ksh93u+ and zsh 5.9.2 with `set -- 1 2`, which agree:
//
//	echo {$@}         {1 2}     is wrong; both answer  {1  2}
//	echo x{p,$@}y     both answer  x{p,1  2}y
//	echo x{p,"$@"}y   the same, so quoting the list does not put it back
//
// bash 5.3.20 and 3.2.57 expand all three, which is
// [Semantics.BraceFanExpandsEachNameOnItsOwn]'s column split rather than this
// axis's — see #4561, where the word count is the subject.
//
// The spans are expanded one at a time through the same pair the unsplit word
// loop uses, rather than through expandWordNoSplit over the body: the pair is
// what says whether an expansion produced a list, and joining first throws
// that away. Nothing here expands a tilde, which is measured — `echo {~,$e}`
// is `~ a b` in ksh93 — and follows from every span reaching this being an
// expansion rather than a literal.
func (r *Runner) resolveBraceBody(w *syntax.Word, body []syntax.Span) ([]syntax.Span, producedText, braceBodyOutcome) {
	// The promise expandWordNoSplit makes, kept here for the same reason it
	// keeps it: a `${u:-*}` inside the body must not match on its own.
	defer r.withoutGlobbing()()
	defer r.inWord(&syntax.Word{Spans: body, Start: w.Start, Stop: w.Stop})()
	failed := r.expandErr
	out := make([]syntax.Span, 0, len(body))
	var produced producedText
	for _, s := range body {
		if s.Kind == syntax.Literal {
			out = append(out, s)
			continue
		}
		if (r.expandErr && !failed) || r.ctl == controlExit {
			// The body is abandoned at its first failed expansion, here as
			// in every other word loop.
			break
		}
		// Held around the pair rather than inside either half, for the
		// reason expandOneWordFields gives. See subscriptSubstHold.
		release := r.armSubscriptSubsts(s)
		parts, _, atList := r.expandAt(s, splitNever, false)
		var text string
		if atList {
			if len(parts) != 1 {
				release()
				return nil, produced, braceBodyNotAGroupHere
			}
			text = r.joinUnsplitEscaped(s.Param, parts)
		} else {
			text, _ = r.expandSpan(s, splitNever, false)
		}
		release()
		// The marks come off a span at a time, exactly as the unsplit word
		// loop takes them off: what goes back into the body is the text a
		// script would see, and the inert spans it lands in are what keep a
		// later stage from reading it.
		text = globUnescape(text)
		if producedBraceAbandonsTheGroup(text) {
			return nil, produced, braceBodyAbandoned
		}
		produced.read(r, text)
		out = append(out, producedBraceSpans(text, s.Pos)...)
	}
	return out, produced, braceBodyResolved
}

// producedText is what a body's expansions left, read for the one question an
// *unanswered* vector has to put: can the two readings of this body part?
//
// Two flags rather than one, because the answer depends on the body's written
// shape as well. See producedText.partsTheReadings.
type producedText struct {
	// structural is a produced `,` or `{`: the characters this axis is
	// about, which divide a body under one reading and stand for themselves
	// under the other.
	structural bool
	// live is produced text that a later stage would *read* — a pattern's
	// metacharacter, a separator the current IFS holds, a `$`, a quote, a
	// tilde, or the `..` a range is written with. Inert text reaches the word
	// the same way whether it went back as a span of data or as the value of
	// an expansion the word ran itself.
	live bool
}

// read folds one produced run into what is known about the body.
func (p *producedText) read(r *Runner, text string) {
	if strings.ContainsAny(text, ",{") {
		p.structural = true
	}
	if !r.producedRunIsInert(text) {
		p.live = true
	}
}

// partsTheReadings reports whether the two readings of this body can give
// different words, which is what an unanswered vector must refuse over and
// must not refuse without.
//
// A produced `,` or `{` always parts them — that is the axis. Live text parts
// them only where the written body is **not** range-shaped: a range's
// endpoints are already read after their expansions under the other reading
// too, and what a range that will not form leaves is inert either way, so
// `sp='2 3'; echo @{1..$sp}@` is the single field `@{1..2 3}@` however the
// body is read. Without that exemption an unanswered vector refused a word
// [Semantics.BraceRangeEndpointsExpanded] already decides on its own.
func (p producedText) partsTheReadings(writtenRange bool) bool {
	return p.structural || (!writtenRange && p.live)
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
