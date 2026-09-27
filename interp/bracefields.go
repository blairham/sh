// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Brace expansion run **after** the word's fields are built, which is the
// reading two of the four columns that have braces take.
//
// The other reading — bash's — is a pre-pass: the braces are found in the word
// the parse cut, each alternative is substituted back into it, and every name
// that comes out is expanded on its own. See Runner.braceExpand, which is
// still that, and Semantics.BraceFanExpandsEachNameOnItsOwn, which is the
// axis between the two.
//
// The panel, measured 2026-09-25 through 2026-09-27 against
// /opt/homebrew/bin/zsh 5.9.2 run `-f`, /bin/ksh `Version AJM 93u+
// 2012-08-01`, /opt/homebrew/bin/bash 5.3.20 and /bin/bash 3.2.57 —
// `go version -m` says *not a Go executable* for each. `set -- 1 2` unless
// stated, and `f` counts its arguments:
//
//	                          bash 5.3 / 3.2       ksh93u+ / zsh 5.9.2
//	f x{p,q}y     (control)   2 | [xpy] [xqy]      2 | [xpy] [xqy]
//	f x{p,q}$@y               4 | [xp1] [2y] …     3 | [xp1] [xq1] [2y]
//	f {p,q}$@                 4 | [p1] [2] …       3 | [p1] [q1] [2]
//	f $@{p,q}y                4 | [1] [2py] …      3 | [1] [2py] [2qy]
//	f x{p,$@}y                3 | [xpy] [x1] [2y]  2 | [x{p,1] [2}y]
//	f x{p,$1}y   (control)    2 | [xpy] [x1y]      2 | [xpy] [x1y]
//	set --; f x{p,q}$@y       2 | [xpy] [xqy]      2 | [xpy] [xqy]
//
// **`x{p,$@}y` is the row that decides what the model is.** A rule that
// distributes the group over the word answers it three words; both reference
// shells answer two, and the braces are still in the output — `x{p,1` and
// `2}y`. The group did not expand at all, because the list put a **field
// boundary between its braces**, and no rule about distributing over a word
// can say that. `x{p,$1}y` is the control that holds the written shape fixed
// and varies only whether the expansion in the group yields one field or two:
// a scalar in the same place keeps the group in both columns.
//
// So the model is: **the word is expanded once with its braces as inert text,
// and the braces are then found in the fields that came out.** Every row above
// falls out of it, and so does `RC_EXPAND_PARAM`'s measured order —
// `a=(1 2); f x{p,q}${^a}y` is `xp1y xq1y xp2y xq2y` in zsh, the brace varying
// fastest *within* a copy of the word, which is what you get when the spread
// copies the word with the braces still text and each copy is brace-expanded
// afterwards. A plan that applies the group at its own position in the word
// answers those four in the other order and cannot be taught out of it.
//
// Which bytes of a field are brace *syntax* is the other half, and it is why
// [wordFields.segs] exists: only the text the script wrote is read. Measured
// on zsh, `e=a,b; f {$e}` is `{a,b}` and `a=("x{p" "q}y"); f ${a}` is
// `[x{p] [q}y]` — a produced comma is data and a produced brace is not a
// delimiter. ksh93 reads a produced comma inside a group the script wrote,
// which is [Semantics.BraceBodyReadAfterExpansion] arriving here as a
// question about the bytes; whether a group an expansion produced *outside*
// any written group is a list there is #4797 and is still open.

// braceFieldsFirst reports whether this word's braces are to be found in the
// fields it comes to rather than in the word the parse cut.
//
// The axis is **read** here and not put. A vector that answered chose its road
// and there is nothing to refuse it over; a vector that has not answered keeps
// the road it had, where the fan puts the question at the hit — a span a
// second name is about to run again — and refuses only there. That is what
// keeps the refusal surface exactly the one this axis already had: the two
// readings agree about `echo {x,y}$V`, which a route decided by *asking*
// would have refused.
//
// The literal fast path is behavior-preserving rather than a shortcut. A word
// whose every span is literal text comes to one field holding exactly the
// word, so the two roads give the same words with the same side effects, and
// the one that keeps the spans the parse cut is the better-worn of the two.
func (r *Runner) braceFieldsFirst(w *syntax.Word) bool {
	if r.noBraceExpand || r.sem().BraceExpansion != Yes {
		return false
	}
	if r.sem().BraceFanExpandsEachNameOnItsOwn != No {
		return false
	}
	if !wordHoldsAnExpansion(w.Spans) || !braceGroupCouldExpand(w.Spans) {
		return false
	}
	if holdsPidBrace(w.Spans) {
		// `$${a,b}` is a run whose outer pair is a *note on the span* rather
		// than text, and a field rebuilt from bytes has nowhere to keep it.
		// The construct is one column's and both routes leave it one word,
		// so it stays on the road that can still read the note. See
		// [syntax.Span.PidBrace].
		return false
	}
	return true
}

// wordHoldsAnExpansion reports whether any span is something other than the
// literal text the parse cut, which is what it takes for the word to come to
// more than the one field holding itself.
func wordHoldsAnExpansion(spans []syntax.Span) bool {
	for _, s := range spans {
		if s.Kind != syntax.Literal {
			return true
		}
	}
	return false
}

// holdsPidBrace reports whether any span opens a `$${ … }` run.
func holdsPidBrace(spans []syntax.Span) bool {
	for _, s := range spans {
		if s.PidBrace {
			return true
		}
	}
	return false
}

// braceGroupCouldExpand reports whether the written spans hold a matched group
// that could make more than one word: a top-level comma, a range shape, or an
// expansion in the body that either reading might make one of those from.
//
// It reads the spans and expands nothing, so a false positive costs only a
// question the caller was going to reach anyway. What it is for is the
// opposite case: `echo {a}` and `echo x{}y` must ask nobody anything.
func braceGroupCouldExpand(spans []syntax.Span) bool {
	from := cursor{0, 0}
	for {
		open, ok := findBraceFrom(spans, from, '{')
		if !ok {
			return false
		}
		if close, matched := matchBraceAcross(spans, open); matched {
			body := sliceSpans(spans, next(open), close)
			if _, literal := literalBody(body); !literal {
				return true
			}
			if _, isList := alternativesInBody(body); isList || rangeShaped(body) {
				return true
			}
		}
		from = next(open)
	}
}

// expandFieldsThenBraces is the word expanded once, with its braces inert, and
// the braces then found in the fields that came out.
func (r *Runner) expandFieldsThenBraces(w *syntax.Word) []string {
	fields, segs := r.expandWordFieldsTracked(w, true)
	if len(segs) != len(fields) {
		// The runs and the fields fell out of step, which is a defect rather
		// than a reading: hand back the fields, which is the answer with no
		// brace in it, rather than reading braces off runs that do not spell
		// the field they claim to.
		return fields
	}
	var out []string
	for i, f := range fields {
		out = append(out, r.braceExpandField(f, segs[i])...)
	}
	return out
}

// braceExpandField is brace expansion over one finished field.
//
// The field is rebuilt as a span list — the runs the script wrote as unquoted
// literals, the rest as spans [braceable] refuses — and handed to the same
// [Runner.braceWords] the pre-pass uses. That is what keeps ranges, the rescan
// rule, the step and padding readings and the rest from being written a second
// time, which is the duplication this tree keeps paying for.
func (r *Runner) braceExpandField(field string, segs []fieldSeg) []string {
	// A field with no `{` in it anywhere holds no group of any provenance,
	// so nothing is rebuilt and nothing is scanned. That is most fields on
	// this road, since the column that reads the braces a value holds has to
	// look at every word with an expansion in it.
	if !strings.ContainsRune(field, '{') || !segsSpell(segs, field) {
		return []string{field}
	}
	made := r.braceFieldProducts(segs)
	if len(made) < 2 {
		// The braces made nothing, so the field is the field. An empty one
		// here is the word having come to nothing and is not an alternative
		// that did.
		return made
	}
	return r.keptBraceProducts(made)
}

// keptBraceProducts removes the products an *alternative* that came to
// nothing left, in the columns that remove them.
//
// This is [Semantics.BraceEmptyAlternativeIsAField] on the road that finds the
// braces in the fields: `f {a,}` is `[a] []` in ksh93u+ and zsh 5.9.2 and
// `[a]` in bash 5.3.20 and 3.2.57. On this road the question is easier to put
// than on the fan's, because a product that is the empty string is exactly an
// alternative that came to nothing with nothing on either side of it — the
// word's own emptiness was decided before the braces were looked at.
//
// Asked only where there is such a product, so a vector that has not answered
// is never refused over a group whose alternatives are all text.
func (r *Runner) keptBraceProducts(made []string) []string {
	empty := false
	for _, f := range made {
		if f == "" {
			empty = true
			break
		}
	}
	if !empty || r.askBrace(r.sem().BraceEmptyAlternativeIsAField,
		"a brace alternative that came to nothing being a field of its own") {
		return made
	}
	out := make([]string, 0, len(made))
	for _, f := range made {
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// braceFieldProducts rebuilds one field as a word and expands its braces.
func (r *Runner) braceFieldProducts(segs []fieldSeg) []string {
	w := &syntax.Word{Spans: fieldSpans(segs)}
	was := r.braceFieldRoute
	r.braceFieldRoute = true
	words := r.braceWords(w, true)
	r.braceFieldRoute = was
	if len(words) == 1 && words[0] == w {
		return []string{joinFieldSpans(r, w)}
	}
	out := make([]string, 0, len(words))
	for _, bw := range words {
		out = append(out, joinFieldSpans(r, bw))
	}
	return out
}

// fieldSpans is the field as a span list: written runs as unquoted literals,
// which is the only shape [braceable] reads, and produced runs as spans it
// refuses.
//
// The quoting on these spans is a **provenance tag** and not a statement
// about the script: the text has already been through the whole of word
// expansion, and what is left to say about it is where it came from.
//
//	Unquoted           the script wrote it, unquoted — the only brace syntax
//	DoubleQuoted       the script wrote it inside quotes — never syntax
//	DollarSingleQuoted an expansion produced it — syntax where a dialect
//	                   says a produced comma or a produced range endpoint is
//
// SingleQuoted is left for the spans the brace machinery *makes*: a range's
// elements arrive that way and are data that still has to be marked against
// the filesystem, while a produced run is already in the marked form the rest
// of the word is in. See joinFieldSpans.
func fieldSpans(segs []fieldSeg) []syntax.Span {
	spans := make([]syntax.Span, 0, len(segs))
	for _, sg := range segs {
		var q syntax.Quoting
		switch sg.kind {
		case segWritten:
			q = syntax.Unquoted
		case segQuoted:
			q = syntax.DoubleQuoted
		case segQuotedProduced:
			q = syntax.BackslashQuoted
		default:
			q = syntax.DollarSingleQuoted
		}
		spans = append(spans, syntax.Span{Kind: syntax.Literal, Quoting: q, Value: sg.text})
	}
	return spans
}

// producedRun reports whether a span of a rebuilt field is text an expansion
// produced. See fieldSpans for the tags.
func producedRun(s syntax.Span) bool {
	return s.Kind == syntax.Literal && s.Quoting == syntax.DollarSingleQuoted
}

// bodyProducedRun is producedRun widened by what a *quoted* expansion
// produced, which a group the script wrote still reads the commas in.
func bodyProducedRun(s syntax.Span) bool {
	return producedRun(s) ||
		(s.Kind == syntax.Literal && s.Quoting == syntax.BackslashQuoted)
}

// braceRangeReads is the same for a *range*: its endpoints are read after the
// expansions in them in both columns that take this road, so the text a range
// is counted from is the written runs and the produced ones together.
//
// `n=3; echo {1..$n}` is `1 2 3` in zsh 5.9.2 and ksh93u+ and `{1..3}` in
// bash, and so is `e=1..3; echo {$e}` — the endpoints question does not ask
// where in the body the expansion stood. See
// Semantics.BraceRangeEndpointsExpanded.
// A *quoted* run counts too, which is measured and is the one place a range
// and the brace scanner read the same bytes differently: `{1..'3'}` is
// `1 2 3` in both columns that expand endpoints at all, so the quotes hide
// the text from the scanner and not from the range. Every run of a rebuilt
// field is literal text, so on this road a range is always counted from the
// body rather than expanded out of it — which is what keeps the endpoint
// road, whose job is to *run* the expansions in a body, from being handed a
// body whose expansions have already run.
func (r *Runner) braceRangeReads() func(syntax.Span) bool {
	if !r.braceFieldRoute {
		return braceable
	}
	if r.sem().BraceRangeEndpointsExpanded != Yes {
		return braceable
	}
	return func(syntax.Span) bool { return true }
}

// fieldAlternatives is the body of a matched group in a rebuilt field.
//
// Two readings, and they part over what an *expansion* left in the body. The
// common one reads only the text the script wrote, so a produced comma is an
// ordinary character. The other reads a produced comma as a separator and is
// [Semantics.BraceBodyReadAfterExpansion] — the same axis the road that
// resolves a body puts, arriving here as a question about the bytes, since on
// this road the expansions have already run.
//
// The second reading goes through the **same** splitter the other road uses,
// which is what keeps two measured facts from being written a second time: a
// produced `}` is data rather than a delimiter, and what a produced
// alternative leaves is neither split nor matched. See
// Runner.braceBodyAlternatives.
//
// The third of those facts — that a produced `{` takes the whole word — is
// **not** asked here, and that is the difference between this road and the
// one that resolves a body. There, a `$e` is still an unexpanded span when
// the group is matched, so the scan cannot see the `{` it will produce and
// producedBraceAbandonsTheGroup is what stands in for the depth nobody could
// count. Here every run is already text, so the depth **is** counted — see
// matchBraceAcrossRead — and a group whose body holds an unbalanced produced
// `{` never matches in the first place. Asking as well would refuse the one
// shape where it *is* balanced, which ksh93u+ expands: `e='{'; f {a,${e}b}c}`
// is `[a] [{b}c]` there, the written `}` closing the produced `{` inside a
// group the written `}` behind it closes. Measured 2026-09-27, in a
// directory of its own with a field counter.
func (r *Runner) fieldAlternatives(w *syntax.Word, open, close cursor) ([][]syntax.Span, bool, bool) {
	body := sliceSpansRead(w.Spans, next(open), close, r.braceScanReads())
	if r.sem().BraceBodyReadAfterExpansion == Yes {
		resolved, deep := resolvedFieldBody(body)
		return r.braceBodyAlternatives(w, open, resolved, deep)
	}
	if alts, ok := alternativesInBody(body); ok {
		return alts, true, false
	}
	if alts, ok := r.rangeAcross(w, open, close, true); ok {
		return alts, true, false
	}
	return nil, false, false
}

// resolvedFieldBody is a rebuilt body in the shape the body splitter wants:
// every run an expansion produced cut into the commas it holds and the inert
// text between them, exactly as a body resolved on the other road arrives.
//
// The marks come off the produced text first and go back on through the inert
// spans, which is the same round trip the other road makes — what goes into
// the splitter is the text a script would see, and what comes out is data.
//
// A produced `{` in the body is left to producedBraceSpans as an ordinary
// inert character: on this road the group only matched because the depth
// counted that `{` and something eligible closed it, so there is nothing
// here for producedBraceAbandonsTheGroup to catch that the match has not
// already caught. See fieldAlternatives.
func resolvedFieldBody(body []syntax.Span) ([]syntax.Span, []bool) {
	out := make([]syntax.Span, 0, len(body))
	var deep []bool
	mark := func(s syntax.Span, produced bool) {
		out = append(out, s)
		if deep != nil {
			deep = append(deep, produced)
		} else if produced {
			deep = make([]bool, len(out)-1, len(body)+4)
			deep = append(deep, true)
		}
	}
	for _, s := range body {
		if !bodyProducedRun(s) {
			mark(s, false)
			continue
		}
		for _, p := range producedBraceSpans(globUnescape(s.Value), s.Pos) {
			// The comma spans producedBraceSpans cuts out are the group's
			// syntax and are read as such; what is left between them is a
			// run of data whose braces still nest. See
			// alternativesInBodyRead.
			mark(p, p.Quoting != syntax.Unquoted && strings.ContainsAny(p.Value, "{}"))
		}
	}
	for len(deep) > 0 && len(deep) < len(out) {
		deep = append(deep, false)
	}
	return out, deep
}

// joinFieldSpans is the word brace expansion produced, back as one field.
//
// Nothing is expanded again: the spans hold text the word has already been
// through, and the only ones that need anything are the spans the brace
// machinery made. A range's elements are *data* — zsh's `{=..?}` is the three
// characters `= > ?` with the `?` never matched against a filename — so they
// are marked here exactly as a quoted span's text is marked on the way into a
// field.
func joinFieldSpans(r *Runner, w *syntax.Word) string {
	var b strings.Builder
	for _, s := range w.Spans {
		if s.Quoting == syntax.SingleQuoted {
			b.WriteString(escapePatternMetaIn(s.Value, r.markedMeta()))
			continue
		}
		b.WriteString(s.Value)
	}
	return b.String()
}

// segsSpell reports whether the runs spell the field they are held beside.
//
// A guard rather than a check that can fail in the ordinary way: the runs are
// built alongside the field by every stage of the word, and a stage that
// forgot one would otherwise read braces out of a field it does not describe.
func segsSpell(segs []fieldSeg, field string) bool {
	n := 0
	for _, sg := range segs {
		n += len(sg.text)
	}
	return n == len(field)
}

// braceSplitStopSpans marks the spans a written `{` stands in front of, for
// the column where a brace ends field splitting for the rest of the word.
//
// Measured 2026-09-27, `f` counting its arguments, `IFS=:` and `v=a:b`:
//
//	                        ksh93u+            zsh 5.9.2 -o shwordsplit
//	f x$v         (control) 2 | [xa] [b]       2 | [xa] [b]
//	f x{p,q}$v              2 | [xpa:b] …      3 | [xpa] [xqa] [b]
//	f x{p}$v                1 | [x{p}a:b]      2 | [x{p}a] [b]
//	f $v{p,q}$w             3 | [a] [bpc:d] …  4 | [a] [bpc] [bqc] [d]
//	f "x{"$v                2 | [x{a] [b]      2 | [x{a] [b]
//	b='{'; f x$b$v          2 | [x{a] [b]      2 | [x{a] [b]
//	f x\{p\}$v              2 | [x{p}a] [b]    2 | [x{p}a] [b]
//
// The control says the probe can see the two columns agree, and it saw the
// three middle rows not agree. The last three rows are what the rule is keyed
// on: a **written, unquoted** `{` stops the splitting and a quoted, escaped or
// produced one does not — and it is the character rather than a group, since
// `{p}` is not a list and stops it just the same. `f $v{p,q}$w` is the row
// that says it is the *rest of the word* and not the word: what stands in
// front of the brace splits.
//
// It subsumes the body: `e='a b,c'; echo {$e}` is the two fields `a b` and
// `c` in ksh93u+ because the body stands behind a brace, and that is the same
// rule rather than one of its own. See Semantics.BraceStopsFieldSplitting.
//
// nil wherever the question does not arise, which is four columns out of
// five. The extent itself is spansBehindAWrittenBrace, shared with the rule
// that reads the same brace one stage later.
func (r *Runner) braceSplitStopSpans(spans []syntax.Span) []bool {
	if r.sem().BraceStopsFieldSplitting != Yes {
		return nil
	}
	return spansBehindAWrittenBrace(spans)
}

// spansBehindAWrittenBrace marks the spans that stand behind the first `{`
// the script wrote unquoted, which is the extent two separate rules share.
//
// One function because there are two of them and they are keyed identically:
// splitting stops there (Semantics.BraceStopsFieldSplitting) and a produced
// `*` or `[` stops being a pattern there
// (Semantics.BraceMakesAProducedStarOrBracketText). Written as a helper each
// had of its own, the second copy is where a later fix to the first would
// fail to arrive — the brace's quoted and escaped exemptions live in
// findBraceFrom's reader, and only a shared call keeps both rules reading
// them the same way.
//
// nil wherever no written unquoted brace has anything behind it, which is
// every word with no brace in it: the ordinary word pays a scan of its spans
// and no allocation.
func spansBehindAWrittenBrace(spans []syntax.Span) []bool {
	open, ok := findBraceFrom(spans, cursor{0, 0}, '{')
	if !ok || open.span+1 >= len(spans) {
		return nil
	}
	marks := make([]bool, len(spans))
	for i := open.span + 1; i < len(spans); i++ {
		marks[i] = true
	}
	return marks
}

// braceRangeText is the text a range is counted from.
//
// On the road that finds the braces in the word the parse cut it is the body
// written as unquoted literals, and nothing else can be read without running
// something. On the road that rebuilt the body from a finished field every
// run is already text, so the range is counted from all of it — which is
// [Semantics.BraceRangeEndpointsExpanded] arriving here, and is also why
// `{1..'3'}` is a range in both columns that expand endpoints: quotes hide
// text from the brace scanner and not from the range.
//
// The marks come off the runs that carry them. A field holds its text in the
// form the *matcher* reads, where a produced `*` is marked in one column and
// not in the other, and a range is counted from the characters a script would
// see: `g='*'; echo {1..$g}` is the eight characters from `1` to `*` in zsh
// 5.9.2, which a marked `\*` would not count.
func (r *Runner) braceRangeText(body []syntax.Span) (string, bool) {
	if !r.braceFieldRoute {
		return literalBody(body)
	}
	read := r.braceRangeReads()
	var b strings.Builder
	for _, s := range body {
		if !read(s) {
			return "", false
		}
		if s.Quoting == syntax.Unquoted {
			b.WriteString(s.Value)
			continue
		}
		b.WriteString(globUnescape(s.Value))
	}
	return b.String(), true
}

// braceScanReads is which of a rebuilt field's runs a brace may be found in.
//
// The runs the script wrote, always — and, in the one column that reads the
// braces a value holds, the runs an expansion produced as well. A run the
// script wrote inside quotes is brace syntax in no column.
//
// See Semantics.BraceScanReadsProducedText.
func (r *Runner) braceScanReads() func(syntax.Span) bool {
	if !r.braceFieldRoute || r.sem().BraceScanReadsProducedText != Yes {
		return braceable
	}
	return braceOrProducedRun
}

// braceOrProducedRun is braceable widened by the runs an expansion produced.
func braceOrProducedRun(s syntax.Span) bool { return braceable(s) || producedRun(s) }

// braceClassOf is the set a `{` may be **closed** by, and it is not symmetric.
//
// A `}` the script **wrote** closes either kind. A `}` an expansion
// **produced** closes only a produced `{`. Measured on ksh93u+ 2026-09-27,
// with a field counter and each case in a directory of its own:
//
//	e='}'; f {a,b$e            1 | [{a,b}]   a produced `}` closes nothing
//	                                         the script wrote
//	e='{'; f ${e}a,b}          2 | [a] [b]   a written one closes what an
//	                                         expansion opened
//	e='{'; c='}'; f ${e}a,b${c}  2 | [a] [b] and so does a produced one,
//	                                         behind a produced opener
//	e='{'; f $e{a,b}           1 | [{{a,b}]  depth is still counted, so an
//	                                         opener with two `{` in front of
//	                                         one `}` stays unmatched
//
// The second row is what #4797's rule did not have. Stated as "a brace pairs
// only with one of its own provenance" it is right about the first row and
// too strong about the second, and the shape that tells them apart is which
// side of the pair the value supplied: the written text of a script closes
// what it can see, and a value closes only what a value opened.
//
// Where produced text is not read at all the two sets are the same set and
// this decides nothing.
func braceClassOf(opener syntax.Span, scan func(syntax.Span) bool) func(syntax.Span) bool {
	if braceable(opener) {
		return braceable
	}
	return scan
}

// braceScanMayReadProducedText reports whether a word with no brace the
// script wrote still has to be looked at, which is the column that reads the
// braces a value holds.
//
// The axis is read and not put, for the reason braceFieldsFirst reads its
// own: this is the road the dialect chose. A word with no expansion in it has
// nothing to produce and never reaches here.
func (r *Runner) braceScanMayReadProducedText(w *syntax.Word) bool {
	return !r.noBraceExpand && r.sem().BraceExpansion == Yes &&
		r.sem().BraceScanReadsProducedText == Yes &&
		r.sem().BraceFanExpandsEachNameOnItsOwn == No &&
		wordHoldsAnExpansion(w.Spans)
}
