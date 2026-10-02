// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// braceExpand turns one word into the words its braces produce.
//
// It runs *before* parameter expansion, which is measured and is why
// `a=1; echo {$a,2}` yields two words rather than one: the braces are
// resolved against the literal text `$a,2`, and only then does `$a` become 1.
//
// A *range* is where that ordering is a disagreement rather than a fact.
// bash keeps it — `n=3; echo {1..$n}` is the literal `{1..3}`, the range
// already gone by the time the variable exists — and zsh and ksh93 expand a
// range's endpoints first and read the range afterwards, so the same line is
// `1 2 3`. `BraceRangeEndpointsExpanded` is that axis, asked only where a
// range is actually written with an expansion in it.
//
// Apart from that it is purely textual: it never consults the filesystem and
// never fails. An unmatched or malformed brace is left alone, which is why
// `echo {a}` prints `{a}`.
//
// Only unquoted literal spans take part in the brace *syntax*. `"{a,b}"` is
// one word, because quoting is what decides whether text is syntax — the same
// rule that decides whether a `*` is a pattern. A quoted endpoint inside a
// range is not that, and is measured: `{1..'3'}` is `1 2 3` in both shells
// that expand endpoints at all, so the quotes hide the text from the brace
// scanner and not from the range.
//
// What comes out then goes back into the word, and *how* is the second
// disagreement: one shell hands it to the rest of word expansion as ordinary
// text, where `$var{x,y}` is the two names `$varx` and `$vary`, and the other
// two substitute the produced spans into the word the parse cut. See
// rereadBraceOutput and Semantics.BraceOutputRereadAsText — and note that
// braceCount below is unaffected either way, since the two readings differ
// about what the words are and never about how many.
func (r *Runner) braceExpand(w *syntax.Word) []*syntax.Word {
	return r.rereadBraceOutput(w, r.braceWords(w, true))
}

// braceCount is braceExpand asked only how many words the braces make, with
// a range's endpoints left unexpanded.
//
// A redirection needs the count, and it needs it about a word whose
// expansions have already been run once — avoiding a second run is the whole
// reason `expandRedirectTargetViews` exists — so a `> {1..$(f)}` counted with
// the endpoints live would run `f` twice. The suppressed count is also the
// measured answer: ksh93 expands endpoints in a word and *not* in a
// redirection target, writing to a file named `{1..2}`.
func (r *Runner) braceCount(w *syntax.Word) int {
	return len(r.braceWords(w, false))
}

// braceRangeShaped reports whether the word holds a matched group whose body
// is range-shaped and is *not* literal — the one shape braceCount is blind
// to, since it leaves a range's endpoints unexpanded and a `{1..$n}` counted
// that way is one word however many the range would make.
//
// It answers "could these braces make words", which is what a caller asks
// before it is worth running anything: it reads the spans and expands
// nothing, and a false positive costs only the questions the caller was
// already going to ask about a group. The scan enters every failed group
// rather than consulting BraceRescanEntersFailedGroup, because a predicate
// that refused a script over where to resume would be answering a question
// its caller has not reached yet.
func (r *Runner) braceRangeShaped(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	from := cursor{0, 0}
	for {
		open, ok := findBraceFrom(w.Spans, from, '{')
		if !ok {
			return false
		}
		if close, matched := matchBraceAcross(w.Spans, open); matched {
			body := sliceSpans(w.Spans, next(open), close)
			if _, literal := literalBody(body); !literal && rangeShaped(body) {
				return true
			}
		}
		from = next(open)
	}
}

// braceWords is the whole of brace expansion, with a switch for whether a
// range's endpoints may be expanded on the way.
func (r *Runner) braceWords(w *syntax.Word, endpoints bool) []*syntax.Word {
	if w == nil {
		return nil
	}
	// A group that does not expand does not end the word. `{x}` has no comma
	// and is literal text in every shell, and the `{a,b}` behind it is still
	// a list: `@{x}{a,b}@` is two words everywhere braces expand at all. The
	// scan therefore carries on past a failed group rather than abandoning
	// the word, and how far it carries is the one thing the panel disagrees
	// about — BraceRescanEntersFailedGroup.
	scan := r.braceScanReads()
	from := cursor{0, 0}
	for {
		open, ok := findBraceFromRead(w.Spans, from, '{', scan)
		if !ok {
			return []*syntax.Word{w}
		}
		close, matched := matchBraceAcrossRead(w.Spans, open, scan, braceClassOf(w.Spans[open.span], scan))
		if matched && r.braceIsAGroupsCount(w.Spans, open, close) {
			// The brace is a pattern group's repetition count, so it is not
			// a list and does not expand. The scan carries on behind the
			// *group*: the count binds to the one group in front of it and
			// a second brace further along the word is a list as usual,
			// while a brace inside the group's body is not one at all. See
			// braceIsAGroupsCount and groupEndAfter.
			from = groupEndAfter(w.Spans, next(close))
			if !before(close, from) {
				// A group nothing closes, so there is nowhere to resume.
				return []*syntax.Word{w}
			}
			continue
		}
		if matched {
			alts, ok, abandoned := r.alternativesAcross(w, open, close, endpoints)
			if abandoned {
				// The group was read after its expansions ran and what they
				// produced holds a brace of its own, which is the one thing
				// that reading will not carry. Nothing further in the word
				// is read either: see producedBraceAbandonsTheGroup.
				return []*syntax.Word{w}
			}
			if ok {
				return r.braceProduct(w, open, close, alts, endpoints)
			}
		}
		// The two readings only part where there is a brace *inside* the
		// group that failed — that is the whole of what one reading reaches
		// and the other steps over. A `{` behind the group is found either
		// way, and a word with no further brace at all is finished either
		// way, so the axis is not asked about `{a}` or `{a..5}`: a question
		// whose two answers are the same answer must not be the thing that
		// refuses a script no shell disagrees about.
		inner, hasInner := findBraceFromRead(w.Spans, next(open), '{', scan)
		diverges := hasInner && (!matched || before(inner, close))
		if diverges && r.askBrace(r.sem().BraceRescanEntersFailedGroup,
			"the scan entering a brace group that did not expand") {
			// bash and zsh resume one byte past the open brace, so a list
			// nested inside the failed group is still found.
			from = next(open)
			continue
		}
		if r.unspecified {
			return []*syntax.Word{w}
		}
		// ksh93 steps over the whole group instead, and has nowhere to
		// resume when the group was never closed.
		if !matched {
			return []*syntax.Word{w}
		}
		from = next(close)
	}
}

// braceProduct substitutes each alternative of the group between open and
// close back into the word, and expands what comes out.
//
// The alternative and the text behind the group are each expanded **on their
// own**, and the three pieces are joined afterwards. Joining first and
// re-reading the result would expand text that no brace in the script wrote:
// `{{1,2,3}..4}` is `{1..4} {2..4} {3..4}` in bash and zsh alike, where the
// group that produced `1` and the `..4}` behind it are both from the file but
// the `{1..4}` they spell between them is not. Measured 2026-09-22, and the
// same for `{6..{7,8,9}}` and `{{1,2,3}..{7,8,9}}` — this is not an axis, it
// is one pass over the word in every shell that expands braces at all.
func (r *Runner) braceProduct(w *syntax.Word, open, close cursor, alts [][]syntax.Span, endpoints bool) []*syntax.Word {
	scan := r.braceScanReads()
	before := sliceSpansRead(w.Spans, cursor{0, 0}, open, scan)
	after := sliceSpansRead(w.Spans, next(close), cursor{len(w.Spans), 0}, scan)

	// The tail is read once rather than once per alternative: it is the same
	// text every time, and `{a,b}{c,d}` is four words either way.
	tails := r.braceWords(&syntax.Word{Spans: after, Start: w.Start, Stop: w.Stop}, endpoints)

	var out []*syntax.Word
	for _, alt := range alts {
		// The alternative is still read, so a group nested inside one — the
		// `{b,c}` of `{a,{b,c}}` — is a list and not text.
		for _, head := range r.braceWords(&syntax.Word{Spans: alt, Start: w.Start, Stop: w.Stop}, endpoints) {
			for _, tail := range tails {
				spans := append([]syntax.Span(nil), before...)
				spans = append(spans, head.Spans...)
				spans = append(spans, tail.Spans...)
				out = append(out, &syntax.Word{Spans: spans, Start: w.Start, Stop: w.Stop})
			}
		}
	}
	return out
}

// cursor is a position in a span list: which span, and how far into it.
//
// Braces are scanned across spans rather than within one, because a word is a
// sequence of them and a brace can straddle several: `{$a,2}` is a literal, a
// parameter expansion and another literal, and its braces are in the first and
// last. Scanning span by span would miss it, which is what made `echo {$a,2}`
// print `{1,2}` where every shell prints `1 2`.
type cursor struct{ span, off int }

func next(c cursor) cursor { return cursor{c.span, c.off + 1} }

// before reports whether one cursor comes strictly before another.
func before(a, b cursor) bool {
	return a.span < b.span || (a.span == b.span && a.off < b.off)
}

// braceable reports whether a span's text takes part in brace syntax. Only
// unquoted literals do: `"{a,b}"` is one word, because quoting decides whether
// text is syntax.
func braceable(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.Unquoted
}

// findBraceFrom finds the next occurrence of c at or after the cursor from.
//
// It takes a cursor rather than a span index because resuming after a group
// that did not expand can land in the middle of a span: `{x}{a,b}` is one
// literal, and a scan that could only resume at the next span would find
// nothing after the first `{`.
func findBraceFrom(spans []syntax.Span, from cursor, c byte) (cursor, bool) {
	return findBraceFromRead(spans, from, c, braceable)
}

// findBraceFromRead is findBraceFrom over a named set of spans. See
// alternativesInBodyRead.
func findBraceFromRead(spans []syntax.Span, from cursor, c byte, read func(syntax.Span) bool) (cursor, bool) {
	for i := from.span; i < len(spans); i++ {
		if !read(spans[i]) {
			continue
		}
		off := 0
		if i == from.span {
			off = from.off
		}
		if off > len(spans[i].Value) {
			continue
		}
		if j := strings.IndexByte(spans[i].Value[off:], c); j >= 0 {
			return cursor{i, off + j}, true
		}
	}
	return cursor{}, false
}

// matchBraceAcross finds the brace closing the one at open.
func matchBraceAcross(spans []syntax.Span, open cursor) (cursor, bool) {
	return matchBraceAcrossRead(spans, open, braceable, braceable)
}

// matchBraceAcrossRead is matchBraceAcross over two named sets of spans, and
// they are two because the question a `{` settles is not the question a `}`
// settles.
//
// scan is where a brace of **either** kind is seen at all: every `{` in one
// of those spans counts toward the depth, whatever produced it. closes is
// the narrower set a `}` may **pair** from — see braceClassOf — and a `}`
// outside it is stepped over without touching the depth, as if it were an
// ordinary character.
//
// The asymmetry is measured rather than designed, on ksh93u+ 2026-09-27,
// each case in a directory of its own with a field counter:
//
//	e='{'; f {a,${e}b}        1 | [{a,{b}]     a produced `{` deepens a
//	                                           written group, so the written
//	                                           `}` closes the produced one
//	e='}'; f {a,b${e}c}       2 | [a] [b}c]    a produced `}` does not close
//	                                           a written group, and does not
//	                                           spend its depth either — the
//	                                           written `}` behind it closes
//	e='{'; f {a,${e}b}c}      2 | [a] [{b}c]   both at once
//
// A scan that counted the second row's `}` would have closed the group at it
// and answered `[a] [b]`, and one that ignored the first row's `{` would
// have answered `[a] [{b]`; neither is what the shell does.
func matchBraceAcrossRead(spans []syntax.Span, open cursor, scan, closes func(syntax.Span) bool) (cursor, bool) {
	depth := 0
	for i := open.span; i < len(spans); i++ {
		if !scan(spans[i]) {
			continue
		}
		mayClose := closes(spans[i])
		start := 0
		if i == open.span {
			start = open.off
		}
		v := spans[i].Value
		for j := start; j < len(v); j++ {
			switch v[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				if !mayClose {
					continue
				}
				depth--
				if depth == 0 {
					return cursor{i, j}, true
				}
			}
		}
	}
	return cursor{}, false
}

// alternativesAcross splits the body between open and close on top-level
// commas, returning each alternative as its own span list.
//
// The third result is the one outcome that is neither "these are the
// alternatives" nor "this group is not a list": a group whose body was read
// *after* its expansions ran, and whose produced text holds a brace. See
// Runner.alternativesAfterExpansion.
func (r *Runner) alternativesAcross(w *syntax.Word, open, close cursor, endpoints bool) ([][]syntax.Span, bool, bool) {
	spans := w.Spans
	if r.braceFieldRoute {
		// The word has already been expanded, so there is no body to
		// resolve: what an expansion produced is text in the field, and
		// whether it is *syntax* is read off the run it arrived in. See
		// interp/bracefields.go.
		return r.fieldAlternatives(w, open, close)
	}
	if !spans[open.span].PidBrace {
		// Asked before the range, because a produced comma beats a produced
		// range exactly as a written one beats a written range, and the body
		// is expanded once for both readings rather than once for each.
		if alts, ok, abandoned := r.alternativesAfterExpansion(w, open, close, endpoints); ok || abandoned {
			return alts, ok, abandoned
		}
	}
	if alts, ok := r.rangeAcross(w, open, close, endpoints); ok {
		return alts, true, false
	}
	if spans[open.span].PidBrace {
		// The outer pair of a `{ … }` run written immediately after `$$` is
		// a *list* nowhere and a range wherever one is written, which is why
		// this stands below the range and not above it. Measured 2026-09-15
		// on zsh 5.9.2, the one column that has the construct, each probe in
		// a script file of its own:
		//
		//	$${a,b}        {a,b}                  one word
		//	$${1,2}        {1,2}                  one word
		//	$${1..3}       <pid>1 <pid>2 <pid>3   the range fired
		//	$${a..c}       <pid>a <pid>b <pid>c
		//	$${1..5..2}    <pid>1 <pid>3 <pid>5   a step too
		//	$${a,1..3}     {a,1..3}               a comma is still no range
		//	$${1..2 3}     {1..2 3}
		//
		// A reading that made the whole pair inert passed the first two rows
		// and lost the next three, and it was a test of this that found it.
		// The *contents* are ordinary spans either way, so the `{1..3}` and
		// the `{a,b}` nested inside `$${x{…}y}` both still expand — the note
		// is on the outer pair and on nothing else. See
		// [syntax.Span.PidBrace].
		return nil, false, false
	}
	alts, ok := alternativesInBody(sliceSpans(spans, next(open), close))
	if !ok {
		// Neither a comma list nor a range, which is the only shape the
		// character-class reading is offered. See
		// Semantics.BraceBodyIsACharacterClass.
		if ccl, got := r.braceCharacterClass(w, open, close); got {
			return ccl, true, false
		}
	}
	return alts, ok, false
}

// braceCharacterClass reads a brace body as a set of characters — `{abc}` as
// `a b c` — for the one shell that has the option for it.
//
// Reached only after the comma reading and the range reading have both
// declined, which is what keeps `{a,b}` and `{1..3}` unchanged when the
// option is on.
func (r *Runner) braceCharacterClass(w *syntax.Word, open, close cursor) ([][]syntax.Span, bool) {
	body := sliceSpansRead(w.Spans, next(open), close, r.braceScanReads())
	text, ok := r.characterClassBody(body)
	if !ok {
		// Nothing literal to read as characters.
		return nil, false
	}
	if r.sem().BraceBodyIsACharacterClass != Yes {
		// Read rather than asked, which is the one thing to notice about
		// this axis. Every dialect answers No — `{abc}` is the word `{abc}`
		// in bash, ksh93, a default zsh, and the two that expand no braces
		// at all — so there is no disagreement for a strict core to refuse
		// over. What moves it is one shell's `braceccl` option, and an
		// option's state is not a dialect's answer. Asking here refused
		// `{a}` in every test that had not heard of the option, which is
		// how this was found.
		return nil, false
	}
	members := characterClassMembers(text)
	if len(members) == 0 {
		// An empty body names nothing, so `{}` stays the word it is in every
		// column, option or no option.
		//
		// **Nothing observable distinguishes this guard**, and it is here
		// saying so rather than claiming a behavior: a mutant that removes
		// it survives, because handing the caller an empty list of
		// alternatives leaves `{}` standing too. It is kept so the intent is
		// local — this function declines rather than relying on how an empty
		// result is treated two frames up — and a reader should not have to
		// take that on trust. A `text == ""` test above it read the same
		// fact and is gone.
		return nil, false
	}
	return r.rangeSpans(members, w.Spans[open.span].Pos), true
}

// characterClassBody is the text a class reads, which is **every literal
// span's value whatever its quoting** — not the unquoted literal run a range
// or an alternative requires.
//
// That is the measurement and not a convenience. On zsh 5.9.2 with the option
// on, a body whose characters arrived quoted or escaped is still a class, and
// the quoting is gone by the time the class sees them:
//
//	{a\-c}   a b c      the escape is data, and the `-` still makes a run
//	{a\,b}   , a b      an escaped comma is a member and not a separator
//	{\a\b}   a b
//	{a\\b}   \ a b      the backslash itself is a member
//	{'ab'}   a b        and so is a quoted run
//	{"ab"}   a b
//
// An **expansion declines here, and the reference does not** — this is a
// gap and not a rule. Measured with `x=abc`: `{$x}` is `a b c` on zsh 5.9.2
// with the option on and stays the word `{abc}` here, which is the same
// question Semantics.BraceRangeEndpointsExpanded answers for a range and
// would need its own answer for a class. Recorded on #5154 rather than
// claimed as behavior; nothing in `D09brace.ztst` reaches it.
//
// A `$'…'` span is its value, as every other quoting is: `{$'\0'-$'\5'}` is
// the six characters NUL to 5, measured 2026-10-02 on zsh 5.9.2 — the
// E01options row `BRACE_CCL option starting from NUL` (#5155). The span keeps
// the escape as written, so reading its value raw made the class the
// backslash, the digits and everything between `0` and `\`.
func (r *Runner) characterClassBody(body []syntax.Span) (string, bool) {
	var b strings.Builder
	for _, s := range body {
		if s.Kind != syntax.Literal {
			return "", false
		}
		if s.Quoting == syntax.DollarSingleQuoted {
			b.WriteString(r.expandDollarSingle(s.Value))
			continue
		}
		b.WriteString(s.Value)
	}
	return b.String(), true
}

// characterClassMembers is the characters a class body names, sorted and
// with duplicates dropped.
//
// An `x-y` run counts only where it **ascends**. A descending one is three
// characters and so is a `-` with nothing on one side of it, which is what
// makes `{c-a}` the three words `-`, `a`, `c` rather than an empty class or
// a reversed run — measured on zsh 5.9.2, and the rows are on the axis.
//
// **Bytes, not characters**, which is measured rather than chosen and is the
// one place a tidier reading would have been wrong: `{áb}` on zsh 5.9.2 in a
// UTF-8 locale is three words — `b` and the two halves of the `á` — so the
// class walks the body a byte at a time and sorts in byte order. A rune
// reading answers `b á`, which is arguably the better shell and is not this
// one. The `{A-z}` row agrees with it from the other side: that run takes in
// `[ \ ] ^ _` and a backquote, which is the ASCII span and not a span of
// letters.
func characterClassMembers(text string) []string {
	var seen [256]bool
	for i := 0; i < len(text); i++ {
		if i+2 < len(text) && text[i+1] == '-' && text[i] <= text[i+2] {
			for c := int(text[i]); c <= int(text[i+2]); c++ {
				seen[c] = true
			}
			i += 2
			continue
		}
		seen[text[i]] = true
	}
	members := make([]string, 0, len(text))
	for c, ok := range seen {
		if ok {
			members = append(members, string([]byte{byte(c)}))
		}
	}
	return members
}

// alternativesInBody splits a group's body on its top-level commas, returning
// each alternative as its own span list.
//
// It reads the body rather than the word it was cut from, because the body is
// not always the spans the parse cut: the reading that runs a group's
// expansions first hands this the *resolved* body, where what an expansion
// produced stands as spans of its own. One walk for both, so the depth rule,
// the escape rule and what counts as a comma cannot drift apart.
func alternativesInBody(body []syntax.Span) ([][]syntax.Span, bool) {
	return alternativesInBodyRead(body, braceable, nil, false)
}

// alternativesInBodyRead is alternativesInBody over a named set of spans.
//
// read says which spans are the group's *syntax*. It is braceable on the road
// that finds the braces in the word the parse cut, and on the road that finds
// them in the fields the word came to it is wider by exactly the runs an
// expansion produced, in the column that reads a produced comma. See
// interp/bracefields.go.
//
// deep marks, by index, the spans that are **not** syntax and whose braces
// still count toward the depth: a run of text an expansion produced, which
// supplies no comma and still nests. deepCloses says whether a `}` in one of
// those runs closes as well as a `{` in one opens, which is the group's own
// opener asked one level down — a produced `}` pairs only behind a produced
// `{`, exactly as it does in matchBraceAcrossRead.
//
// Measured on ksh93u+ 2026-09-27, each case in a directory of its own with a
// field counter:
//
//	e='{{'; f ${e}a,b}}              1 | [{{a,b}}]        the produced `{`
//	                                                      deepens the body
//	e=}; f {a,b$e,c}                 3 | [a] [b}] [c]     a produced `}` in a
//	                                                      written group does
//	                                                      not shallow it
//	e='{'; g='{}'; f ${e}a,${g}b,c}  3 | [a] [{}b] [c]    behind a produced
//	                                                      `{`, it does
//
// A walk that counted neither answers `[{a] [b}]` to the first; one that
// counted the second row's `}` answers `[a] [b},c]`; one that ignored the
// third's answers `[a] [{}b,c]`. nil and false where nothing in the body is
// in that state, which is every body with no expansion behind it. A backslash
// in such a run is data and not an escape, exactly as the comma cut that made
// it treats one.
func alternativesInBodyRead(body []syntax.Span, read func(syntax.Span) bool, deep []bool, deepCloses bool) ([][]syntax.Span, bool) {
	var out [][]syntax.Span
	depth := 0
	from := cursor{0, 0}
	for i, s := range body {
		syntaxHere := read(s)
		if !syntaxHere && (deep == nil || !deep[i]) {
			continue
		}
		v := s.Value
		for j := 0; j < len(v); j++ {
			switch v[j] {
			case '\\':
				if syntaxHere {
					j++
				}
			case '{':
				depth++
			case '}':
				if syntaxHere || deepCloses {
					depth--
				}
			case ',':
				if syntaxHere && depth == 0 {
					out = append(out, sliceSpansRead(body, from, cursor{i, j}, read))
					from = cursor{i, j + 1}
				}
			}
		}
	}
	if len(out) == 0 {
		// `{a}` is not a brace expression and is left alone.
		return nil, false
	}
	return append(out, sliceSpansRead(body, from, cursor{len(body), 0}, read)), true
}

// rangeAcross expands `{n..m}`, either from the literal text between the
// braces or — where the endpoints are written as expansions and the dialect
// says so — from what those expansions come to.
func (r *Runner) rangeAcross(w *syntax.Word, open, close cursor, endpoints bool) ([][]syntax.Span, bool) {
	body := sliceSpansRead(w.Spans, next(open), close, r.braceScanReads())
	pos := w.Spans[open.span].Pos
	if text, ok := r.braceRangeText(body); ok {
		alts, outcome := r.braceRange(text)
		switch outcome {
		case braceRangeCounted:
			return r.rangeSpans(alts, pos), true
		case braceRangeCollapsed:
			return collapsedRange(text, pos), true
		}
		return nil, false
	}
	if !endpoints || r.braceFieldRoute || !rangeShaped(body) {
		// Never on the road that rebuilt this body from a finished field:
		// the expansions in it have already run, and the text they left is
		// what literalBodyRead above already read. See braceRangeReads.
		return nil, false
	}
	if !r.askBrace(r.sem().BraceRangeEndpointsExpanded,
		"a brace range's endpoints expanding before the range is read") {
		return nil, false
	}
	// Once, whatever comes of it. A range that fails to form is put back as
	// the text the endpoints came to rather than as the word that produced
	// it, which is measured twice over: `{1..$(f)}` with a non-numeric `f`
	// runs `f` a single time, and the text it leaves is neither split nor
	// matched — ksh93 splits `$sp` on its own and leaves `{1..$sp}` whole.
	text := strings.Join(r.expandWordNoSplit(&syntax.Word{
		Spans: body, Start: w.Start, Stop: w.Stop,
	}), "")
	alts, outcome := r.braceRange(text)
	switch outcome {
	case braceRangeCounted:
		return r.rangeSpans(alts, pos), true
	case braceRangeCollapsed:
		return collapsedRange(text, pos), true
	}
	return [][]syntax.Span{{{
		Kind: syntax.Literal, Quoting: syntax.SingleQuoted,
		Value: "{" + text + "}", Pos: pos,
	}}}, true
}

// collapsedRange is the one alternative a range that lost its braces leaves:
// the body, as ordinary unquoted text.
//
// Unquoted, which is measured rather than tidy: with a file named `1..x` in
// the directory, `echo {1..}*` prints `1..x`, so what the braces left is a
// word like any other and is still a pattern. It is the half that separates
// this from the text a failed *expanded* endpoint leaves above, which is
// neither split nor matched.
func collapsedRange(text string, pos syntax.Pos) [][]syntax.Span {
	return [][]syntax.Span{{{Kind: syntax.Literal, Value: text, Pos: pos}}}
}

// literalBody gives the text of a brace body written entirely as unquoted
// literals, which is the only body a range can be read from without expanding
// anything. A quoted or substituted span makes it the other case.
func literalBody(body []syntax.Span) (string, bool) {
	return literalBodyRead(body, braceable)
}

// literalBodyRead is literalBody over a named set of spans. See
// alternativesInBodyRead.
func literalBodyRead(body []syntax.Span, read func(syntax.Span) bool) (string, bool) {
	var b strings.Builder
	for _, s := range body {
		if !read(s) {
			return "", false
		}
		b.WriteString(s.Value)
	}
	return b.String(), true
}

// rangeShaped reports whether a brace body is *written* as a range: a `..`
// outside any nested brace, and no comma out there.
//
// The comma decides, because a list beats a range wherever both readings fit:
// `{1..3,5}` is the two words `1..3` and `5` in bash, ksh93 and zsh alike.
// Asking first is also what keeps a list's expansions from being run twice —
// nothing here may expand a body that a range will not read.
func rangeShaped(body []syntax.Span) bool {
	return rangeShapedRead(body, braceable)
}

// rangeShapedRead is rangeShaped over a named set of spans. See
// alternativesInBodyRead.
func rangeShapedRead(body []syntax.Span, read func(syntax.Span) bool) bool {
	depth, dots := 0, false
	for _, s := range body {
		if !read(s) {
			continue
		}
		v := s.Value
		for j := 0; j < len(v); j++ {
			switch v[j] {
			case '\\':
				j++
			case '{':
				depth++
			case '}':
				depth--
			case ',':
				if depth == 0 {
					return false
				}
			case '.':
				if depth == 0 && j+1 < len(v) && v[j+1] == '.' {
					dots = true
					j++
				}
			}
		}
	}
	return dots
}

// rangeSpans turns a range's elements into one-span alternatives.
//
// Quoted, which is the measured difference between a range and a list and is
// reached the moment a character range is wider than the letters. On zsh
// 5.9.2, 2026-09-14, in a directory holding `q` and `z`:
//
//	{=..?}   [=][>][?]     a range's `?` is a character
//	{?,x}    [q][z][x]     a list's `?` is a pattern
//
// So what a *range* counted is data and what a list held is text the word
// still has to expand — in the shells that put the output back as spans.
//
// This comment used to say bash answers the same way, on the same probe, and
// it is wrong about the backslash. Measured 2026-09-22 on bash 5.3.20:
//
//	printf '[%s]' {A..z}   …[Z][[][][]][^][_][`][a]…   the backslash is empty
//	                       zsh keeps it: …[Z][[][\][]]…
//
// bash rereads what the braces produced as shell **text**, so its elements are
// raw characters and the backslash reaches quote removal like any other
// unquoted one. That is BraceOutputRereadAsText, and it is why the quoting
// here is a question rather than a constant (#4200).
func (r *Runner) rangeSpans(alts []string, pos syntax.Pos) [][]syntax.Span {
	q := syntax.SingleQuoted
	if !inertElements(alts) && r.askBrace(r.sem().BraceOutputRereadAsText,
		"brace expansion's output re-entering the word as shell text") {
		// Except where the word re-reads what the braces produced, which is
		// the same question from the other end: there a range's elements are
		// raw characters in a string rather than data in a span, so the
		// backslash `{Z..a}` counts is an unquoted backslash and reaches
		// quote removal. See Semantics.BraceOutputRereadAsText.
		q = syntax.Unquoted
	}
	out := make([][]syntax.Span, 0, len(alts))
	for _, a := range alts {
		out = append(out, []syntax.Span{{
			Kind: syntax.Literal, Quoting: q, Value: a, Pos: pos,
		}})
	}
	return out
}

// inertElements reports whether every element of a counted range is text that
// means the same thing quoted and unquoted.
//
// It is what keeps Semantics.BraceOutputRereadAsText out of the way of the
// ranges anybody writes. `{1..10}`, `{a..z}` and `{01..12}` are the same words
// however their elements re-enter the word, so a dialect that has not answered
// that axis must not be refused over one; `{A..z}` counts the six characters
// between the cases, and that is where the readings part.
//
// The inert set is written out rather than the active one, because the two
// are not complements here: a character nobody has thought about has to land
// on the side that asks the question, not on the side that answers it.
func inertElements(alts []string) bool {
	for _, a := range alts {
		if strings.ContainsFunc(a, func(c rune) bool { return !inertInAWord(c) }) {
			return false
		}
	}
	return true
}

// inertInAWord reports whether a character stands for itself in an unquoted
// word in every dialect: a letter, a digit, and the punctuation that opens
// nothing, quotes nothing, matches nothing and ends no word.
func inertInAWord(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.ContainsRune("-+._/,", c)
}

// sliceSpans copies the spans between two cursors, splitting the ones at the
// ends.
func sliceSpans(spans []syntax.Span, from, to cursor) []syntax.Span {
	return sliceSpansRead(spans, from, to, braceable)
}

// sliceSpansRead is sliceSpans over a named set of spans: read says which
// ones a cursor can point *into*, which is the same set the scan that made
// the cursor was walking. See alternativesInBodyRead.
func sliceSpansRead(spans []syntax.Span, from, to cursor, read func(syntax.Span) bool) []syntax.Span {
	var out []syntax.Span
	for i := from.span; i <= to.span && i < len(spans); i++ {
		s := spans[i]
		if !read(s) {
			if i > from.span || from.off == 0 {
				out = append(out, s)
			}
			continue
		}
		start, end := 0, len(s.Value)
		if i == from.span {
			start = from.off
		}
		if i == to.span {
			end = to.off
		}
		if start > end {
			continue
		}
		if start == 0 && end == len(s.Value) {
			out = append(out, s)
			continue
		}
		if end > start {
			c := s
			c.Value = s.Value[start:end]
			out = append(out, c)
		}
	}
	return out
}

// braceOutcome is what a brace body read as a range came to. Three answers
// rather than two, because one shell takes the braces off a range it could
// not count and leaves the body standing as text — `{1..}` is `1..` — which
// is neither "the word as written" nor a list of elements.
type braceOutcome uint8

const (
	// braceNotARange leaves the word exactly as it was written.
	braceNotARange braceOutcome = iota
	// braceRangeCounted produced the elements below.
	braceRangeCounted
	// braceRangeCollapsed produced no elements and took the braces off.
	braceRangeCollapsed
)

// braceRange expands `{n..m}` and `{a..z}`, counting either way, with an
// optional `..step`.
//
// Three readings are tried in the order a shell reaches them, and the order
// is measured rather than chosen. The **numeric** reading comes first, which
// is what keeps `{1..2..08}` padded to `01` in the shell that pads: its
// endpoints are also single characters, so a character range tried first
// would have answered `1` and lost the width. The **character** reading comes
// next, which is what makes `{1...}` the range `1` to `.` rather than a body
// with a component missing. What is left is a body that is shaped like a
// range and cannot be counted, and that is where the panel parts three ways.
//
// The shells that expand braces at all disagree five times inside a range,
// and each disagreement is a named axis asked where it is reached: whether an
// endpoint's leading zeros pad the range (BraceRangePadsToEndpointWidth),
// what a written step's sign means (BraceRangeStepSignHonored, then
// BraceRangeNegativeStepReverses), what a range between two single characters
// spans (BraceCharRangeSpansAnyCharacter), and what a range that cannot be
// counted leaves behind (BraceRangeMissingEndCountsFromZero,
// BraceRangeZeroStepCountsAsOne and BraceRangeThatCannotBeCounted).
func (r *Runner) braceRange(body string) ([]string, braceOutcome) {
	lo, rest, ok := strings.Cut(body, "..")
	if !ok {
		return nil, braceNotARange
	}
	hi, stepText, hasStep := strings.Cut(rest, "..")

	if holdsADigit(lo) && holdsADigit(hi) && (!hasStep || holdsADigit(stepText)) {
		return r.numericRange(lo, hi, stepText, hasStep)
	}
	if alts, ok := r.charRange(body, lo, hi, stepText, hasStep); ok {
		return alts, braceRangeCounted
	}
	return r.rangeMissingAComponent(lo, hi, stepText, hasStep)
}

// numericRange counts between two written numbers.
func (r *Runner) numericRange(lo, hi, stepText string, hasStep bool) ([]string, braceOutcome) {
	if strings.ContainsRune(lo+hi+stepText, '+') &&
		!r.askBrace(r.sem().BraceRangeNumberMayCarryAPlus, "a range's number carrying a leading plus") {
		return nil, braceNotARange
	}
	if r.unspecified {
		return nil, braceNotARange
	}
	from, err1 := strconv.Atoi(lo)
	to, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil {
		return nil, braceNotARange
	}
	step, negStep := 1, false
	if hasStep {
		n, err := strconv.Atoi(stepText)
		if err != nil {
			return nil, braceNotARange
		}
		negStep = n < 0
		if n < 0 {
			n = -n
		}
		switch {
		case n != 0:
			step = n
		case r.askBrace(r.sem().BraceRangeZeroStepCountsAsOne, "a written step of zero counting as one"):
			// A step of zero is a walk that never arrives, so no shell takes
			// it at its word. One of them reads it as one and counts the
			// range anyway; the other two treat the body as a range they
			// could not count, and part again over what that leaves.
			negStep = false
		default:
			if r.unspecified {
				return nil, braceNotARange
			}
			return r.rangeThatCannotBeCounted()
		}
		if r.unspecified {
			return nil, braceNotARange
		}
	}
	// A step written with leading zeros pads in one of the two columns that
	// pad at all. Asked before the endpoint question and only where the step
	// carries them, so a range whose step is written plainly reaches nothing
	// new. See Semantics.BraceRangeStepPadsTheRange.
	stepPads := false
	if hasStep && paddedEndpoint(stepText) {
		stepPads = r.askBrace(r.sem().BraceRangeStepPadsTheRange,
			"a written step's leading zeros padding the range")
		if r.unspecified {
			return nil, braceNotARange
		}
	}
	width := 0
	paddedEnds := paddedEndpoint(lo) || paddedEndpoint(hi)
	if paddedEnds || stepPads {
		if r.askBrace(r.sem().BraceRangePadsToEndpointWidth, "an endpoint's leading zeros padding the range") {
			// **Either the endpoints or the step, not the wider of them.**
			// A padded endpoint settles the width outright and the step is
			// not consulted at all: measured, `{01..3..0005}` is `01` and
			// not `0001`, where taking the maximum would have widened it to
			// the step. The step supplies the width only when neither
			// endpoint carries zeros — `{1..3..0005}` is `0001`.
			//
			// The two readings agree wherever the step is no wider than the
			// endpoints, which is every row of D09brace and most of a grid
			// written without this pair in mind.
			switch {
			case paddedEnds:
				width = max(len(lo), len(hi))
			case stepPads:
				// The step as **written**, sign and all: `{5..1..-02}` is
				// three characters wide and comes back `001 003 005`.
				//
				// Named rather than left to a `default`, so that the branch
				// says which condition it is the answer to: the outer guard
				// already implies it, and a reader should not have to go
				// back up to find that out.
				width = len(stepText)
			}
		} else if r.unspecified {
			return nil, braceNotARange
		}
	}
	alts, ok := r.walkRange(from, to, step, hasStep, negStep,
		func(i int) string { return padNumber(i, width) })
	if !ok {
		return nil, braceNotARange
	}
	return alts, braceRangeCounted
}

// charRange counts between two single characters, which is two different
// readings rather than one with a wider edge — see
// Semantics.BraceCharRangeSpansAnyCharacter for the measurements.
//
// The two agree about a range between two single *letters* with no step
// written, which is how nearly every character range in the wild is spelled,
// so the axis is not asked there: a question whose two answers are the same
// answer must not be the thing that refuses a script.
func (r *Runner) charRange(body, lo, hi, stepText string, hasStep bool) ([]string, bool) {
	letters := len(lo) == 1 && len(hi) == 1 && isRangeLetter(lo[0]) && isRangeLetter(hi[0])
	loRune, hiRune, twoChars := r.charRangeEndpoints(body)
	renderRune := func(i int) string { return niceRangeChar(rune(i)) }
	if letters && !hasStep {
		// A letter range walks the code points, which is also what makes
		// `{a..C}` produce the punctuation between the cases — measured, not
		// chosen.
		return r.walkRange(int(lo[0]), int(hi[0]), 1, false, false, renderRune)
	}
	if !letters && !twoChars {
		// Neither reading reaches it, so neither has an opinion to ask for.
		return nil, false
	}
	if r.askBrace(r.sem().BraceCharRangeSpansAnyCharacter,
		"a range counting between two characters that are not both letters") {
		// The wider reading takes no step: the body is the two characters
		// and the `..` between them and nothing else, so `{a..z..2}` is not
		// a character range at all in the shell that reads it this way.
		if !twoChars {
			return nil, false
		}
		return r.walkRange(int(loRune), int(hiRune), 1, false, false, renderRune)
	}
	if r.unspecified || !letters {
		return nil, false
	}
	// The narrower reading is letters only, and it does take a step.
	n, err := strconv.Atoi(stepText)
	if err != nil {
		return nil, false
	}
	negStep := n < 0
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	return r.walkRange(int(lo[0]), int(hi[0]), n, true, negStep, renderRune)
}

// niceRangeChar is one element of a character range as the measured shell
// writes it: a character that cannot be printed comes back in that shell's
// escape form rather than as itself.
//
// **Only a character range does this**, which is measured and is what says
// the rendering belongs here and not in rangeSpans. On zsh 5.9.2, with the
// byte 0x01 written as `$'\x01'`:
//
//	{$'\x01'..$'\x01'}   ^A          the range renders
//	{$'\x01',b}          0x01 b      an alternative does not
//	{$'\x01'$'\x02'}     0x01 0x02   nor does a BRACE_CCL class
//	$'\x01'              0x01        nor does `print`
//
// The one-element range is the probe that separates the range from `print`:
// a wider range produces the same text under either hypothesis, and a range
// of one still goes through this code. See instruments.md §4.
//
// Reachable only from the reading Semantics.BraceCharRangeSpansAnyCharacter
// turns on — the letters reading spans at most `A` to `z`, whose widest gap
// is the six punctuation characters between the cases, all of them printable
// — so this function is identity on every character that reading produces.
//
// The three forms, measured 2026-09-29 on zsh 5.9.2, each as a one-element
// range:
//
//	09, 0a            \t and \n           the two with C escapes
//	00-08, 0b-1f      ^@ … ^_             `^` plus the byte or'd with 0x40
//	7f                ^?
//	80-9f             \M-^@ … \M-^_       `\M-` plus the C0 form of c-0x80
//	20-7e, a0-ff      the character       printable, so identity
//
// **What is deliberately not modeled**, because one measurement is not
// enough to model it: above 0x9f the answer is the locale's own idea of
// printable, and it is not a range. Measured on the same shell, `{$'\u00ad'`
// (soft hyphen) is `\M--` while `{$'\u00a0'` (no-break space) is the
// character; `$'\u200b'` and `$'\u0378'` come back as the escapes `\u200b`
// and `\u0378` while `$'\ue000'` is the character. So the shell is asking
// `iswprint` and choosing an escape by the codepoint's size, and modeling
// that from this one locale would pin a platform's answer as a dialect's.
// Those characters keep the identity they have here. Recorded on #5154.
func niceRangeChar(c rune) string {
	switch {
	case c == '\t':
		return `\t`
	case c == '\n':
		return `\n`
	case c < 0x20:
		return "^" + string(c|0x40)
	case c == 0x7f:
		return "^?"
	case c >= 0x80 && c <= 0x9f:
		// `\M-` plus the C0 form of the low half, which is one rule rather
		// than a second table: 0x80 is `\M-^@` because 0x00 is `^@`.
		//
		// **The subtraction is what bounds the recursion**, not a
		// convenience: without it the call lands in this same branch and the
		// stack goes. A mutant that drops it takes the test binary down
		// rather than failing a test, which is a kill that reads as a
		// survivor unless the harness separates the two — see
		// instruments.md §1.
		return `\M-` + niceRangeChar(c-0x80)
	}
	return string(c)
}

// byteRangeEndpoints is charRangeEndpoints where a character is a byte: the
// body is four bytes with the two dots in the middle, and each end is the
// code point its byte numbers.
//
// A function of its own rather than a branch, because the shape test has to
// count **bytes** and the one above counts runes — written as one, a body of
// four runes and a body of four bytes would take whichever test the caller
// happened to reach, and the two differ for exactly the input this reading
// exists for.
func byteRangeEndpoints(body string) (lo, hi rune, ok bool) {
	if len(body) != 4 || body[1] != '.' || body[2] != '.' {
		return 0, 0, false
	}
	return rune(body[0]), rune(body[3]), true
}

// charRangeEndpoints reads a body spelled as exactly one character, `..`, and
// one more character. Counting the characters rather than cutting at the
// first `..` is what the measurements say: `{....}` is the range from `.` to
// `.` and `{.....}` is left alone, which a cut at the first `..` gets the
// wrong way round.
//
// A *character* is the locale's, which is the same question a pattern asks
// and is asked the same way: on zsh 5.9.2, `{α..γ}` is `α β γ` under a UTF-8
// locale and the word as written under `LC_ALL=C`, where the endpoints are
// two bytes each and neither is one character.
func (r *Runner) charRangeEndpoints(body string) (lo, hi rune, ok bool) {
	if !isASCII(body) && !r.patternCountsCharacters(body) {
		// A shell that does not count the locale's characters counts bytes,
		// and a byte above ASCII is then an endpoint of its own rather than
		// a reason to decline. Measured 2026-09-29 on zsh 5.9.2 under
		// `setopt no_multibyte`, `print -rn`:
		//
		//	{$'\x80'..$'\x81'}   \M-^@ \M-^A
		//	{$'\x7e'..$'\x80'}   ~ ^? \M-^@
		//	{$'\xa0'..$'\xa1'}   the two Latin-1 characters
		//
		// The byte is read as the code point it numbers, which is what the
		// third row measures: 0xa0 comes back as U+00A0, encoded for the
		// output. The escape forms the first two rows show are
		// niceRangeChar's and not this function's.
		return byteRangeEndpoints(body)
	}
	if !utf8.ValidString(body) {
		// A byte the encoding cannot decode is not a character to count
		// from, and the range declines rather than counting from something
		// else. Measured 2026-09-29 on zsh 5.9.2 in a UTF-8 locale, `print
		// -rn`, every row the word left exactly as written:
		//
		//	{$'\x80'..$'\x81'}   {\x80..\x81}
		//	{$'\x80'..$'\x80'}   {\x80..\x80}
		//	{$'\x7e'..$'\x80'}   {~..\x80}
		//	{a..$'\xff'}         {a..\xff}
		//	{$'\xc3\xa9'..$'\xc3\xa9'}  é — decodable, so it counts
		//
		// **Without this the conversion below invents a character.**
		// `[]rune` maps each undecodable byte to U+FFFD, and the length test
		// that follows then *passes*: `{$'\x80'..$'\x81'}` became a
		// one-element range of U+FFFD and came back as that one character.
		// Two endpoints the shell cannot read produced a word the script
		// never wrote, and one that is indistinguishable from the answer a
		// byte-counting reading would give — which is why this stands before
		// that reading is added rather than after. See #5154.
		return 0, 0, false
	}
	rs := []rune(body)
	if len(rs) != 4 || rs[1] != '.' || rs[2] != '.' {
		return 0, 0, false
	}
	return rs[0], rs[3], true
}

// rangeMissingAComponent answers a body whose components are not all numbers,
// which is where a range stops being arithmetic and becomes a question about
// what the shell leaves behind.
func (r *Runner) rangeMissingAComponent(lo, hi, stepText string, hasStep bool) ([]string, braceOutcome) {
	// One shell counts from zero when the *second* endpoint is the missing
	// one, and only then: a missing first endpoint or a missing step leaves
	// it with no range at all. Asked only where the first endpoint is a
	// number, since `{a..}` is left alone in every column.
	if hi == "" && (!hasStep || stepText != "") {
		if _, err := strconv.Atoi(lo); err == nil {
			if r.askBrace(r.sem().BraceRangeMissingEndCountsFromZero, "a missing second endpoint counting from zero") {
				return r.numericRange(lo, "0", stepText, hasStep)
			}
			if r.unspecified {
				return nil, braceNotARange
			}
		}
	}
	// What is left is the shape of a range with a gap in it, and the shape
	// is narrow: the first endpoint is a run of digits with no sign, and the
	// second endpoint and the step may each carry one. `{+1..2}` and
	// `{1..2..x}` are outside it and are left alone everywhere.
	last := hi
	if hasStep {
		last = stepText
	}
	if !digitRun(lo, false) || !digitRun(hi, true) || (hasStep && !digitRun(stepText, true)) {
		return nil, braceNotARange
	}
	// And a body with no digit at either end is left alone everywhere too,
	// so the axis is not asked about `{..}`, `{..2..}` or `{......}`.
	if !holdsADigit(lo) && !holdsADigit(last) {
		return nil, braceNotARange
	}
	return r.rangeThatCannotBeCounted()
}

// rangeThatCannotBeCounted resolves what a range-shaped body that produced no
// elements leaves behind.
func (r *Runner) rangeThatCannotBeCounted() ([]string, braceOutcome) {
	switch r.braceRangeFailure() {
	case BraceRangeFailureDropsTheBraces:
		return nil, braceRangeCollapsed
	default:
		return nil, braceNotARange
	}
}

// digitRun reports whether a range component is a run of decimal digits,
// possibly empty, with a leading `-` allowed where signed says so.
func digitRun(s string, signed bool) bool {
	if signed {
		s = strings.TrimPrefix(s, "-")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// holdsADigit reports whether a range component has a decimal digit in it at
// all, which is what separates a component that is missing from one that is
// merely a sign.
func holdsADigit(s string) bool {
	return strings.ContainsFunc(s, func(c rune) bool { return c >= '0' && c <= '9' })
}

// walkRange produces a range's elements from one endpoint to the other. The
// endpoints decide the direction and the step contributes magnitude alone —
// except where a written sign says otherwise, which is a measured
// disagreement asked here rather than resolved by a constant.
func (r *Runner) walkRange(from, to, step int, hasStep, negStep bool, render func(int) string) ([]string, bool) {
	desc := to < from
	if hasStep && negStep != desc &&
		r.askBrace(r.sem().BraceRangeStepSignHonored, "a step's sign overriding the endpoints' direction") {
		// The walk leaves the first endpoint the way the sign says, which
		// here is away from the far one, so the range holds one element.
		return []string{render(from)}, true
	}
	if r.unspecified {
		return nil, false
	}
	dir := 1
	if desc {
		dir = -1
	}
	var out []string
	// No bound on the length. There was one — ten thousand elements, "so a
	// typo cannot hang the shell" — and no shell in the panel has it:
	// measured 2026-10-01, `x=( {1..1000000} )` holds a million elements in
	// zsh 5.9.2, bash 5.3.20 and ksh93u+ alike, and the bound left
	// `{1..20000}` unexpanded here, as one literal word. That is what
	// A05execution.ztst's pipe-hang chunk caught: `printf "%d\n" {1..20000}`
	// printed `0` and a `bad math expression` where zsh prints the numbers
	// (#5140).
	for i := from; (dir > 0 && i <= to) || (dir < 0 && i >= to); i += dir * step {
		out = append(out, render(i))
	}
	if hasStep && negStep &&
		r.askBrace(r.sem().BraceRangeNegativeStepReverses, "a negative step reversing the range") {
		slices.Reverse(out)
	}
	if r.unspecified {
		return nil, false
	}
	return out, true
}

// askBrace asks a brace axis, but only in a dialect whose braces expand at
// all. When BraceExpansion is off or unanswered, whatever brace expansion
// produced is put back or refused by that outer axis, so a question that will
// never change an answer must not be the thing that refuses the script — dash
// prints `{01..3}` as written and is never asked what the zeros mean.
func (r *Runner) askBrace(a Answer, axis string) bool {
	// And the same for a shell whose braces are switched off at run time:
	// what the range came to is put back by the caller, so a range that will
	// never be used must not be the thing that refuses the script.
	if r.sem().BraceExpansion != Yes || r.noBraceExpand {
		return false
	}
	return r.ask(a, axis)
}

// isRangeLetter reports whether a byte can stand as a letter endpoint.
func isRangeLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// paddedEndpoint reports whether an endpoint was written with leading zeros,
// which is what turns the whole range on to padding.
func paddedEndpoint(s string) bool {
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	return len(s) > 1 && s[0] == '0'
}

// padNumber renders n at the given total width, zeros after the sign.
func padNumber(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) >= width {
		return s
	}
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	return sign + strings.Repeat("0", width-len(sign)-len(s)) + s
}
