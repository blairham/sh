// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(~)` expansion flag, which is not the flag-group spelling of `${~name}`
// and was measured before it was implemented.
//
// `${~name}` forces the *whole substituted string* to be read as a pattern —
// that is the GlobExpansionResults override in tildeflag.go. `(~)` inside the
// parentheses does something narrower and, once seen, quite different: it
// marks the **string arguments of the flags written after it in the same
// group**, so a separator the group *inserts* keeps its metacharacters while
// the value the separator stands between does not. The vendor manual says so
// in as many words, and the panel's one shell with the construct agrees.
//
// Measured on zsh 5.9.2, 2026-09-08, with `a=('?' 'x')`:
//
//	[[ '?'   = (${(~j.|.)a}) ]]   yes   the inserted `|` is an alternation
//	[[ 'q'   = (${(~j.|.)a}) ]]   no    and the alternatives are the elements
//	[[ '?|x' = (${(j.|.)a})  ]]   yes   without it the bar is text
//	b=('a*' 'x'); [[ abc = (${(~j.|.)b}) ]]   no   the element stays literal
//	p='a|b';      [[ a   = (${(~)p})    ]]   no   and with nothing to mark,
//	                                                nothing is marked
//	p='a|b';      [[ a   = (${~p})      ]]   yes  which the written one does
//
// The group around each probe is #1497's, not this flag's; see the note
// below.
//
// The last two rows are the ones that separate it from `${~name}`, which
// answers yes to both. So `(~)` is *not* a second way to reach
// GlobExpansionResults, and reading it as one would have made `${(~kj.|.)MAP}`
// match on the map's values as well as on the bar between them.
//
// Order inside the group is load-bearing and parity toggles, both measured:
//
//	${(~j.|.)a}    marked      the tilde is in front of the `j`
//	${(U~j.|.)a}   marked      and anything may stand between them
//	${(j.|.~)a}    not         behind the flag it marks nothing
//	${(~~j.|.)a}   not         two tildes are none
//	${(~j.|.~)a}   marked      the count is taken where the flag is
//	${(~~~j.|.)a}  marked
//
// What the flag can reach is bounded by #1497 and not by this file: a live
// `|` outside a `( … )` is an alternation in that shell and is matched as a
// character here, so a marked separator says what it means inside a group and
// nowhere else yet. Every site that writes the flag in the plugin manager it
// was implemented for writes the group.

// tildeMarksFlagArg reports whether an odd number of `~` stand in front of
// this flag letter in the group.
//
// The letter's *last* occurrence is the one asked about, because that is the
// occurrence whose argument the parser kept: `${(j.a.j.b.)x}` joins on `b`,
// so a tilde that reached only the first `j` reaches no argument that is used.
func tildeMarksFlagArg(flags string, flag byte) bool {
	last := strings.LastIndexByte(flags, flag)
	if last < 0 {
		return false
	}
	return strings.Count(flags[:last], "~")%2 == 1
}

// tildeMarksJoinSep reports whether this group's join separator is marked.
//
// A `j` has to be written for there to be a marked separator at all. The join
// that happens *without* one uses the first character of IFS, which is a
// space by default and is nobody's flag argument — the manual's rule is about
// the arguments of flags, and measured, `"${(~)a}"` on `('?' 'x')` joins on a
// space that matches only a space.
func tildeMarksJoinSep(e *syntax.ParamExpr) bool {
	return e != nil && e.HasFlags && tildeMarksFlagArg(e.Flags, 'j')
}

// tildeMarksSplitSep reports whether this group's `s` separator is marked.
//
// Refused rather than carried, and the measurement is why. A marked split
// separator does not split on a *pattern*; it stops matching at all as soon
// as it holds a character the shell tokenizes, which is an artifact of how
// that shell marks the string rather than a behavior. Measured on zsh 5.9.2,
// 2026-09-08:
//
//	v='a[XY]b'; ${(s.[XY].)v}    `a b`      the literal separator splits
//	v='a[XY]b'; ${(~s.[XY].)v}   `a[XY]b`   marked, it never matches
//	v='a*b';    ${(~s.*.)v}      `a*b`      the same for a bare star
//	v='aXb';    ${(~s.X.)v}      `a b`      a plain separator is unaffected
//
// The set of characters that stops the match is neither the pattern
// metacharacters nor the tokens: `<` splits while `>`, `-` and `~` do not.
// Modeling that would be modeling one implementation's internal marking, so
// the composition is refused by name and the reader is told which half is
// missing. No script in reach writes it — the plugin manager this flag was
// implemented for has twenty-five `(~j…)` sites over twenty-four lines and no
// `(~s…)` site at all.
func tildeMarksSplitSep(e *syntax.ParamExpr) bool {
	return e != nil && e.HasFlags && tildeMarksFlagArg(e.Flags, 's')
}

// tildeMarksPadFill reports whether this group's `l` or `r` fill is marked,
// and which letter it was.
//
// Refused rather than carried, for the reason the `s` separator is. A marked
// fill is live where the rest of the result is escaped, and the padding is
// measured in units of the text it is laid into — so a fill of `*` would have
// to be counted as one unit and escaped as none, which is two answers about
// one string. Measured on zsh 5.9.2 with `w=ab`: `${(~l:5::*:)w}` is the live
// pattern `***ab`, where `${(l:5::*:)w}` is those five characters literally.
// No script in reach writes it.
func tildeMarksPadFill(e *syntax.ParamExpr) (rune, bool) {
	if e == nil || !e.HasFlags {
		return 0, false
	}
	for _, c := range []byte{'l', 'r'} {
		if tildeMarksFlagArg(e.Flags, c) {
			return rune(c), true
		}
	}
	return 0, false
}

// joinLiveSep is the join a marked separator gets: each word escaped on its
// own and the separator laid between them as it was written, so the
// metacharacters that survive are exactly the ones the group inserted.
//
// esc is the escape the result would have been given as a whole, and nil is
// "none". Where the dialect reads every expansion result as a pattern, or a
// written tilde said so for this one, nothing is escaped and the flag has
// nothing left to exempt — measured, `setopt globsubst; b=('a*' 'x');
// [[ abc = ${(~j.|.)b} ]]` is yes, where the same line without the option is
// no.
//
// A function rather than a bool because the two ends of the expansion spell
// the escape differently — a field carries it as globEscape and a pattern
// operand as escapePatternMeta — and handing one of them the other's is how
// a bracket in a value would stop being protected on one path only.
func joinLiveSep(words []string, sep string, esc func(string) string) string {
	if esc == nil {
		return strings.Join(words, sep)
	}
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = esc(w)
	}
	return strings.Join(out, sep)
}

// markedSeparatorPattern is the marked join on the *pattern operand* path —
// the `case` arm, the `[[ ]]` right-hand side and the operand of a trim, which
// share patternOf and reach the flag group through expandParam rather than
// through expandSpan.
//
// It is here rather than left to patternSpan's ordinary route because that
// route escapes the expansion's result as one string, and one string is
// exactly what a marked separator cannot be: the words in it are literal and
// the separator between them is not. Measured on zsh 5.9.2, 2026-09-08, with
// `a=('?' 'x')`:
//
//	case '?' in ((${(~j.|.)a})) …    matches, the bar an alternation
//	case 'q' in ((${(~j.|.)a})) …    does not, the `?` being a literal
//	v='qtail'; ${v#(${(~j.|.)a})}    `qtail`, the same `?` in a trim
//	v='xtail'; ${v#(${(~j.|.)a})}    `tail`
//
// The inner parentheses are #1497's, not this flag's: a `case` arm's own
// paren leaves the expansion at the top level of the pattern, and a live `|`
// there is still matched as a character here.
//
// The second and third rows are the ones that fail if the whole result is left
// live, and the first is the one that fails if the whole result is escaped —
// so neither of patternSpan's two answers is this construct's.
//
// live is always true: the separator is a metacharacter the group asked for,
// so the caller must not escape what comes back. ok is false for every
// expansion this does not apply to, which is all but a marked join.
func (r *Runner) markedSeparatorPattern(s syntax.Span) (text string, live, ok bool) {
	e := s.Param
	if e == nil || e.Bad || !tildeMarksJoinSep(e) {
		return "", false, false
	}
	var esc func(string) string
	if s.Quoting != syntax.Unquoted ||
		!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
		esc = escapePatternMeta
	}
	words, _, _, wok := r.flaggedWords(e, splitNever, false, esc)
	if !wok {
		return "", false, true
	}
	return strings.Join(words, ifsFirst(r.ifs())), true, true
}

// tildeMarkRefusal names the composition this interpreter cannot hold a
// marked separator through, and returns the tail of the diagnostic.
//
// The shell being modeled marks the separator itself and then runs the rest
// of its rule list over the joined text, every step stepping over the mark.
// This interpreter has no mark to carry, so it holds the *join* back past
// those steps instead — which is the same answer for a step that rewrites
// text one character at a time, and a different one for a step that does not:
//
//	${(~qj.|.)d}     a\ b|c      per character, so the two orders agree
//	${(~Uj.|.)d}     A B|C       the same
//	${(~qqj.|.)d}    'a b|c'     one pair of quotes round the *join*
//	${(~Qj.|.)k}     a|b         one level taken off the *join*
//
// Measured 2026-09-08 on zsh 5.9.2 with `d=('a b' 'c')` and `k=("'a" "b'")`.
// The last two rows are what a held-back join cannot produce: quoting each
// word on its own gives `'a b'|'c'`, and unquoting each on its own gives back
// the unbalanced quotes it started with. So they are named rather than
// answered plausibly.
//
// The four refusals, then:
//
//   - A split. `${(~j.|.)a}` beside an `s`, an `f` or a `${(~j.|.)=a}` joins
//     and then splits the join apart again, and what the split hands back is
//     words with no separator between them for the mark to be on. Named
//     first where an expansion has both — `${(~j.|.)=a:-z}` reports the
//     split — because that is the half of it the group itself asked for, and
//     the order is pinned by a test rather than left to the reading order of
//     this function.
//   - An operator, which runs in front of every step the join is held back
//     past: `${(~j.|.)a#p}` would have to trim a pattern out of text that has
//     not been joined yet, and where the expansion is quoted, out of text
//     that has.
//   - `(qq)` and up, `(q-)`, `(q+)`, `(Q)` and `(%)` — the steps that are
//     not a per-character rewrite. A single `(q)` is one, and is carried;
//     the two modifier styles are not, despite being spelled with a single
//     `q`, because each wraps a whole word rather than rewriting its
//     characters. Measured with the same `d`, `${(~q-j.|.)d}` and
//     `${(~q+j.|.)d}` are both `'a b|c'` — one pair of quotes round the
//     join — where quoting each word on its own gives `'a b'|c`. `q+` is
//     the further one: a value it renders moves into a single `$'…'` round
//     the join, which no per-word rewrite could produce at all.
//
// Which is the whole reason the refusal is by *name* and not by a count of
// `q` characters. It was written that way once, and `(q-)` — one `q`, like
// the plain flag it is nothing like — went straight through it.
//
// Named rather than answered wrongly at status 0, which is this file's whole
// convention: a `(~)` read as a no-op turns a plugin manager's alternation
// into one long literal, and that is a name sent to the wrong loader rather
// than a line that fails.
func tildeMarkRefusal(e *syntax.ParamExpr, markJoin, ifsSplit bool) (string, bool) {
	if tildeMarksSplitSep(e) {
		return "for the (s) separator", true
	}
	if c, marked := tildeMarksPadFill(e); marked {
		return "for the (" + string(c) + ") fill", true
	}
	if !markJoin {
		return "", false
	}
	if strings.ContainsAny(e.Flags, splitFlagLetters) || ifsSplit {
		return "beside a split", true
	}
	if padApplies(e) {
		// The padding runs after this join, and the join is the one place
		// where the words have already been escaped and the separator has
		// not. A fill laid into that string would be measured against
		// escaped text and inserted unescaped, so the two are refused
		// together rather than answered from a width nobody wrote. Measured
		// beside it: `arr=(p q); ${(~j.|.l:9::x:)arr}` in the shell that has
		// the flags pads the joined word and leaves the bar live. No script
		// in reach writes the pair — the plugin manager with twenty-five
		// `(~j…)` sites has no padding flag in any of them.
		return "beside a padding flag", true
	}
	if e.Op != syntax.ParamNone {
		return "beside an operator", true
	}
	if e.QuoteModifier != 0 {
		// Ahead of the count, because a group spelling either modifier has
		// exactly one `q` and would otherwise fall through to being carried.
		return "beside the (q" + string(e.QuoteModifier) + ") flag", true
	}
	if n := strings.Count(e.Flags, "q"); n > 1 {
		return "beside the (" + strings.Repeat("q", n) + ") flag", true
	}
	if patternQuoteFlagApplies(e) {
		// `(b)` is the one member of the quoting family the mark does not
		// survive, and it is measured rather than derived. With
		// `a=('x*y' 'p')` on zsh 5.9.2, 2026-09-10:
		//
		//	${(~qj.|.)a}   x\*y|p    the bar live, which is what the mark is for
		//	${(~bj.|.)a}   x\*y\|p   the bar escaped, exactly as ${(bj.|.)a}
		//
		// So the mark buys nothing here: this flag escapes the separator the
		// group inserted along with everything else. Holding the join back
		// past the escape — which is how a marked separator is carried, see
		// the block this refusal guards — would leave that bar live and give
		// the opposite answer, so the pair is named rather than carried.
		// Nothing in reach writes it: the plugin manager and prompt that
		// between them account for every `(b)` on this machine write the
		// flag alone or with `(@)`, and no `(~)` site anywhere carries one.
		return "beside the (b) flag", true
	}
	if strings.ContainsRune(e.Flags, 'Q') {
		return "beside the (Q) flag", true
	}
	if strings.ContainsRune(e.Flags, '%') {
		return "beside the (%) flag", true
	}
	if escapeFlagApplies(e) {
		// Rule 13's other half, refused for rule 13's reason: the escape
		// reading rewrites the joined text and a separator held out of it
		// would be read along with everything else.
		return "beside the (g) flag", true
	}
	if reevalFlagApplies(e) {
		// And the re-reading is the furthest of the lot from a
		// character-at-a-time rewrite: it reads the joined text as shell
		// source, where a separator is not a separator at all.
		return "beside the (e) flag", true
	}
	return "", false
}
