// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// The parenthesized expansion flags: `${(U)x}` and its family. One grammar in
// the panel has the construct, so what it means here is that shell's answer,
// measured and recorded in docs/spec/grammar/parameter-expansion.md; the
// vendor manual's rule list gives the order the steps run in, and the steps
// below carry the rule numbers they implement.

// implementedParamFlags are the flag letters this slice carries. Anything
// else the grammar accepted is refused *by name* when the expansion is
// reached, because the only thing worse than refusing a flag is answering it
// wrong with status 0.
// The `-` in here is the signed-numeric sort flag and only ever that: the
// `-` a `q` ate is not in Flags at all, the parser having taken it out into
// QuoteModifier. `+` is deliberately absent — it is no flag on its own, and
// the parser refuses every `+` a `q` could not take.
const implementedParamFlags = "ULfsj@kvP%qMuoOniaQbcwWA~Zze-lr0VtSm"

// expandFlagged answers an expansion that carries a flag group, as fields.
// It reports false only when the node carries no group, so the ordinary
// paths stay exactly as they were.
// head has the meaning expandSpan gives it: whether this span stands at the
// head of the word being built, which is what the `${~spec}` flag's tilde
// half asks about.
func (r *Runner) expandFlagged(s syntax.Span, sp splitPolicy, head bool) ([]string, bool) {
	e := s.Param
	if e == nil || !e.HasFlags {
		return nil, false
	}
	quoted := s.Quoting != syntax.Unquoted
	// A `(~)` in the group exempts the separator this expansion *inserts*
	// from the escape the rest of the result gets, so whether that escape
	// runs has to be known at the join rather than after it — see
	// interp/tildeflaggroup.go. Asked only where a separator is marked, so
	// no other expansion answers this question twice.
	var escapeSep func(string) string
	if tildeMarksJoinSep(e) {
		if quoted ||
			!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			escapeSep = globEscape
		}
	}
	words, isList, escaped, ok := r.flaggedWords(e, sp, quoted, escapeSep)
	if !ok {
		return nil, true
	}
	if !isList {
		v := words[0]
		if quoted {
			if !escaped {
				v = globEscape(v)
			}
			return []string{v}, true
		}
		if v == "" {
			// An unquoted expansion of an empty value is no field at all.
			return nil, true
		}
		if !escaped &&
			!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			v = globEscape(v)
		}
		// `${(U)~g}` is measured: the group is read, the case applied, and
		// the tilde marks what came out. The tilde may only follow the
		// group — `${~(U)g}` is a bad substitution — so this is the one
		// order there is.
		return []string{r.tildeFlagHead(s, head, v)}, true
	}
	// Empty words are removed from a list result — measured on both sides:
	// `${(s.:.)x}` on `a::b` is two words however it is quoted, and only
	// `"${(@s.:.)x}"` keeps the third. `$@` and an `[@]` subscript keep
	// their empties in quotes without needing the flag, exactly as they do
	// without one.
	// A `${(U)=v}` splits on IFS inside the group, and the fields it made
	// are kept whole in quotes: measured, `v=' a '; "${(U)=v}"` is three
	// fields, the outer two empty. That is the same rule `(@)` asks for,
	// reached by a different flag.
	// Or when the fields are an inner's: the operator around them is about to
	// read them, so an empty one is a value rather than a word the command
	// line loses. See Runner.expandingNestedInner, which carries the
	// measurement — `${(j:,:)${(@)${a[@]}}}` keeps its hole where the same
	// expansion written as a word does not.
	keepEmpty := (quoted || r.expandingNestedInner) &&
		(r.flagKeepsFields(e) || splitFlagInGroup(e, sp))
	// And a quoted `(f)` or `(s)` keeps the empty field at each *edge* while
	// still dropping the interior ones, which is the same rule `${=spec}`
	// already follows for an IFS split and was measured separately for these
	// two flags — see splitFlagEdges.
	edges := !keepEmpty && splitFlagEdges(e, quoted)
	out := make([]string, 0, len(words))
	for i, w := range words {
		// Named rather than negated inline: `!(edges && …)` reads as a
		// double negative at the point it matters most, and the condition
		// it stands for — "this empty field is one the flag keeps because
		// it is at an end" — is the whole reason the branch exists.
		atKeptEdge := edges && (i == 0 || i == len(words)-1)
		if w == "" && !keepEmpty && !atKeptEdge {
			continue
		}
		if quoted || !r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			w = globEscape(w)
		}
		out = append(out, w)
	}
	return r.tildeFlagElements(s, head, out), true
}

// flaggedWords runs the flag pipeline and returns the resulting words, raw.
// ok is false when the expansion failed and the failure has been reported.
// escapeSep says whether the escape that a marked `(~)` separator has to sit
// outside of would run at all; escaped reports that it has already been done
// here, around that separator, so the caller must not do it again.
func (r *Runner) flaggedWords(e *syntax.ParamExpr, sp splitPolicy, quoted bool,
	escapeSep func(string) string,
) (words []string, isList, escaped, ok bool) {
	if e.FlagsErrPos > 0 {
		// A character the group could not carry, deferred here by the
		// parser: reached in a branch never taken, it is no error at all,
		// which is measured. The position counts from the `$`.
		r.diagf("%s\n", Wording(r.diag().ExpansionFlagsError,
			"error in flags near position %[1]d in '%[2]s'",
			e.FlagsErrPos, "${"+e.Src+"}"))
		r.expandErr = true
		return nil, false, false, false
	}
	if strings.ContainsRune(e.Flags, '%') {
		// **Nothing inside a prompt-escape expansion is a pattern.**
		// Measured on zsh 5.9.2 in a directory with no match for it:
		// `${(%):-ab(N)}` unquoted draws the seven characters, where
		// `${(U):-ab(N)}` and a plain `${x:-ab(N)}` are both read as a
		// pattern with a qualifier list and generated away. So it is this
		// flag, and not substitution in general — and not the rewriting the
		// flag does either, since `(U)` rewrites and still globs.
		//
		// Around the whole pipeline rather than around the result, because
		// the word a `:-` substitutes is expanded *as a word* and globs on
		// its own, before anything below could mark it. That is what put
		// a second diagnostic beside the escape's own in #1695:
		// `%$y(l.1.0)` is a qualifier list to a reader that globs it, and
		// the escape being unimplemented was only what left the text there
		// to be read. The sentence it produced was `unknown file attribute:
		// l` then and is `number expected` now — the letter is the link
		// count since #1700 — which changes nothing here: the reading is
		// wrong either way, and this is what stops it.
		//
		// Switched off through the same field `set -f` uses, because it is
		// the same question asked from a different place — see
		// Runner.withoutGlobbing.
		defer r.withoutGlobbing()()
	}
	// Which of the two readings the group's `+` or `-` had was settled by
	// the parser, which keeps the eaten one out of Flags — so this pass has
	// one question to ask of each character and not two, and a `-` reaching
	// it is the sort flag by construction.
	for _, c := range e.Flags {
		if !r.paramFlagCarried(c) {
			r.diagf("${%s}: the (%c) expansion flag is not implemented\n", e.Src, c)
			r.expandErr = true
			return nil, false, false, false
		}
	}
	if strings.ContainsRune(e.Flags, 'm') && padApplies(e) {
		// `(m)` is carried for the length operator and not for the padding
		// pair — see interp/lengthflags.go for the one and this refusal for
		// the other. The two are one letter and two behaviors: a length is a
		// number, and a field measured in columns has to decide what to do
		// when a wide character straddles the edge of it, which is measured
		// and is *not* symmetrical. On zsh 5.9.2 with `w=$'日本'`, four
		// columns of word:
		//
		//	${(ml:3:)w}   `本`     the left field keeps what fits
		//	${(mr:3:)w}   `日本`   the right one keeps what crosses
		//	${(ml:1:)w}   empty
		//	${(mr:1:)w}   `日`
		//	${(ml:7::日:)w}  empty, where `${(mr:7::日:)w}` is `日本日日`
		//
		// The last row is that shell's own edge and not a rule. Building the
		// first four from the last would be inventing one, and a field of the
		// wrong width at status 0 is exactly the shape this refusal exists to
		// prevent: it is a prompt drawing off the end of a line, with nothing
		// anywhere saying which measurement was wrong.
		r.diagf("${%s}: the (m) expansion flag is not implemented beside a padding flag\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	if extendedQuoteRefusal(e) {
		// The one legal `q+` group this slice does not carry. See
		// interp/minimalquote.go for what that shell answers instead.
		r.diagf("${%s}: the (q+) expansion flag is not implemented beside a further (q)\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	// `(~)` marks the string argument of a flag written behind it, and the
	// only argument this interpreter can hold marked is the join separator.
	// The compositions it cannot are named rather than carried — see
	// interp/tildeflaggroup.go for the measurement behind each.
	markJoin := tildeMarksJoinSep(e)
	if why, refuse := tildeMarkRefusal(e, markJoin, splitFlagInGroup(e, sp)); refuse {
		r.diagf("${%s}: the (~) expansion flag is not implemented %s\n", e.Src, why)
		r.expandErr = true
		return nil, false, false, false
	}
	if strings.ContainsRune(e.Flags, 'A') && isAssignOp(e.Op) {
		// `(A)` is the one flag whose whole job is a *side effect*: it makes
		// the name an array — `(AA)` an association — where the expansion
		// assigns, and does nothing at all where it does not. Measured on
		// zsh 5.9.2, both halves:
		//
		//	unset u; ${(A)u=x y}   leaves `typeset -a u=( 'x y' )`
		//	unset u; ${(A)u:-x y}  leaves u unset, `:-` being no assignment
		//	v="a b"; ${(A)#v}      3, the string's length, exactly as ${#v}
		//	v="a b"; "${(A)v}"     one field, exactly as "${v}"
		//	v="a|b"; ${(As:|:)v}   `a b`, exactly as ${(s:|:)v}
		//
		// So the flag is carried by *not* acting on the six lines the panel's
		// plugin managers actually write, which are all the second kind, and
		// the assignment it exists for is refused by name here rather than
		// left to look like it worked: an assignment silently making a
		// scalar where the script asked for an array is the shape a later
		// `${u[2]}` reads as empty.
		//
		// Refused for the *operator* rather than for the assignment actually
		// firing, which is deliberate. `${(A)u=x y}` assigns nothing when `u`
		// is already set, so a check on whether it fired would refuse a line
		// on one run and carry it on the next, and the reader would have
		// nothing to go on. The refusal is a statement about the construct.
		r.diagf("${%s}: the (A) expansion flag is not implemented for an assignment\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	if strings.Count(e.Flags, "q") > 4 {
		r.diagf("${%s}: the (%s) expansion flag is not implemented\n",
			e.Src, strings.Repeat("q", strings.Count(e.Flags, "q")))
		r.expandErr = true
		return nil, false, false, false
	}

	words, set, isList := r.flagBase(e)

	// Rule 4: (P) treats the value so far as a further name, before any
	// operator runs — `${(P)x:-def}` tests the *resolved* value.
	// The whole group goes through with the name. `k` and `v` are the two
	// letters a lookup answers, and baseFlags kept them out of the one that
	// produced this name so that this one has them: measured, `${(kP)n}`
	// with `n=tab` is the keys of `tab` and `${(kvP)n}` its pairs, where
	// passing no group at all answered the values for both — silently, the
	// letters having been accepted and then dropped on the way. Every other
	// letter in the group acts on the words further down this pipeline,
	// which is why these two are the whole of what this step can lose.
	var indirect *indirectTarget
	if strings.ContainsRune(e.Flags, 'P') {
		// Kept, because an operator that assigns further down this pipeline
		// writes *this* name and not the one the expansion spelled. See
		// interp/indirectassign.go — reading the flag on the way out only
		// left the name holding what the parameter should have.
		text, tset := r.indirectSourceText(e, words, set)
		indirect = &indirectTarget{text: text, set: tset}
		// The *name* is read off the front of that text and the rest is
		// dropped, where the assignment further down is handed the text
		// whole: measured, `x='tgt junk'; ${(P)x}` reads `$tgt` and
		// `${(P)x::=new}` is `not an identifier: tgt junk`. So the two are
		// not the same string, and indirectTarget keeps the one the write
		// wants. See indirectName.
		words, set, isList = r.indirectBase(indirectName(text), e.Flags)
	}

	// Rule 4b: `(t)` puts the *type* of the name in place of its value, and
	// everything below this runs on that word — measured, `${(Ut)v}` is
	// `SCALAR`, `${(t)#v}` is 6 and `${(t)v#s}` is `calar`. Behind the `(P)`
	// above, which is also measured: `${(Pt)h}` with `h=arr` describes `arr`
	// and not `h`. See typeFlagBase.
	if strings.ContainsRune(e.Flags, 't') {
		var tok bool
		if words, set, tok = r.typeFlagBase(e, indirect, words, set); !tok {
			return nil, false, false, false
		}
		isList = false
	}

	// The is-it-set question, asked of whatever the base and `(P)` came to:
	// measured, `v=nosuchvar; ${(P)+v}` is 0 while `${(P)v}` is empty and
	// `${+v}` is 1, so the group's name resolution runs and its value
	// transformations do not — `${(U)+v}` is `1` and not an uppercased
	// anything. In front of the nounset check on purpose: `set -u` is not
	// tripped by asking.
	if setTestAnswers(e) {
		// Padded even here, which is measured: `${(l:5::-:)+v}` is `----1`,
		// so the width is applied to the answer the `+` gave. An early
		// return that skipped it answered a bare `1` at status 0.
		padded, pok := r.padFlagged(e, []string{setTestResult(set)})
		return padded, false, false, pok
	}

	if !set && e.Name != "" {
		r.checkNounset(e)
	}

	// Rule 5: in double quotes the words are joined — with the `j`
	// separator when one was given, else the first character of IFS —
	// unless the fields were asked for.
	//
	// A length is asked of the words *before* this, which is measured and is
	// not what the rule numbers suggest: `"${(U)#a}"` on `(abc de f)` is 3,
	// the element count, and not 8, the length of the joined text. A `j`
	// separator does not reach the count either — `"${(Uj.-.)#a}"` is 3 as
	// well — so the join is skipped rather than undone, and `(c)` reads a
	// separator of its own where it wants one.
	joined := false
	if quoted && isList && !e.Length && !r.flagKeepsFields(e) && !markJoin {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
		joined = true
	}

	// Rule 7: the operator, applied to the value at this level. Measured:
	// the flags apply to what the operator leaves — `${(U)x:-def}` is DEF,
	// `${(U)u:=def}` assigns def and substitutes DEF.
	words, isList, ok, nothing := r.applyFlagOp(e, words, set, isList, indirect)
	if !ok {
		return nil, false, false, false
	}
	// The state substitutedNothing names, narrowed to where the shell being
	// modeled can see it. Outside double quotes it cannot: measured on zsh
	// 5.9.2, `v=${(q)y:-$y}` with `y=""` stores `''`, the same two characters
	// an ordinary empty value gives, and only `v="${(q)y:-$y}"` stores
	// nothing. The field itself survives either way — `set -- "${(q)y:-$y}"`
	// leaves `$#` at 1 — so this is a fact about the *text*, and neither the
	// number of fields nor whether one exists.
	nothing = nothing && quoted

	// Rule 9: length — the element count for a list and the value's own
	// length for a scalar, unless `c`, `w` or `W` said to count something
	// else. See lengthflags.go.
	if e.Length {
		words, isList = []string{itoa(r.flaggedLength(e, words, isList))}, false
	}

	// An `=` beside the group is this same step with IFS for a separator.
	ifsSplit := splitFlagInGroup(e, sp)
	// The two splits are kept apart because rule 10 below treats them
	// differently: `(@)` exempts a letter split from the join ahead of it and
	// leaves the `=` one joining. Only rule 11 wants them together.
	letterSplit := strings.ContainsAny(e.Flags, splitFlagLetters)
	hasSplit := letterSplit || ifsSplit
	// Rule 10: forced joining, ahead of a split — `${(s.:.)a}` on an array
	// joins its elements with IFS's first character and splits the result.
	// `Z` is deliberately absent from this condition, and that is measured
	// rather than an oversight: an unquoted `${(Z+n+)a}` over the array
	// `('"x' 'y"')` is two fields there, the elements read as command lines
	// one at a time, where `${(s.:.)a}` over the same array is the single
	// field `"x y` — joined first, exactly as this rule says. So the two
	// splits differ here, and only the *quoted* join at rule 5 reaches `Z`,
	// which is why `"${(Z+n+)a}"` on that array is one field and
	// `"${(@Z+n+)a}"`, whose `@` skips that join, is two again.
	//
	// And `(@)` beside `f` or `s` skips it, which is measured and is the
	// whole of #1683. With `a=(a b)`:
	//
	//	${(s.:.)a}    `a b`  joined on IFS, then split on a colon it has not
	//	${(@s.:.)a}   a b    two fields, each element split on its own
	//	${(@f)a}      a b    the same for the newline split
	//	"${(@s.:.)a}" a b    in quotes as well, rule 5 having been skipped
	//
	// The exemption is the `@` *letter* and not flagKeepsFields, measured
	// apart: `${(s.:.)a[@]}` and `${(s.:.)@}` both join, where
	// `${(@s.:.)a[@]}` and `${(@s.:.)@}` do not. A `j` in the same group
	// puts the join back — `${(@j:-:s.:.)a}` is the one field `a-b` — since
	// a separator was asked for by name.
	//
	// `=` is not exempted, and that is measured rather than symmetry left
	// out: `a=(a '' '' b); "${(@)=a}"` is the two fields `a` and `b`, where
	// splitting each element on its own would keep the two holes the way
	// `"${(@s.:.)a}"` keeps them.
	if (strings.ContainsRune(e.Flags, 'j') || ifsSplit ||
		(letterSplit && !strings.ContainsRune(e.Flags, '@'))) &&
		!joined && isList && !markJoin {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
	}

	// Rule 11: splitting. `f` is split-at-newlines; an empty `s` separator
	// splits into characters, which is measured.
	if hasSplit {
		var split []string
		for _, w := range words {
			if ifsSplit {
				// `${=spec}` splitting, which is field splitting on IFS and
				// not a separator the group named. Quoted it keeps the
				// fields at the edges — see interp/splitflag.go.
				ifs, set := r.ifs()
				split = append(split, r.splitFieldsAsking(w, nil, ifs, set, quoted)...)
				continue
			}
			split = append(split, r.splitFlagged(w, e)...)
		}
		words, isList = split, true
	}

	// Rules 12, 13, 14 in the manual's order: case, prompt escapes, quoting.
	for _, c := range e.Flags {
		if c == 'U' || c == 'L' {
			for i, w := range words {
				words[i] = r.convertCase(w, c == 'U')
			}
		}
	}
	// Rule 13, in the manual's own order: "first any replacements from the
	// `(g)` flag are performed, then any prompt-style formatting". See
	// escapeflag.go for where the reading itself lives and why.
	if escapeFlagApplies(e) {
		words = r.escapeFlagged(e, words)
	}
	if strings.ContainsRune(e.Flags, '%') {
		// Written *twice* is a second question, and the only one of the
		// repeated flags in this file that is. Measured on zsh 5.9.2,
		// 2026-09-11, with `V=WORLD` and `s='a${V}b'`:
		//
		//	setopt promptsubst    ${(%)s}   a${V}b     ${(%%)s}   aWORLDb
		//	unsetopt promptsubst  ${(%)s}   a${V}b     ${(%%)s}   a${V}b
		//
		// So one `%` draws the escapes and never expands, and two also run
		// the value through parameter, command and arithmetic expansion —
		// gated on PROMPT_SUBST, which is what the third row says. A third
		// `%` adds nothing over the second.
		subst := strings.Count(e.Flags, "%") >= 2
		for i, w := range words {
			v, pok := r.promptEscapes(w, e, subst)
			if !pok {
				return nil, false, false, false
			}
			words[i] = v
		}
	}
	if n := strings.Count(e.Flags, "q"); n > 0 {
		for i, w := range words {
			words[i] = quoteFlagged(w, n, e.QuoteModifier, nothing)
		}
	}
	// And after the quoting, which is the order `${(Vq)}` measures: a tab
	// comes out `a$'\t'b`, so the quoting saw the control character and
	// this did not. See visibleflag.go.
	if strings.ContainsRune(e.Flags, 'V') {
		for i, w := range words {
			words[i] = visibleText(w)
		}
	}
	// Rule 14's third spelling: `(b)` marks the *pattern* metacharacters and
	// nothing else. It stands beside `q` rather than before or behind it
	// because the grammar makes the pair unwritable — the two are one family
	// and a group carrying both is `error in flags`, so no measurement could
	// tell an order here from its opposite. What it does compose with is
	// measured, and every row of it lands on this slot:
	//
	//	g='a\x2ab'; ${(bg:e:)g}     a\*b        after the `g` reading
	//	${(b%):-%B*}, TERM set     ESC\[1m\*   after the prompt escapes
	//	v='a*b'; ${(bQ)v}          a*b         before the unquoting
	//	v='a|b'; "${(bZ+n+)v}"     a\|b        before the shell split
	//	${(bl:8:)v}                4 spaces then `a\*b`, so before the pad
	//
	// The first two are the discriminating ones for rule 13. A step that ran
	// ahead of the `g` reading would mark the value's own backslash and hand
	// `(g)` a doubled one, giving the text back rather than a marked `*`; and
	// one that ran ahead of the prompt escapes would leave the `[` the escape
	// produced unmarked, where it is marked. The fourth is `Z`'s, for the
	// reason the `q` row beside it carries — a split that ran first would
	// hand this three words.
	if patternQuoteFlagApplies(e) {
		for i, w := range words {
			words[i] = quotePatternMeta(w)
		}
	}
	// Rule 14's other half: `Q` takes one level of quoting *off*. The manual
	// lists it beside `q` and measurement says which of the two runs first —
	// `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both `'a b'`, the round trip,
	// where a `Q` that ran first would have left `a\ b`. See quoteflag.go.
	if strings.ContainsRune(e.Flags, 'Q') {
		for i, w := range words {
			words[i] = r.unquoteFlagged(w)
		}
	}
	// A marked separator's join, held back to here from rule 10.
	//
	// The shell being modeled joins at rule 10 and marks the separator so
	// the steps between there and here step over it. Holding the join back
	// instead is the same answer wherever those steps rewrite text one
	// character at a time — `b=('a b' 'c'); ${(~qj.|.)b}` is `a\ b|c` either
	// way, since `q` marks a character at a time and never the bar — and the
	// steps where it is *not* the same answer are refused by name in
	// tildeflaggroup.go rather than held back through.
	//
	// Ordering is the one step that has to see the join, and it is the next
	// one: `c=(x a); ${(~oj.|.)c}` is `x|a`, unsorted, exactly as
	// `${(oj.|.)c}` is, and `e=(p p q); ${(~uj.|.)e}` keeps both `p`s for the
	// same reason — the sort and the dedup have one word by the time they
	// run.
	if markJoin {
		words = []string{joinLiveSep(words, r.flagJoinSep(e), escapeSep)}
		isList = false
	}
	// The shell-split flag, `${(Z:opts:)v}`, which reads the value as a
	// command line. It stands here rather than beside the other splits at
	// rule 11, and the place is measured on zsh 5.9.2 — the one shell that
	// has the flag — from both sides:
	//
	//	v="a|b";  "${(Z+n+q)v}"   a\|b       one field: the `q` ran first
	//	v="'a|b'"; "${(Z+n+Q)v}"  a | b     three: the `Q` ran first
	//	v="b  a"; "${(oZ+n+)v}"   a b       sorted: the ordering ran after
	//
	// The first two are the discriminating ones. A split that ran at rule 11
	// would hand `q` three words and give back three fields, and would hand
	// `Q` one quoted word and give back one — both the opposite of what the
	// shell answers. The third pins the other end: the words this makes are
	// what `(o)` sorts, so it cannot go last either.
	//
	// `(f)` and `(s)` compose with it rather than racing it, measured the
	// same day: `${(fZ+n+)v}` on `a:b\nc` is `a:b` and `c`, each line then
	// read as a command line of its own.
	if opts, ok := shellSplitOpts(e); ok {
		words, isList = r.splitShellWordsAll(words, opts), true
	}

	// The ordering step is last of all, which is *later* than the rule
	// numbers suggest and later than this file used to put it. Three
	// measurements fix it there rather than one:
	//
	//	a=(B a);        ${(@oU)a}   A B          after the case conversion
	//	a=("%x" "*");   ${(@%o)a}   * <path>     after the prompt escapes
	//	a=("a b" "a!"); ${(@qo)a}   a! a\ b      after the quoting
	//	a=("'z'" b);    ${(@Qo)a}   b z          and after the unquoting
	//
	// The last two are the ones that would be got wrong by reading the rule
	// list: `*` sorts ahead of a path only once `%x` has become one, and
	// `a!` ahead of `a\ b` only once the space has become a backslash —
	// both orders reverse if the sort runs first.
	if orderApplies(e) {
		words = orderWords(e, words)
	}

	// Rule 22: padding, which stands here rather than where the rule numbers
	// put it — ahead of the re-reading below rather than behind it. See
	// padflags.go, which carries the measurement that fixes the order.
	words, ok = r.padFlagged(e, words)
	if !ok {
		return nil, false, false, false
	}

	// Rule 21: the re-reading, which is last of everything above — later than
	// the ordering, which this file already puts later than the rule numbers
	// say. See reevalflag.go.
	if reevalFlagApplies(e) {
		fields, joined, rok := r.reevalFlagged(e, words, quoted)
		if !rok {
			return nil, false, false, false
		}
		// Rule 23's rejoin, when it ran, leaves as many words as it was
		// given, so what the result *is* has not changed; without it every
		// field the re-reading made is a word of its own.
		words = fields
		if !joined {
			isList = true
		}
	}
	return words, isList, markJoin && escapeSep != nil, true
}

// assignThroughFlags is the assignment side of `${(U)u:=def}` and of
// `${(U)v::=abc}`: the word is stored as written and the flags apply only to
// what is substituted, which is measured — the first leaves `def` behind and
// expands to `DEF`, the second leaves `abc` and expands to `ABC`.
//
// One function for both operators so the two cannot drift: the only
// difference between them is *whether* this runs, which is the caller's
// question and not this one's.
func (r *Runner) assignThroughFlags(e *syntax.ParamExpr, indirect *indirectTarget) ([]string, bool, bool) {
	v := r.joinWord(e.Arg)
	switch {
	case indirect != nil:
		// `(P)` moved the name one step along, and the assignment moves with
		// it — the base's own subscript belongs to *that* resolution and not
		// to the target, measured: `arr=(tgt zz); ${(P)arr[1]::=new}` writes
		// `tgt` and leaves `arr` as it was.
		if !r.assignIndirect(e.Name, indirect, v) {
			return nil, false, false
		}
	case e.Index != nil && !r.wholeArrayIndex(e):
		r.assignSubscript(e, v)
	default:
		// The name check, through the same door the route without a flag
		// group uses: a `${(U)#::=w}` that answered `W`, or a `${(U):=w}`
		// that answered `W`, would each be an assignment to nothing at
		// status 0. Which names it refuses is the operator's question and
		// assignableTarget's to answer, not this switch's — asking it here
		// is what let the two operators drift apart before.
		if !r.assignableTarget(e.Op, e.Name) {
			return nil, false, false
		}
		r.setVar(e.Name, v)
	}
	return []string{v}, false, true
}

// isAssignOp reports whether an operator assigns — the conditional `=` and
// `:=`, which assign when their test fires, and `::=`, which always does.
//
// The `(A)` flag's refusal asks this rather than naming ParamAssign, because
// the flag's job is to say what *kind* of parameter the assignment leaves
// behind and every operator that assigns has that question. Measured
// 2026-09-07 on zsh 5.9.2: `unset u; ${(A)u=x y}` and `unset u; ${(A)u::=x y}`
// both leave `typeset -a u=( 'x y' )`.
func isAssignOp(op syntax.ParamOp) bool {
	return op == syntax.ParamAssign || op == syntax.ParamAssignAlways
}

// flagKeepsFields reports whether a double-quoted result keeps one field per
// word: the `@` flag asks for it, and `$@` and an `[@]` subscript already
// have it — `"${(U)@}"` keeps its fields exactly as `"$@"` does.
func (r *Runner) flagKeepsFields(e *syntax.ParamExpr) bool {
	if strings.ContainsRune(e.Flags, '@') || e.Name == "@" {
		return true
	}
	if strings.ContainsRune(e.Flags, 'P') {
		// A `(P)` moves the expansion one parameter along, and the subscript
		// was written on the one it moved *from*: the `@` belonged to the
		// base, and the words being substituted come from somewhere else
		// entirely. Measured on zsh 5.9.2 with `typeset -A one=(a arr)` and
		// `arr=(x y z)`, `set -- "${(P)one[@]}"` leaves `$#` at 1 — one
		// field holding `x y z` — against 3 for the reading that carries the
		// base's `@` across. The letter is a different matter and still
		// keeps them: `"${(@P)one[@]}"` is three fields (#1638).
		return false
	}
	return e.Index != nil && r.atArrayIndex(e)
}

// flagJoinSep is what joining uses: the `j` argument when one was given, and
// the first character of IFS — a space by default — when not.
func (r *Runner) flagJoinSep(e *syntax.ParamExpr) string {
	if strings.ContainsRune(e.Flags, 'j') {
		return r.flagArgument(e, 'j', e.JoinSep)
	}
	return ifsFirst(r.ifs())
}

// SetFlagArgumentEscapes installs the escape set the `(p)` expansion flag
// reads a following flag's argument with, so `${(pj:\n:)a}` joins on a real
// newline.
//
// A function the dialect supplies rather than a table here, because "the
// escapes" is not one answer even inside one shell: the same shell's `echo`
// keeps the backslash on a letter it does not know and needs `\0` in front of
// an octal number, where its `print` drops the backslash and takes `\101`. A
// flag argument is read with the second of those, measured — and with one
// documented exception, which the decoder handed here has to carry: `\c` ends
// `print`'s output and is an ordinary unknown escape in a flag argument, so
// `${(pj:A\cB:)a}` joins on `AcB`.
//
// The decoder is handed the runner for the reason SetExpansionEscapes's is:
// what a `\u` escape naming a code point the locale has no room for comes to
// is the dialect's answer read against the runner's own locale variables, and
// a decoder with no runner behind it wrote the character in every locale
// there is (#2021).
//
// Nil is the runner nobody told, and there `(p)` is refused by name. That is
// deliberate: a `(p)` read as a no-op joins on a backslash and an `n` at
// status 0, which is the plausible-answer failure this codebase minds most.
func (r *Runner) SetFlagArgumentEscapes(decode func(r *Runner, text string) string) {
	r.flagArgEscapes = decode
}

// paramFlagCarried reports whether this runner carries a flag letter.
//
// All but two are answered by the constant above. `p` and `g` are answered by
// whether the dialect supplied the escape set they read with, because that set
// is a measurement about one shell and this package holds nobody's — so a
// runner nobody told refuses the letter by name instead of reading it as a
// no-op that joins on a backslash and an `n` at status 0.
func (r *Runner) paramFlagCarried(c rune) bool {
	switch c {
	case 'p':
		return r.flagArgEscapes != nil
	case 't':
		// The same rule again, and the sharpest case of it: `(t)` answers
		// with a *word* for what a name is, and the words are one shell's
		// vocabulary. A runner nobody told has none, and any word it made up
		// would be a `[[ ${(t)x} == *array* ]]` answered at status 0 by a
		// shell that had never been asked. See SetParameterTypeWord.
		return r.parameterTypeWord != nil
	case 'g':
		// The same rule and a sharper case of it: `(g)` reads escapes in the
		// *value*, which is again one shell's measurement, and a `(g)` read
		// as a no-op is right for every value with no backslash in it. See
		// SetExpansionEscapes.
		return r.expansionEscapes != nil
	}
	return strings.ContainsRune(implementedParamFlags, c)
}

// flagArgument is what an argument-taking flag's argument comes to: the text
// as written, unless a `p` was written *in front of that flag*, which is the
// whole of what the `(p)` flag does.
//
// Two readings, and they are alternatives rather than steps. Measured
// 2026-09-07 on zsh 5.9.2, the only shell in the panel with the flag, with
// `a=(x y)`:
//
//	s=-;     ${(pj:$s:)a}    x-y       an argument that is one `$name`
//	s='\n';  ${(pj:$s:)a}    x\ny      and the value is *not* then escaped
//	         ${(pj:\n:)a}     x<LF>y    an argument with an escape in it
//	         ${(pj:\$s:)a}    x$sy      which is why the name is read raw
//	         ${(pj:$s\t:)a}   x$s<TAB>  and why one `$name` means the whole
//	         ${(j:\n:)a}      x\ny      no `p`: neither reading runs
//	         ${(j:$s:p)a}     x$sy      and `p` behind the flag is neither
//
// The fourth row is what fixes the order: `\$s` decodes to `$s`, so a reading
// that escaped first and looked for a name second would substitute there, and
// the shell does not. The second says the substituted value is handed over
// verbatim. So: one `$name` that is set, else the escapes, never both.
//
// The name may be a variable, an array — whose elements arrive joined — or a
// positional. Anything else stays as written, `$#` and `$@` and a bare `$`
// included, and so does a name that is not set: measured, `${(pj:$nosuch:)a}`
// joins on the five characters `$nosuch`.
func (r *Runner) flagArgument(e *syntax.ParamExpr, flag rune, raw string) string {
	if !precededByPrintFlag(e.Flags, flag) {
		return raw
	}
	if name, ok := soleParameterReference(raw); ok {
		if isPositional(name) {
			// A positional is always substituted, out of range included,
			// where a *name* has to be set. Measured with `set -- P Q`:
			// `${(pj:$2:)a}` joins on `Q`, `${(pj:$9:)a}` joins on nothing
			// at all, and `${(pj:$nosuch:)a}` joins on the seven characters
			// `$nosuch`. `$0` is a positional here and answers with the
			// script's own name.
			v, _ := r.specialParam(&syntax.ParamExpr{Name: name})
			return v
		}
		if v, set := r.getVar(name); set {
			return v
		}
	}
	return r.flagArgEscapes(r, raw)
}

// precededByPrintFlag reports whether a `p` was written before this flag in
// the group. The order is the rule and not a convenience: measured,
// `${(pj:\n:)a}` joins on a newline and `${(j:\n:p)a}` on a backslash and an
// `n`, so a `p` behind the flag it would modify modifies nothing.
func precededByPrintFlag(flags string, flag rune) bool {
	for _, c := range flags {
		if c == flag {
			return false
		}
		if c == 'p' {
			return true
		}
	}
	return false
}

// soleParameterReference reports the name in an argument that is exactly
// `$name`, and false for anything else.
//
// Exactly: `$s` is the name and `A$s`, `$sA`, `$s $s`, `$s\t`, `${s}`, `$M[k]`,
// `$(echo -)` and a bare `$` are all not — every one of them measured left as
// written. So this is a narrow reading on purpose, and widening it to
// "expand the argument" would substitute in six places the shell does not.
func soleParameterReference(arg string) (string, bool) {
	name, ok := strings.CutPrefix(arg, "$")
	if !ok || name == "" {
		return "", false
	}
	if !isNameLike(name) && !isPositional(name) {
		return "", false
	}
	return name, true
}

// splitFlagLetters are the letters that name a separator split inside the
// group: `f` at newlines, `0` at a NUL, and `s` at the separator it carries.
//
// One constant rather than a literal at each reading. The set is asked about
// in four places — whether the join ahead of a split runs, whether `@`
// exempts it, which empty fields a quoted split keeps, and whether an `=`
// beside the group still decides anything — and a letter added to three of
// them splits correctly while keeping the empty field the shell drops.
const splitFlagLetters = "fs0"

// splitFlagged splits one word the way the group asked: `f` at newlines, `0`
// at a NUL, `s` at its separator, and an empty `s` separator into characters.
//
// **The last of the three letters written is the one that decides**, which is
// measured on zsh 5.9.2 and is not what a fixed precedence gives. With
// `m=$'a\0b:c'` and `n=$'a\nb'`:
//
//	${(@fs.:.)m}   `a\0b` `c`   the `s` behind the `f` splits at the colon
//	${(@s.:.f)m}   `a` `b:c`    and the `f` behind the `s` at the newline
//	${(@f0)n}      one field    the `0` behind the `f` finds no NUL
//	${(@0f)n}      `a` `b`      and the `f` behind the `0` finds the newline
//
// Reading `s` last whatever the order answered the first row for the second,
// which is a plausible split of the wrong string.
func (r *Runner) splitFlagged(w string, e *syntax.ParamExpr) []string {
	sep := "\n"
	i := strings.LastIndexAny(e.Flags, splitFlagLetters)
	switch {
	case i < 0:
		// Unreachable from the pipeline, which asks the same set before it
		// calls this. Written as a case rather than as an index that would
		// panic, because the newline is the answer a group with no letter at
		// all would want and not an invented one.
	case e.Flags[i] == 's':
		sep = r.flagArgument(e, 's', e.SplitSep)
	case e.Flags[i] == '0':
		// The NUL split, `(0)`, which the vendor manual gives as a shorthand
		// for `ps:\0:` and which measures as exactly that: every rule the
		// separator splits already follow — the join ahead of it, the `@`
		// exemption, which empty fields survive — is the same answer here.
		// It has no long form, because an `s` argument cannot hold a NUL:
		// `${(@s.\0.)w}` splits on the two characters `\` and `0`.
		sep = "\x00"
	}
	if sep == "" {
		if w == "" {
			// One empty field rather than none. `strings.Split` on any
			// non-empty separator already answers this way — `${(f)v}` and
			// `${(s.:.)v}` on an empty value are one empty field — and the
			// character split has to agree, because whether the field
			// survives is then the *edge* question and not a second rule.
			// Measured: `v=""; set -- "${(s::)v}"` is one parameter in the
			// shell that has the flag, and none unquoted.
			return []string{""}
		}
		out := make([]string, 0, len(w))
		for _, c := range w {
			out = append(out, string(c))
		}
		return out
	}
	return strings.Split(w, sep)
}

// splitFlagEdges reports whether this expansion keeps the empty field at each
// edge of what `(f)`, `(s)` or `(0)` split.
//
// Measured against zsh 5.9.2 with `(s.:.)` and a colon separator, quoted and
// without `@`, which is the reading that has an answer of its own — `(@)`
// keeps every empty field and unquoted keeps none:
//
//	""       1  ['']            an empty value is one empty field
//	":"      2  ['' '']         both edges, and no interior field between
//	"::"     2  ['' '']         the interior one is dropped
//	":::"    2  ['' '']         and so is a run of them
//	"a:"     2  ['a' '']        the trailing edge is kept
//	":a"     2  ['' 'a']        so is the leading one
//	":a:"    3  ['' 'a' '']     both, around a field that is not empty
//	"a::b"   2  ['a' 'b']       the interior one is dropped
//	"a:::b"  2  ['a' 'b']
//	"a::"    2  ['a' '']        interior dropped, trailing kept
//	"::a"    2  ['' 'a']        leading kept, interior dropped
//
// So it is the edges that are kept and the interior that goes, which is the
// same shape `${=spec}` already had for an IFS split (splitFieldsEdges) and
// is why this is a rule about *which* empty field rather than about whether
// empty fields survive at all. Dropping every one of them answered `n=0` for
// `":"` where the shell says 2, and 0 for an empty value where it says 1 —
// a plausible count at status 0, which is the failure this codebase minds
// most (#1097).
func splitFlagEdges(e *syntax.ParamExpr, quoted bool) bool {
	return quoted && (strings.ContainsAny(e.Flags, splitFlagLetters) || shellSplitActive(e))
}

// flagBase is the value the pipeline starts from: the words, whether the
// parameter was set, and whether the value is a list rather than a scalar.
func (r *Runner) flagBase(e *syntax.ParamExpr) (words []string, set, isList bool) {
	if e.Inner != nil {
		// An expansion standing where a name would. The flags then apply to
		// what it came to, which is the same rule they follow for a name —
		// `${(U)${v}}` and `${${(U)v}}` are both `ABC`, measured.
		//
		// And they apply to a *list* the same way, which is the whole of
		// `${(j: :)${(qkv)m[@]}}`: the inner is one field per key and per
		// value, and the join then runs over all of them. Reporting a list
		// as a scalar joined it here, before the group ever saw it, so the
		// separator went in once around a value that already held them all.
		words, set, isList = r.nestedWords(e)
		return words, set, isList
	}
	if e.Index != nil {
		if list, lok := r.arraySubscript(e); lok {
			if r.wholeArrayIndex(e) {
				if _, isAssoc := r.assocFor(e.Name); isAssoc {
					// `${(kv)m[@]}` is `${(kv)m}`: a whole-array subscript
					// on an association selects every pair, and `k` and `v`
					// then say which half of each pair is substituted.
					// namedBase is where that is decided, and reaching it is
					// what keeps the subscripted spelling from having a
					// second answer of its own — this path took the *values*
					// whatever the letters said, so `${(qkv)ICE[@]}` came
					// back one field per value, half the list, at status 0.
					return r.namedBase(e.Name, baseFlags(e.Flags))
				}
			} else if key, kok := r.assocSubscriptKey(e, list); kok {
				// `${(k)m[key]}` substitutes the *key* rather than what it
				// holds — measured, `${(k)m[b]}` is `b` where `${m[b]}` is
				// `2` — and only `k` on its own does: `${(kv)m[b]}` and
				// `${(v)m[b]}` are both the value, so `v` beside `k` puts
				// the pair's other half back. An absent key is nothing at
				// all under either spelling and never reaches here.
				return []string{key}, true, false
			}
			// Whether the subscript named several elements is
			// subscriptYieldsAList's question and nobody else's. This asked
			// `wholeArrayIndex` instead, which is that predicate with its
			// range clause missing, so a group in front of a range was
			// handed one word with the elements joined — and joined with a
			// hard space at that, where rule 5 next door joins with the
			// group's own `j` separator. Measured on `a=(x y z)`:
			// `"${(j:-:)a[1,3]}"` is `x-y-z` and `set -- "${(@)a[1,3]}"`
			// leaves three parameters, where this reading gave `x y z` and
			// one. The length question was already right — `${#a[1,3]}` is
			// 3 — because it consults subscriptYieldsAList, so the two
			// readings of one subscript disagreed inside this
			// implementation; there is one of them now.
			//
			// The join below is what remains: a subscript naming a single
			// element, and a range over a *scalar*, which is a substring and
			// one value however many characters it holds. Both arrive as one
			// element already, and so does everything else that gets here —
			// an association's key, and a search over an *indexed* array,
			// which names one element in the shell however its operand
			// looks.
			//
			// **A surviving mutant lives on that join, deliberately.**
			// Changing its separator — a space to an empty string, or to
			// anything else — passes the whole suite, because nothing ever
			// reaches it with two elements to put a separator between. A
			// test pinning the space would be a test asserting that this
			// branch can see a list, which is the bug this line was on the
			// wrong side of. What keeps it honest is the predicate above,
			// not the separator here; if a construct ever does arrive here
			// as a list, rule 5 next door is what should join it, with the
			// group's own `j` separator or IFS.
			//
			// `list != nil` is the set-ness for all three constructs this
			// covers, measured rather than assumed. A range over a live
			// array is set even when it names nothing — `${(U)a[3,1]-D}` and
			// `${(U)a[5,6]-D}` are both empty, not `D` — and over an unset
			// name it is unset; a search always answers, `${(U)m[(I)zz]-D}`
			// being empty where `${(U)m[zz]-D}` is `D`. The two are the same
			// fact here: what a live name yields comes from `make` and is
			// never nil, and only an unset name yields nil at all.
			if r.subscriptYieldsAList(e) {
				return list, list != nil, true
			}
			return []string{strings.Join(list, " ")}, list != nil, false
		}
	}
	return r.namedBase(e.Name, baseFlags(e.Flags))
}

// baseFlags is the group as the lookup that produces the *base* reads it.
//
// `k` and `v` are answered by whichever lookup the substituted value is
// finally taken from, and a `(P)` moves that lookup one step along: the base
// is then only the name of the parameter the expansion is really about, and
// the letters belong to the second lookup rather than the first. So they are
// taken out here and passed on at the indirection instead.
//
// Measured on zsh 5.9.2 with `typeset -A tab=(k1 v1 k2 v2)`, an association
// whose value is the name of another one, `typeset -A m=(a tab)`, and a
// scalar `a=A_val` that the keys of `m` would name:
//
//	${(kP)m}          k1 k2   the keys of tab, not of m
//	${(kvP)m}         k1 v1 k2 v2
//	${(kP)m[a]}       k1 k2   a subscript is a base like any other
//	${(kP)m[(r)tab]}  k1 k2   and so is a search
//	${(P)m}           v1 v2   unchanged, values being what P already gave
//
// Reading the letters here as well took the keys of `m`, looked up `A_val`,
// and answered that — a plausible word at status 0, which is the failure
// this codebase minds most. Three lookups read them and all three ask this,
// so the rule is stated once: namedBase, assocSubscriptKey and
// assocSearchWords. See #1608.
func baseFlags(flags string) string {
	if !strings.ContainsRune(flags, 'P') {
		return flags
	}
	return strings.Map(func(c rune) rune {
		if c == 'k' || c == 'v' {
			return -1
		}
		return c
	}, flags)
}

// namedBase resolves a name to its words. flags matters for an associative
// array, where `k` substitutes the keys and `kv` key and value as two
// consecutive words each; the keys come sorted, the same deterministic order
// `${m[@]}` already yields where the shells promise none at all.
//
// Callers resolving the base of an expansion pass baseFlags rather than the
// group itself, because a `(P)` in the group means these letters are the
// *indirection's* to answer.
func (r *Runner) namedBase(name, flags string) (words []string, set, isList bool) {
	switch name {
	case "@", "*":
		return append([]string(nil), r.Params...), len(r.Params) > 0, true
	case "":
		return []string{""}, false, false
	}
	if a, aok := r.assocFor(name); aok {
		hasK := strings.ContainsRune(flags, 'k')
		hasV := strings.ContainsRune(flags, 'v')
		switch {
		case hasK && hasV:
			keys := a.keys()
			out := make([]string, 0, 2*len(keys))
			for _, k := range keys {
				out = append(out, k, a[k])
			}
			return out, len(a) > 0, true
		case hasK:
			return a.keys(), len(a) > 0, true
		default:
			return a.values(), len(a) > 0, true
		}
	}
	if elems, pok := r.pipelineStatuses(name); pok {
		return elems, true, true
	}
	if _, aok := r.Arrays[name]; aok {
		elems, _ := r.arrayElems(name)
		return elems, true, true
	}
	if produce, dok := r.DynamicArrays[name]; dok {
		return produce(r), true, true
	}
	if v, sok := r.specialParam(&syntax.ParamExpr{Name: name}); sok {
		return []string{v}, true, false
	}
	v, vok := r.getVar(name)
	return []string{v}, vok, false
}

// assocSubscriptKey is the key a `${(k)m[key]}` substitutes in place of the
// value the same subscript would have read, and whether that is what this
// expansion asked for.
//
// Three conditions, each measured on zsh 5.9.2 with `typeset -A m=(a 1 b 2)`:
//
//	${(k)m[b]}   b   the key, on an association and with `k` alone
//	${(kv)m[b]}  2   `v` beside it puts the value back
//	${(v)m[b]}   2   and `v` on its own changes nothing
//	${(k)m[zz]}      an absent key is nothing, not the key that was asked for
//
// The last is why the elements are passed in rather than looked up again:
// nil is how assocSubscript says the element was not there, and a key
// substituted for a missing element would answer `zz` where the shell
// answers with no field at all.
//
// An ordinary array reads the same letter as its *index* — `${(k)x[2]}` is
// `2` there, and `${(k)x[-1]}` is the subscript counted forward — which is a
// different question with a different source, and this answers false for it
// rather than guessing. See #1515.
func (r *Runner) assocSubscriptKey(e *syntax.ParamExpr, elems []string) (string, bool) {
	if elems == nil || !e.HasFlags {
		return "", false
	}
	if flags := baseFlags(e.Flags); !strings.ContainsRune(flags, 'k') ||
		strings.ContainsRune(flags, 'v') {
		return "", false
	}
	if _, isAssoc := r.assocFor(e.Name); !isAssoc {
		return "", false
	}
	if e.IndexFlags != nil {
		// A flag group selects by matching rather than by naming, and which
		// half of each match it substitutes is assocSearchWords' question —
		// already answered there, for the same two letters.
		return "", false
	}
	return r.assocKey(e.Subscript()), true
}

// substitutedNothing reports whether the operator substituted a written word
// that came to nothing — a state one shell in the panel keeps apart from a
// value that is merely empty, and which exactly one flag can see.
//
// The two are the same length and the same text. What tells them apart is
// `(q)`, whose backslash style has to write `”` for an empty value because
// backslashes cannot spell one, and which writes nothing at all for this.
// Measured 2026-09-08 on zsh 5.9.2, `y=""` throughout, inside double quotes:
//
//	${(q)y}          ''      an empty value
//	${(q)y:-}        ''      the branch ran with no word to substitute
//	${(q)y:-$y}      nothing the branch ran and the word came to nothing
//	${(q)y:-$nope x}  \      the word came to " ", which is not nothing
//	${(q)nope-$nope} nothing the same, through the operator without a colon
//	${(q)y:+$nope}   nothing y is "a": the alternate ran and came to nothing
//	${(q)y:+}        ''      y is "": the alternate did not run
//	${(q)nope:=$nope} ''     the assigning form substitutes what it stored
//	${(q)y#*}        ''      trimmed to empty is an empty value
//
// So the state belongs to the two operators that substitute a *word* — `-`
// and `+`, with or without the colon — and only on the side that substitutes
// it. The assigning forms are excluded by measurement rather than by
// oversight: they substitute the value they put in the parameter.
//
// The word has to be written. `${(q)y:-}` is `”` and `${(q)y:-$y}` is
// nothing, which is the whole distinction, and e.Arg is nil for exactly the
// first of the two — the parser builds no operand word when there is no text
// behind the operator.
//
// Only the flag group can see this, and only inside double quotes; see
// flaggedWords, where both of those gates are applied.
func substitutedNothing(e *syntax.ParamExpr, words []string, fired bool) bool {
	if e.Arg == nil {
		return false
	}
	switch e.Op {
	case syntax.ParamDefault:
		if !fired {
			return false
		}
	case syntax.ParamAlternate:
		if fired {
			return false
		}
	default:
		// Doubled on purpose, and a mutation that lets the assigning forms
		// through here changes no answer: applyFlagOp reaches those through
		// assignThroughFlags and never asks this at all. The guard that
		// holds today is the call site; this one is what holds if a later
		// caller is added, and it is the readable statement of which
		// operators the state belongs to.
		return false
	}
	// The text test is likewise doubled: quoteWithBackslashes acts on this
	// only where the value it was handed is empty, so a mutation dropping it
	// survives, and a test written to catch that would be asserting nothing.
	// It stays because the name of this function is a claim about the value,
	// and a reader who moved the consumer would have nothing else to go on.
	return len(words) == 1 && words[0] == ""
}

// applyFlagOp runs the expansion's operator over the flagged value — the
// same operators expandParam applies, elementwise where the value is a list.
// ok is false when the expansion was fatal.
//
// nothing is the fourth result and is substitutedNothing's answer: whether
// the operator substituted a written word that came to nothing. It is decided
// here, in the branch that chooses between the operator's two sides, because
// that is the only place that knows which side ran — a caller asking again
// from the outside would have to re-derive `fires`, and a second reading of
// which side ran is how the two come apart.
func (r *Runner) applyFlagOp(e *syntax.ParamExpr, words []string, set, isList bool,
	indirect *indirectTarget,
) ([]string, bool, bool, bool) {
	fires := !set
	if e.Colon {
		fires = !set || strings.Join(words, "") == ""
	}
	switch e.Op {
	case syntax.ParamNone:
	case syntax.ParamDefault:
		if fires {
			sub := []string{r.joinWord(e.Arg)}
			return sub, false, true, substitutedNothing(e, sub, fires)
		}
	case syntax.ParamAssign:
		if fires {
			w, l, ok := r.assignThroughFlags(e, indirect)
			return w, l, ok, false
		}
	case syntax.ParamAssignAlways:
		// No test, so the assignment is the only branch there is. The flags
		// still apply to what is substituted and not to what is stored:
		// measured, `${(U)v::=abc}` is `ABC` and leaves `abc` behind.
		w, l, ok := r.assignThroughFlags(e, indirect)
		return w, l, ok, false
	case syntax.ParamAlternate:
		if fires {
			return []string{""}, false, true, false
		}
		sub := []string{r.joinWord(e.Arg)}
		return sub, false, true, substitutedNothing(e, sub, fires)
	case syntax.ParamError:
		if fires {
			r.fatalParamError("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				e.Name, r.paramErrorWord(e, set)))
			return nil, false, false, false
		}
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		pattern := r.patternOf(e.Arg)
		// Rule: `M` substitutes what the pattern *took* rather than what it
		// left. The same operator and the same match, read from the other
		// side — measured, `${(M)v#h*l}` on `hello` is `hel` and
		// `${(M)v##h*l}` is `hell`, so the shortest/longest choice is still
		// the operator's.
		//
		// `(S)` is the other flag this operator reads, and the node carries
		// it rather than a second `take` here: it changes *which* match the
		// trim found and not which side of it is substituted, so the two
		// compose without either knowing about the other. See
		// interp/searchflag.go.
		take := r.trimWith
		if matchingFlag(e) {
			take = r.matchedWith
		}
		for i, w := range words {
			words[i] = take(w, pattern, e)
		}
	case syntax.ParamReplace:
		pattern := r.patternOf(e.Arg)
		for i, w := range words {
			words[i] = r.replaceWith(w, pattern, e)
		}
	case syntax.ParamSubstring:
		if isList {
			return sliceElems(words, r.numOf(e.Arg, e, e.Arg2), e, r), true, true, false
		}
		words[0] = r.substringRange(words[0], e)
	case syntax.ParamZip, syntax.ParamZipCycle:
		// A list either way: the zip produces one, and a scalar is the
		// one-element list it already is — `s=one; b=(x y); ${s:^b}` is
		// `one x`, measured.
		return r.zipElements(e, words), true, true, false
	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection,
		syntax.ParamElementReplace:
		if isList {
			return r.reshapeElements(e, words), true, true, false
		}
		// Not a list: `${(U)v:#p}` asks the same question of one value, and
		// the answer is that value or nothing. It stays a scalar rather than
		// becoming an empty list, so `"${v:#p}"` is one empty field the way
		// `"${v#p}"` is.
		words[0] = r.reshapeScalar(e, words[0])
	case syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		for i, w := range words {
			words[i] = r.changeCase(w, e)
		}
	}
	return words, isList, true, false
}

// convertCase is the `U` and `L` flags: every letter, under the same locale
// policy the case-changing operators follow — an explicit C locale narrows
// to ASCII and anything else is Unicode-aware.
func (r *Runner) convertCase(v string, upper bool) string {
	convert := unicode.ToUpper
	if !upper {
		convert = unicode.ToLower
	}
	return r.caseChanged(v, convert)
}

// PromptExpand is the prompt-escape language over one string, for a builtin
// whose whole job is the `${(%)…}` flag under another spelling.
//
// Exported so that `print -P` is the *same* expansion rather than a second
// one. Two tables of prompt escapes is how the two answers drift apart, and
// the escapes this shell carries are few enough that the drift would be
// invisible until a script wrote the one they disagreed about.
//
// The second result is false where an escape was refused; the refusal has
// already been written, and the caller's business is only to stop.
func (r *Runner) PromptExpand(text string) (string, bool) {
	// Always expanded, subject to the option: measured, `print -P 'a${V}b'`
	// draws `aWORLDb` under PROMPT_SUBST and `a${V}b` without it. That is
	// the doubled `%` flag's answer rather than the single one's, and this
	// is the route a *builtin* asks by.
	return r.promptEscapes(text, nil, true)
}

// promptEscapes is the `%` flag over one word.
//
// The dialect's table and the one walker, which is the whole of #1090: this
// used to carry four escapes of its own — `%%`, `%x`, `%N`, `%n` — and refuse
// the rest by name, while the prompt drawer read a second table of about
// forty. So `print -P '%F{196}…'` was refused and a drawn prompt answered it.
// The four are rows of the one table now: `%x` and `%N` became
// FieldSourceFile and FieldUnitName, and `%%` and `%n` were already there as
// FieldEscape and FieldUser.
//
// The refusal stays, and it is still the promise: an escape this shell cannot
// answer is named rather than dropped. What names it is now the resolver
// saying it has no answer — for the session's own facts, which a Runner has
// not got — or the table listing the code as Unsupported, for the shapes
// needing a mechanism rather than a value. See interp/prompt.go.
//
// e is the expansion this is the `%` flag of, and nil where a *builtin*
// asked — see PromptExpand. It decides only how a refusal names the place it
// happened.
//
// subst says whether the value is also expanded before the escapes are
// drawn, which is the difference between one `%` flag and two and is what
// `print -P` does always. It goes through [RenderPromptValue] rather than
// running a second expansion here, because that helper holds the pair —
// which pass runs first is [PromptStyle.ExpandBeforeEscapes] and differs by
// dialect, and a refusal has to stop both. Writing the order out again here
// is how the two readers drift apart.
func (r *Runner) promptEscapes(v string, e *syntax.ParamExpr, subst bool) (string, bool) {
	expand := ExpandPromptStyle
	if subst {
		expand = func(st PromptStyle, text string, f PromptResolver, q PromptQuantityResolver) (string, string, bool) {
			return RenderPromptValue(st, r, text, f, q)
		}
	}
	out, code, ok := expand(r.promptStyle, v, r.promptField, r.promptQuantity)
	if !ok {
		src := ""
		if e != nil {
			src = e.Src
		}
		return r.refusePromptEscape(src, code)
	}
	return out, true
}

// refusePromptEscape says, by name, that an escape is not carried here.
//
// One place rather than two, because the wording is the promise: it names the
// escape the script asked for, so a reader can tell which of several in one
// word was the one this shell could not answer. A conditional names its test
// letter with the character that opened it — `%(e` — because the letter alone
// is not an escape and would send a reader looking for the wrong thing.
//
// The expansion route quotes the whole construct back, because `${(%)…}` can
// hold several words and a reader needs to know which — src is that construct
// as written, and empty where a *builtin* asked. A builtin has already been
// named by the location the dialect writes, so it says the sentence alone —
// and it does not set expandErr, because nothing is being expanded and the
// builtin's own status is the answer.
//
// The escape character is the dialect's rather than a percent sign written
// out: the same refusal reaches `${v@P}`, where the language is bash's and an
// escape is spelled `\!`. A message naming `%!` there would send a reader
// looking for a construct their script does not contain.
//
// Setting it there would in fact change nothing observable: the flag is
// cleared at the start of every command, so a builtin cannot leak it into
// the next one, and the builtin's own operands were expanded before it ran.
// It is left out because it would be false rather than because it would
// break, and a mutation that puts it back survives for that reason.
func (r *Runner) refusePromptEscape(src, c string) (string, bool) {
	esc := string(r.promptStyle.Escape)
	if src == "" {
		r.diagf("the %s%s prompt escape is not implemented\n", esc, c)
		return "", false
	}
	r.diagf("${%s}: the %s%s prompt escape is not implemented\n", src, esc, c)
	r.expandErr = true
	return "", false
}

// promptUnitName is `%N`: the name of the function, sourced file or script
// being read — the function's *name* where `%x` stays its defining file.
func (r *Runner) promptUnitName() string {
	if len(r.frames) > 0 {
		f := r.frames[len(r.frames)-1]
		if f.Name != "" && f.Name != sourceFrameName {
			return f.Name
		}
		if f.File != "" {
			return f.File
		}
	}
	if r.scriptFile != "" {
		return r.scriptFile
	}
	return r.name()
}

// quoteFlagged is the `q` family, one style per count — all measured:
// backslashes, then single quotes, double quotes, and `$'…'` — with the two
// modifier styles, `q-` and `q+`, reached through the same door rather than
// beside it, so that a caller cannot pick the count and forget the modifier.
// See interp/minimalquote.go.
//
// mod is the character the group's `q` ate, and 0 when it ate none. It beats
// the count rather than composing with it: `${(q-)v}` and `${(q+)v}` each
// have one `q` and neither writes the single-`q` style.
//
// nothing is substitutedNothing's answer, and reaches only the single-`q`
// style below — which is measured rather than convenient: `"${(q)y:-$y}"`
// with `y=""` is nothing in the shell being modeled, and `(qq)`, `(qqq)`,
// `(qqqq)`, `(q-)` and `(q+)` all write their own empty wrapper for the same
// expansion.
func quoteFlagged(v string, count int, mod byte, nothing bool) string {
	switch mod {
	case '-':
		return quoteMinimal(v)
	case '+':
		return quoteExtended(v)
	}
	switch count {
	case 1:
		return quoteWithBackslashes(v, nothing)
	case 2:
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	case 3:
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(v); i++ {
			if strings.IndexByte("\\`\"$", v[i]) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(v[i])
		}
		b.WriteByte('"')
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("$'")
		eachQuotableByte(v, func(_ int, c byte) {
			switch {
			case c == '\'':
				b.WriteString(`\'`)
			case c == '\\':
				b.WriteString(`\\`)
			case c == '!':
				b.WriteString(`\!`)
			case c < 0x20 || c >= 0x7f:
				b.WriteString(controlEscape(c))
			default:
				b.WriteByte(c)
			}
		}, func(_ int, raw string) { b.WriteString(raw) })
		b.WriteString("'")
		return b.String()
	}
}

// quoteWithBackslashes is the single-`q` style: the characters the shell
// gives meaning to are escaped, each control or non-UTF-8 byte becomes its
// own `$'…'` segment, and an empty value is `”` — every detail measured.
//
// Which characters those are is the table in interp/minimalquote.go, shared
// with `q-` and reached by the `:q` modifier through this function, because
// the question all three ask is the same one and a second copy of the answer
// is how two of them come to disagree. The table is where the two start-only
// specials live: measured, `${(q)…}` on `a~b` is `a~b` and on `~x` is `\~x`.
func quoteWithBackslashes(v string, nothing bool) string {
	if v == "" {
		// `''` is what an empty *value* has to be written as, backslashes
		// having no way to spell one. A word branch that substituted nothing
		// is not that, and is written as nothing — see substitutedNothing for
		// the measurement, and note that this is the only style it reaches.
		// The wrapping styles below write their own empty wrapper for it,
		// measured on zsh 5.9.2 with `y=""`: `"${(qq)y:-$y}"` is `''`,
		// `"${(qqq)y:-$y}"` is `""`, `"${(qqqq)y:-$y}"` is `$''` and the
		// minimal `"${(q-)y:-$y}"` is `''` — every one of them the same
		// answer it gives an ordinary empty value.
		if nothing {
			return ""
		}
		return "''"
	}
	var b strings.Builder
	eachQuotableByte(v, func(i int, c byte) {
		switch {
		case c < 0x20 || c >= 0x7f:
			// Control bytes and bytes that are not UTF-8 alike — measured,
			// `$'\177'` and `$'\377'`. Ahead of the table on purpose: a tab
			// and a newline are in it, and here they are `$'\t'` and `$'\n'`
			// rather than a backslash and a raw byte.
			b.WriteString("$'" + controlEscape(c) + "'")
		case quotableByte(i, c):
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}, func(_ int, raw string) { b.WriteString(raw) })
	return b.String()
}

// eachQuotableByte walks a string handing single bytes — ASCII, and any byte
// that is not part of a valid multibyte rune — to one function and whole
// multibyte runes to the other, because quoting escapes bytes while UTF-8
// passes through untouched.
//
// Both are given the offset the piece starts at, because two of the bytes
// that need quoting need it only at offset 0.
func eachQuotableByte(v string, one func(i int, c byte), run func(i int, s string)) {
	for i := 0; i < len(v); {
		if v[i] < utf8.RuneSelf {
			one(i, v[i])
			i++
			continue
		}
		c, size := utf8.DecodeRuneInString(v[i:])
		if c == utf8.RuneError && size == 1 {
			one(i, v[i])
			i++
			continue
		}
		run(i, v[i:i+size])
		i += size
	}
}

// controlEscape writes one control byte the way `$'…'` spells it: the seven
// named escapes by name and everything else as three-digit octal — `$'\033'`
// for escape rather than `\e`, which is measured.
func controlEscape(c byte) string {
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\v':
		return `\v`
	}
	return fmt.Sprintf(`\%03o`, c)
}

// matchingFlag reports whether the `M` flag was written, which turns the
// operators that *remove* what a pattern matched into ones that keep it.
//
// It reaches exactly two of them, measured across every operator the flag
// group may stand in front of: the four trims, where it substitutes the
// matched part, and `:#`, where it keeps the matching elements instead of
// dropping them. On `/`, `:|`, `:*`, a substring, the conditionals and an
// expansion with no operator at all it does nothing — which is why there is
// no third call site rather than an oversight.
func matchingFlag(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsRune(e.Flags, 'M')
}
