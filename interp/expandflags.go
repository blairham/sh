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
const implementedParamFlags = "ULC#fsjF@kvP%qMuoOniaQbcwWA~Zze-lr0VtSmBENRXID"

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
	r.flagWordBare, r.flagKeepsBare = nil, false
	words, isList, escaped, ok := r.flaggedWords(e, sp, quoted, escapeSep)
	bare, keepsBare := r.flagWordBare, r.flagKeepsBare
	r.flagWordBare, r.flagKeepsBare = nil, false
	// Nothing below this line is a list of a word's fields until the loop at
	// the end says so, and the marks belong to the call that is reading them.
	// See interp/emptynullfield.go.
	r.listNulls, r.listEdges = nil, listEdges{}
	if !ok {
		return nil, true
	}
	for i, w := range words {
		// A `:s` replacement's deferred expansions, now that the flags have
		// run on its source. See pendingMark.
		words[i] = r.resolvePending(w)
	}
	if !escaped {
		// A `:s` replacement's live marks are read here, where the words
		// are escaped, and nowhere further on. See liveMark.
		for i, w := range words {
			if strings.Contains(w, liveMark) && quoted {
				words[i] = stripLiveMarks(w)
			}
		}
	} else {
		for i, w := range words {
			words[i] = stripLiveMarks(w)
		}
	}
	if !isList {
		v := words[0]
		if quoted {
			if !escaped {
				v = r.globEscape(v)
			}
			return []string{v}, true
		}
		if v == "" {
			// An unquoted expansion of an empty value is no field at all.
			return nil, true
		}
		if !escaped && strings.Contains(v, liveMark) {
			v = r.equalsHeadMark(head, v) + escapeWithLiveMarks(v)
		} else if !escaped &&
			!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			v = r.globEscape(v)
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
	//
	// An IFS split keeps its empty fields unquoted too, which an `(s)` split
	// does not. Measured 2026-10-01 on zsh 5.9.2 with `IFS=:` and
	// `u='a::b:'`: `${(@)=u}`, `${(U)=u}` and `${(@)=u:-x}` are all
	// `[a][][b][]`, the four fields the flag-free `${=u}` already gave here,
	// while `${(s.:.)u}` and `${(@s.:.)u}` are `[a][b]` — so the empties are
	// the IFS split's own, and an unquoted array's empty *elements* still go:
	// `a=(x '' y); ${(@)a}` is `[x][y]` (#5275).
	// An `=` split keeps its empty fields because they are bare, and a group
	// that unquoted them has made them values — which then go the way a
	// letter split's empty fields go. Measured on zsh 5.9.2 with IFS=: and
	// `u=a::b:`: `${(Q)=u}` is `a` and `b`, `"${(Q)=u}"` is `a`, `b` and the
	// edge, `"${(@Q)=u}"` keeps all four. See flaggedWords.
	eqSplit := splitFlagInGroup(e, sp)
	unquotedSplit := eqSplit && strings.ContainsRune(e.Flags, 'Q')
	if unquotedSplit {
		eqSplit = false
	}
	// And a nested `=` split's bare fields are kept unquoted for the same
	// reason, which flaggedWords says through keepsBare.
	eqSplit = eqSplit || keepsBare
	keepEmpty := (quoted || r.expandingNestedInner) &&
		(r.flagKeepsFields(e) || eqSplit) ||
		!quoted && eqSplit
	// And a quoted `(f)` or `(s)` keeps the empty field at each *edge* while
	// still dropping the interior ones, which is the same rule `${=spec}`
	// already follows for an IFS split and was measured separately for these
	// two flags — see splitFlagEdges.
	edges := !keepEmpty && (splitFlagEdges(e, quoted) || unquotedSplit && quoted)
	out := make([]string, 0, len(words))
	nulls := make([]bool, 0, len(words))
	for i, w := range words {
		// Named rather than negated inline: `!(edges && …)` reads as a
		// double negative at the point it matters most, and the condition
		// it stands for — "this empty field is one the flag keeps because
		// it is at an end" — is the whole reason the branch exists.
		atKeptEdge := edges && (i == 0 || i == len(words)-1)
		// Unquoted, what an `=` split keeps is the bare field and only that:
		// an empty *value* is no word, so `${=e}` with `e=` is nothing while
		// `${=d}` with `d=:` is two empty words.
		valueGone := !quoted && eqSplit && !r.expandingNestedInner && w == "" && bare != nil && !bare[i]
		if w == "" && (!keepEmpty || valueGone) && !atKeptEdge {
			// Not removed here: it is a field of the word until the word
			// says otherwise, exactly as an empty element of an unquoted
			// array is. Measured on zsh 5.9.2 — `v='::b'; x${(s.:.)v}y` is
			// `[x][by]`, so the first of the two nulls is where the `x`
			// ends, and `b=('' 2); x${(o)b}y` is `[x][2y]` for the same
			// reason the unflagged `x${b}y` is. See
			// interp/emptynullfield.go.
			out = append(out, "")
			nulls = append(nulls, true)
			continue
		}
		if strings.Contains(w, liveMark) {
			w = escapeWithLiveMarks(w)
		} else if quoted || !r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			w = r.globEscape(w)
		}
		out = append(out, w)
		nulls = append(nulls, false)
	}
	r.listNulls = nulls
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
		//
		// What is quoted is the rest of the *word* the group stands in and
		// not the expansion alone (#1647) — see ParamExpr.FlagsErrTail for
		// the measurements, and for the shapes where there is no word to
		// recover and the expansion is all there is to say. A control
		// character in that text is made visible rather than written into
		// the diagnostic, which is measured on the same shell: a tab is
		// `\t`, a newline `\n`, and a backslash is left alone — the same
		// rendering `(V)` uses, so the two share one function.
		text := e.FlagsErrTail
		if text == "" {
			text = "${" + e.Src + "}"
		}
		r.diagf("%s\n", Wording(r.diag().ExpansionFlagsError,
			"error in flags near position %[1]d in '%[2]s'",
			e.FlagsErrPos, visibleText(text)))
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
	if strings.ContainsRune(e.Flags, 'I') && e.Op == syntax.ParamReplace {
		// `I` is carried for a searching trim, which is what reads it — see
		// matchIndexFlag. Over a substitution it picks which match is
		// replaced, and that half is not built: refused by name rather than
		// answered with the first match at status 0.
		r.diagf("${%s}: the (I) expansion flag is not implemented beside a substitution\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	if reportsAboutTheMatch(e) && reportsAnElementOperator(e.Op) {
		// The four reporting flags reach the trims and are refused over an
		// operator that reshapes a list. They do something there and the
		// family does not do it alike — see interp/reportflags.go for the
		// measurement — so a projection of the trim's span would be a
		// plausible value at status 0 rather than an answer.
		r.diagf("${%s}: the (%c) expansion flag is not implemented beside this operator\n",
			e.Src, firstOf(e.Flags, "BENR"))
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
	if arrayAssignFlag(e) && isAssignOp(e.Op) && strings.ContainsRune(e.Flags, 'P') {
		// `(A)` beside a `(P)`: the array would be the *target* the
		// indirection names, and that target is a text rather than a name —
		// it may spell an element or a reference, each of which stores
		// differently. Measured on zsh 5.9.2, `v=tgt; ${(AP)v::=x y}` leaves
		// `tgt` a one-element array, and the shapes beside it are a
		// measurement this does not carry. Named rather than stored as a
		// scalar, which is what the pair did before the single letter's
		// assignment was carried at all.
		r.diagf("${%s}: the (A) expansion flag is not implemented beside a (P) for an assignment\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	if assocAssignFlag(e) && isAssignOp(e.Op) {
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
		// The single letter's assignment is carried now — see
		// interp/arrayassignflag.go, which is the other half of the rows
		// above. **The doubled letter is not**, and that is a narrowing
		// rather than an omission: `(AA)` makes the name an *association*,
		// so the value's fields have to pair off and an odd count is a
		// refusal with wording of its own — `${(AA)u=k v}` on zsh 5.9.2 is
		// `bad set of key/value pairs for associative array`, the one word
		// `k v` being an odd number of fields, where `${(AA)=u=k v}` splits
		// into two and builds the table. That is a separate measurement and
		// this does not carry it, so the letter is named rather than left to
		// look like it worked: an assignment silently making an indexed
		// array where the script asked for a table is the shape a later
		// `${u[k]}` reads as empty.
		//
		// Refused for the *operator* rather than for the assignment actually
		// firing, which is deliberate. `${(AA)u=k v}` assigns nothing when
		// `u` is already set, so a check on whether it fired would refuse a
		// line on one run and carry it on the next, and the reader would have
		// nothing to go on. The refusal is a statement about the construct.
		r.diagf("${%s}: the (AA) expansion flag is not implemented for an assignment\n", e.Src)
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
		if indirect != nil && indirect.set {
			// The name a `(P)` resolved to is what is unset, and it is named
			// as the text it came from, which for an array is its elements
			// joined: measured 2026-10-03 on zsh 5.9.2, `setopt nounset;
			// n=(a b); ${(P)n}` is `a b: parameter not set`, `s=a; ${(P)s}`
			// is `a: …` and `n=(); ${(P)n}` is `: …`. An unset base is
			// named as itself: `${(P)nope}` is `nope: …`.
			saved := r.unboundByIndirection
			r.unboundByIndirection = &indirect.text
			r.checkNounset(e)
			r.unboundByIndirection = saved
		} else {
			r.checkNounset(e)
		}
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
	// A `(P)` whose resolved text is an `[@]` subscript keeps its fields
	// here for the same reason `"${a[@]}"` does — the `@` is written, and
	// the reference is that expansion. See referenceKeepsFields.
	refFields := indirect != nil && r.referenceKeepsFields(indirectName(indirect.text))
	// A context with room for exactly one word — an assignment's value, a
	// `case` subject, a `[[ ]]` operand, a here-string — joins too, and
	// **not here**: the join it does is rule 10's, below the operator. See
	// there for the measurements that fix the position, and for #1705, which
	// is what the join leaves the ordering flags nothing to order.
	//
	// The inner of a nesting arrives with the same policy and is not one of
	// these contexts: what it comes to is read by the operator around it
	// rather than by a command line, so its fields are values and not words
	// — the same split expandingNestedInner already draws for the empty ones
	// just below. Measured: `a=(one '' two)` and `"${(j:,:)${(@)${a[@]}}}"`
	// is `one,,two`, three fields reaching the join, where a scalar context
	// would have handed it one.
	scalarContext := sp == splitNever && !r.expandingNestedInner
	joined := false
	if quoted && isList && !e.Length && !r.flagKeepsFields(e) && !markJoin && !refFields {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
		joined = true
	}

	// Rule 7: the operator, applied to the value at this level. Measured:
	// the flags apply to what the operator leaves — `${(U)x:-def}` is DEF,
	// `${(U)u:=def}` assigns def and substitutes DEF.
	// **The empty elements of a nested list are bare**, the way the empty
	// fields a split makes are: they are what an expansion came to, not
	// values the script held. Measured on zsh 5.9.2 (`-f`, `LC_ALL=C`) with
	// `b=(x '' y)`, `IFS=:` and `u=a::b:` (#5299):
	//
	//	"${(@q)${b[@]}}"         x, nothing, y   the empty left bare
	//	"${(@qq)${b[@]}}"        'x', '', 'y'    only the single `q` style
	//	${(@q)${b[@]}}           x, y            unquoted, gone before any flag:
	//	${(@qq)${b[@]}}          'x', 'y'        even the quoting that would
	//	${#${b[@]}}              2               have kept it, and the count
	//	${(q)${(s.:.)u}}         a, b
	//	"${(@q)${b[2]}}"         ''              a nested *value* is a value
	//	s=''; "${(@q)${(@)s}}"   ''              a scalar under (@) is one too
	//
	// So it is the elements of an array or a split a level down that are
	// bare, and not every word a nested expansion hands back. An operator in
	// the group can turn a word into an empty one, which is a value, so the
	// marks are kept only where no operator ran.
	var nestedBare []bool
	keepsBare := false
	if e.Inner != nil && isList && indirect == nil && e.Op == 0 {
		// Unquoted, a list's empty elements never got here — see
		// unquotedInnerDropsItsEmpties — so the marks are for the quoted
		// list and for an `=` split a level down, which keeps its bare
		// fields unquoted as one in the group itself does: `${(q)${=u}}` and
		// `${(@)${=u}}` are four words, two of them empty.
		if kind := r.nestedElementsAreBare(e); kind == nestedBareFromAnIFSSplit ||
			(kind == nestedBareFromAList && quoted) {
			nestedBare = bareEmptyMarks(words, nil)
			keepsBare = !quoted
		}
	}

	words, isList, ok, nothing := r.applyFlagOp(e, words, set, isList, indirect, quoted)
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
		ghosts := (set || isList) && r.nestedEmptiesAreGhosts(e)
		words, isList = []string{itoa(r.flaggedLength(e, words, isList, ghosts))}, false
	}

	// An `=` beside the group is this same step with IFS for a separator.
	ifsSplit := splitFlagInGroup(e, sp)
	if r.arrayAssignSplit == e {
		r.arrayAssignSplit = nil
		ifsSplit = false
	}
	// The two splits are kept apart because rule 10 below treats them
	// differently: `(@)` exempts a letter split from the join ahead of it and
	// leaves the `=` one joining. Only rule 11 wants them together.
	// And never in a scalar context, which is the rule splitFlagInGroup
	// already follows for the `=` spelling and which `f`, `s`, `0` and `p`
	// follow with it. Measured on zsh 5.9.2, 2026-09-12, with `v=c,a,b` and
	// `w=$'c\na\nb'`:
	//
	//	x=${(s:,:)v}                c,a,b      the assignment takes one word
	//	printf '[%s]' ${(s:,:)v}    [c][a][b]
	//	printf '[%s]' "${(s:,:)v}"  [c][a][b]  quoting is not what decides it
	//	x=${(f)w}                   the three lines, unsplit
	//	x=${(@s:,:)v}               c,a,b      nor does the `@` letter
	//
	// The second and third rows are the discriminating pair: a split the
	// quoting turned off would have answered one field in the third, and a
	// split nothing turns off would have answered three in the first.
	letterSplit := !scalarContext && strings.ContainsAny(e.Flags, splitFlagLetters)
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
	//
	// A scalar context joins here whatever the group asked for, and this is
	// the position rather than rule 5 — the operator above has already run
	// on the elements. Measured on zsh 5.9.2, 2026-09-12, with `y=(ab ab)`,
	// `z=(x y)` and `q=(one two)`:
	//
	//	x=${y#ab}          ` `      each element trimmed, then joined
	//	x="${y#ab}"        ` ab`    where quoting joins first, at rule 5
	//	x=${(j:+:)y#ab}    `+`      and the separator the group named
	//	x=${(@)z:/x/Q}     `Q y`    the element operators run elementwise
	//	x=${(o)q:#one}     `two`    all four of them
	//	x=${(q)$(f)}       `b\ b\ a\ a\ c`  and the quoting sees one word
	//	x=${(l:3::_:)$(h)} `1 2`    as does the pad, `3 1 2` clipped to three
	//
	// Rows one and two are each other's control: the same characters in the
	// same assignment, parting on where the join sits. Rows six and seven
	// are what fixes the *lower* bound — every step below this one has to
	// see the joined word — and #1705 is the ordering step among them, which
	// finds one word and leaves it alone.
	if (joinFlagWritten(e.Flags) != 0 || ifsSplit || scalarContext ||
		(letterSplit && !strings.ContainsRune(e.Flags, '@'))) &&
		!joined && isList && !markJoin {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
	}

	// Which of the words are *bare* empties: an empty field the split below
	// made, as against an empty value. The single-`q` style quotes only the
	// second — see bareEmptyMarks. Nil when the group does not split, and
	// nothing ahead of the split can have made one.
	bare := nestedBare
	if joined {
		bare = nil
	}

	// Rule 11: splitting. `f` is split-at-newlines; an empty `s` separator
	// splits into characters, which is measured.
	if hasSplit {
		var split []string
		var splitBare []bool
		for i, w := range words {
			from := len(split)
			if w == "" {
				// An empty word splits into itself, in either context, and
				// keeps what it was: measured, unquoted `${(q)=e}` with
				// `e=` is the one word `''`, where `${=e}` is no word at
				// all — the empty value going the way an unquoted empty
				// value goes once the group is done with it.
				split = append(split, "")
				splitBare = append(splitBare, bare != nil && bare[i])
				continue
			}
			if ifsSplit {
				// `${=spec}` splitting, which is field splitting on IFS and
				// not a separator the group named. Quoted it keeps the
				// fields at the edges — see interp/splitflag.go.
				//
				// The words here are the text they came to, not the
				// escaped form — the escaping happens after the group has
				// run — so they are split as plain text. Reading them as
				// escaped took a value's backslash for a quote and a
				// value's NUL for the value-backslash mark, so `${(@)=v}`
				// never split at the NUL zsh's default IFS holds, and an
				// IFS of a backslash split at the NUL instead (#5263).
				ifs, set := r.ifs()
				split = append(split, r.splitFieldsAsking(w, nil, ifs, set, quoted, false)...)
			} else {
				split = append(split, r.splitFlagged(w, e)...)
			}
			// What a split of a word with something in it leaves empty is a
			// bare field; a word that was already empty splits into itself
			// and keeps what it was. Measured on zsh 5.9.2 with IFS=:, `d=:`
			// and `e=`: `${(@q)=d}` is two empty words, `${(@q)=e}` is `''`.
			splitBare = bareEmptyMarks(split[from:], splitBare)
		}
		words, isList, bare = split, true, splitBare
	}

	// Rules 12, 13, 14 in the manual's order: case, prompt escapes, quoting.
	//
	// Each letter in the order it was written, so the last of `U`, `L` and
	// `C` is what the value ends as: measured on zsh 5.9.2 with `a="xyZ
	// abC"`, `${(CU)a}` is `XYZ ABC` and `${(UC)a}` is `Xyz Abc`.
	for _, c := range e.Flags {
		switch c {
		case 'U', 'L':
			for i, w := range words {
				words[i] = r.convertCase(w, c == 'U')
			}
		case 'C':
			for i, w := range words {
				words[i] = r.capitalizedRuns(w)
			}
		case '#':
			for i, w := range words {
				c, complaint := r.characterForCodeReporting(w)
				if complaint != "" && reportsFlagErrors(e) {
					r.diagf("%s\n", complaint)
					r.expandErr = true
					return nil, false, false, false
				}
				words[i] = c
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
		// And what the prompt escapes come to is a value: measured,
		// `${(@q%)=u}` quotes the empty fields `${(@q)=u}` leaves bare.
		bare = nil
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
			doubled := r.DoubledQuoteInSingleQuotes()
			empty := nothing || bare != nil && bare[i]
			if strings.Contains(w, liveMark) {
				// An operand's own pattern characters: the single `q`
				// leaves them as they are, live, and every other style
				// quotes them with the rest. See operandTokenWords.
				if n == 1 && e.QuoteModifier == 0 {
					words[i] = quoteAroundLiveMarks(w, empty, r.historyQuoting().plain)
				} else {
					words[i] = quoteFlagged(stripLiveMarks(w), n, e.QuoteModifier, empty, doubled, r.historyQuoting())
				}
				continue
			}
			words[i] = quoteFlagged(w, n, e.QuoteModifier, empty, doubled, r.historyQuoting())
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
		// What the unquoting comes to is a value, as the prompt escapes'
		// is: measured, unquoted `${(Q)=u}` is `a` and `b` where `${=u}`
		// keeps the two empty fields, and quoted `"${(Q)=u}"` keeps only the
		// one at the edge, exactly as a letter split does. See expandFlagged.
		bare = nil
		for i, w := range words {
			v, complaint := r.unquoteFlaggedReporting(w)
			if complaint != "" && reportsFlagErrors(e) {
				r.diagf("%s\n", complaint)
				r.expandErr = true
				return nil, false, false, false
			}
			words[i] = v
		}
	}
	// `D`: each value written as a directory the way `%~` draws one — under
	// the home as `~`, under a named directory as `~name` — and then quoted
	// as a single `q` would quote it. Measured 2026-10-05 on zsh 5.9.2
	// under -f, with `HOME=/Users/x`, `hash -d proj=/opt/p pp=/opt/p/src`:
	//
	//	a=/Users/x/doc     ${(D)a}          ~/doc
	//	b=/opt/p/src       ${(D)b}          ~pp     the shortest drawing wins
	//	c=/tmp             ${(D)c}          /tmp
	//	v='/Users/x/a b'   ${(D)v}          ~/a\ b  quoted, abbreviated or not:
	//	                   ${(D):-/q/it's}  /q/it\'s
	//	x='~/q'            ${(D)x}          \~/q
	//	e=                 "[${(D)e}]"      []      and no '' for an empty one
	//	                   ${(UD)a}         /USERS/X/DOC  after the case,
	//	                   ${(qqD)v}        \'/Users/x/a\ b\'  the quoting,
	//	                   ${(DQ):-/Users/x/a\\ b}   ~/a\ b  and the unquoting
	//	                   ${(D)a:h}        ~       and after the operator
	//	arr=(/Users/x /tmp) "${(D)arr}"     joined first, then drawn
	//
	// So it is the last of the value's rewrites, and it is the directory
	// drawing `print -D` and the prompt already share (#5980).
	if strings.ContainsRune(e.Flags, 'D') {
		bare = nil
		doubled := r.DoubledQuoteInSingleQuotes()
		for i, w := range words {
			w = stripLiveMarks(w)
			// The tilde word the drawing put in front is not quoted, and the
			// rest is: `~/a\ b` and not `\~/a\ b`, while a `~` that was in
			// the value is quoted like any other character.
			drawn, head := r.abbreviatedDirectory(w), ""
			if drawn != w {
				tail := drawnTail(drawn)
				head, drawn = drawn[:len(drawn)-len(tail)], tail
			}
			if drawn != "" {
				drawn = quoteFlagged(drawn, 1, 0, false, doubled, r.historyQuoting())
			}
			words[i] = head + drawn
		}
	}
	// And after the quoting, which is the order `${(Vq)}` measures: a tab
	// comes out `a$'\t'b`, so the quoting saw the control character and
	// this did not. See visibleflag.go.
	//
	// After the unquoting and the directory drawing too, which is measured
	// on zsh 5.9.2 (2026-10-05): with `t=$'a\tb'`, `${(VQ)t}` is `a\tb` —
	// a `Q` that ran after this would have taken the backslash and left
	// `atb`, which is what this answered while it stood ahead of `Q` — and
	// with `HOME=/Users/x`, `${(DV)t}` on `/Users/x/a<TAB>b` is
	// `~/a$'\t'b`, the `D` quoting having seen the tab (#5980).
	if strings.ContainsRune(e.Flags, 'V') {
		for i, w := range words {
			words[i] = r.visibleFlagText(w)
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
	// The shell split is the one step below rule 10 that can hand a scalar
	// context more than one word, so the join is repeated here — and this is
	// the last point that can see it, since the two steps below both read
	// the words this leaves. Measured on zsh 5.9.2, 2026-09-12, `u='b a'`:
	//
	//	x=${(oZ+n+)u}          `b a`   the ordering finds one word
	//	x=${(l:3::_:Z+n+)u}    `b a`   and so does the pad, `b a` being three
	//	printf '[%s]' ${(oZ+n+)u}  [a][b]   where a list context sorts
	//	IFS=: x=${(Z+n+)u}     `b:a`   joined on IFS
	//	x=${(j:+:Z+n+)u}       `b a`   and on IFS even where `j` named one
	//
	// The last row is why this is not rule 10's join reached twice: that one
	// honors the separator the group asked for and this one does not.
	if scalarContext && isList {
		words, isList = []string{strings.Join(words, r.ifsFirst(r.ifs()))}, false
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
	// A sort moves the words and not their bare marks, so the marks are made
	// again behind it where they can be: where every empty word was bare, the
	// empty words are still exactly the bare ones. Measured on zsh 5.9.2 with
	// `IFS=:` and `u=a::b:`: `${(@o)${=u}}` is the four words `a`, `b` and two
	// empty ones, where dropping the marks lost both.
	marksFollowSort := false
	if orderApplies(e) {
		marksFollowSort = len(bare) == len(words) && emptiesAllBare(words, bare)
		words = r.orderWords(e, words)
		if marksFollowSort {
			bare = bareEmptyMarks(words, nil)
		}
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
		// Rule 23's single word, when it applied, leaves as many words as it
		// was given, so what the result *is* has not changed; without it
		// every field the re-reading made is a word of its own. The count is
		// checked rather than assumed, because the one thing that survives
		// the quoting is `"$@"`: `set -- one "two three" four; a='$@'` makes
		// `"${(e)a}"` three words and an empty `$@` makes it none, so a
		// result taken as a single word here would keep only the first of
		// the three and index past the end of the none.
		if !joined || len(fields) != len(words) {
			isList = true
		}
		words = fields
	}
	// The bare marks, for the one reader that decides which empty words the
	// command line keeps — see expandFlagged. Only while they still line up
	// with the words: a sort, a re-reading or a shell split moves them.
	r.flagWordBare, r.flagKeepsBare = nil, false
	if len(bare) == len(words) && (!orderApplies(e) || marksFollowSort) && !reevalFlagApplies(e) && !markJoin {
		if _, shell := shellSplitOpts(e); !shell {
			r.flagWordBare, r.flagKeepsBare = bare, keepsBare && bare != nil
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
func (r *Runner) assignThroughFlags(e *syntax.ParamExpr, indirect *indirectTarget, quoted bool) ([]string, bool, bool) {
	// The `(A)` flag makes this an *array* assignment over the words the
	// value comes to, and only where the name itself is what is being
	// written: a `(P)` has moved the name along and a written subscript aims
	// at one element, and neither of those is the array. The branch is taken
	// before the text is built, because the operand is expanded to fields
	// there instead and a command substitution in it must run once. See
	// interp/arrayassignflag.go.
	if arrayAssignFlag(e) && indirect == nil && !writesOneElement(e, r) {
		elems := r.assignedArrayElements(e, quoted)
		if !r.assignableTarget(e.Op, e.Name) {
			return nil, false, false
		}
		r.setArray(e.Name, elems)
		// The group's `=` has split the operand already, element by element
		// as it expanded, so what is substituted is not split a second time:
		// measured on zsh 5.9.2, `${(A)=foo=a "k p" b}` substitutes `a`,
		// `k p` and `b` — the three elements it stored — where splitting the
		// yield again gave four (#5151).
		r.arrayAssignSplit = e
		if quoted {
			// One word, the elements joined — measured with `a=(1 2)`,
			// `print -rl -- "${(A)u=$a}"` is the one line `1 2`. So the
			// yield joins under quotes the way `$*` does and not the way a
			// quoted `${a[@]}` does, which would keep a field each.
			return []string{strings.Join(elems, " ")}, false, true
		}
		// Unquoted, the elements are the fields: `print -rl -- ${(A)u=$a}`
		// is two lines where a joined word would be one.
		return elems, true, true
	}
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
	case writesOneElement(e, r):
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

// writesOneElement reports whether a written subscript aims this assignment
// at one element rather than at the name. One reading in two places, because
// the `(A)` branch above and the subscript branch below have to agree about
// which of them a `${(A)u[2]=x y}` belongs to — measured, it is the element's:
// zsh 5.9.2 leaves `u` a two-element array with `x y` in the second.
func writesOneElement(e *syntax.ParamExpr, r *Runner) bool {
	return e.Index != nil && !r.wholeArrayIndex(e)
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

// joinFlagLetters are the two letters that name a join: `j`, which carries
// its separator as an argument, and `F`, whose separator is a newline.
//
// `F` is the vendor manual's own shorthand for `pj:\n:` and is carried by
// being read as one, rather than as a second join with a rule list of its
// own — the whole of the difference is which separator joinFlagWritten hands
// back. Measured on zsh 5.9.2, 2026-09-25, with `a=(x y z)`:
//
//	${(F)a}          x\ny\nz   the shorthand
//	${(pj:\n:)a}     x\ny\nz   what it is short for
//	printf '[%s]' ${(F)a}  one field — it joins unquoted too, as `j` does
//	${(@F)a}         one field, where `"${(@)a}"` is three
//
// The last two are the rows that say it is the *same* join: both are `j`'s
// answer and neither is what a flag that only acted under quoting would give.
const joinFlagLetters = "jF"

// joinFlagWritten is the join letter this group's separator comes from, or 0
// where the group named no join at all.
//
// The **last** of the two written wins, which is measured rather than
// assumed — the two letters fill one slot, so a `j` behind an `F` replaces
// the newline and an `F` behind a `j` replaces the argument. On zsh 5.9.2,
// 2026-09-25, with `a=(x y z)`:
//
//	${(Fj:-:)a}       x-y-z     the `j` behind the `F`
//	${(j:-:F)a}       x\ny\nz   the `F` behind the `j`
//	${(Fj:-:F)a}      x\ny\nz   and the last one written either way
//	${(j:-:Fj:+:)a}   x+y+z
//
// A repeated `j` is already last-wins in the parser, which keeps only the
// final argument; this is the same rule reaching across the two spellings.
func joinFlagWritten(flags string) rune {
	i := strings.LastIndexAny(flags, joinFlagLetters)
	if i < 0 {
		return 0
	}
	return rune(flags[i])
}

// flagJoinSep is what joining uses: the `j` argument when one was given, a
// newline for the `F` that is short for one, and the first character of IFS
// — a space by default — when the group named neither.
//
// `F`'s newline is not read through flagArgument: there is no argument to
// read, the escape the shorthand stands for having been spent on the letter.
// A `(p)` beside it therefore changes nothing, and neither does `$IFS` —
// measured, `IFS=-` leaves `"${(F)a}"` joined on newlines.
func (r *Runner) flagJoinSep(e *syntax.ParamExpr) string {
	switch joinFlagWritten(e.Flags) {
	case 'j':
		return r.flagArgument(e, 'j', e.JoinSep)
	case 'F':
		return "\n"
	}
	return r.ifsFirst(r.ifs())
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
		if keys, ok := r.wholeTableKeys(e); ok {
			return keys, len(keys) > 0, true
		}
		if list, lok := r.arraySubscript(e); lok {
			if r.wholeArrayIndex(e) {
				if isAssoc := r.assocDeclared(e.Name); isAssoc {
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
			if len(list) == 0 && r.subscriptBeforeTheFirst {
				// The place before the first element, which is no value
				// rather than an empty one: a group that keeps its fields
				// makes no word of it in quotes. See
				// Runner.subscriptBeforeTheFirst.
				return []string{}, false, true
			}
			return []string{strings.Join(list, " ")}, list != nil, false
		}
	}
	return r.namedBase(e.Name, baseFlags(e.Flags))
}

// wholeTableKeys is `${(k)m[@]}` on a produced association that can name its
// keys without producing its values: the answer the branch below reaches
// through namedBase, taken before arraySubscript reads every value of the
// table only for that branch to set them aside.
//
// Only that one shape. A chain, or `v` beside the `k`, or a table something
// has stored over, all go the long way — producedAssocKeys refuses the last,
// and the first two need the values.
func (r *Runner) wholeTableKeys(e *syntax.ParamExpr) ([]string, bool) {
	if len(e.Leading) > 0 || !r.wholeArrayIndex(e) {
		return nil, false
	}
	flags := baseFlags(e.Flags)
	if !strings.ContainsRune(flags, 'k') || strings.ContainsRune(flags, 'v') {
		return nil, false
	}
	return r.producedAssocKeys(e.Name)
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
		p := r.params()
		return append([]string(nil), p...), len(p) > 0, true
	case "":
		return []string{""}, false, false
	}
	if r.assocDeclared(name) {
		hasK := strings.ContainsRune(flags, 'k')
		hasV := strings.ContainsRune(flags, 'v')
		if hasK && !hasV {
			// The names alone, which a produced table may be able to give
			// without producing a single value — see SetDynamicAssocKeys.
			if keys, ok := r.producedAssocKeys(name); ok {
				return keys, len(keys) > 0, true
			}
		}
		a, _ := r.assocFor(name)
		switch {
		case hasK && hasV:
			keys := r.assocKeys(name, a)
			out := make([]string, 0, 2*len(keys))
			for _, k := range keys {
				out = append(out, k, a[k].scalar())
			}
			return out, len(a) > 0, true
		case hasK:
			return r.assocKeys(name, a), len(a) > 0, true
		default:
			return r.assocValues(name, a), len(a) > 0, true
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
	if isAssoc := r.assocDeclared(e.Name); !isAssoc {
		return "", false
	}
	if e.IndexFlags != nil {
		// A flag group selects by matching rather than by naming, and which
		// half of each match it substitutes is assocSearchWords' question —
		// already answered there, for the same two letters.
		return "", false
	}
	return r.assocKeyRead(e), true
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
	indirect *indirectTarget, quoted bool,
) ([]string, bool, bool, bool) {
	fires := !set
	if e.Colon {
		fires = !set || strings.Join(words, "") == ""
	}
	switch e.Op {
	case syntax.ParamNone:
	case syntax.ParamDefault:
		if fires {
			if sub, ok := r.operandTokenWords(e, quoted); ok {
				return sub, len(sub) != 1, true, substitutedNothing(e, sub, fires)
			}
			sub := []string{r.joinWord(e.Arg)}
			return sub, false, true, substitutedNothing(e, sub, fires)
		}
	case syntax.ParamAssign:
		if fires {
			w, l, ok := r.assignThroughFlags(e, indirect, quoted)
			return w, l, ok, false
		}
	case syntax.ParamAssignAlways:
		// No test, so the assignment is the only branch there is. The flags
		// still apply to what is substituted and not to what is stored:
		// measured, `${(U)v::=abc}` is `ABC` and leaves `abc` behind.
		w, l, ok := r.assignThroughFlags(e, indirect, quoted)
		return w, l, ok, false
	case syntax.ParamAlternate:
		if fires {
			return []string{""}, false, true, false
		}
		if sub, ok := r.operandTokenWords(e, quoted); ok {
			return sub, len(sub) != 1, true, substitutedNothing(e, sub, fires)
		}
		sub := []string{r.joinWord(e.Arg)}
		return sub, false, true, substitutedNothing(e, sub, fires)
	case syntax.ParamError:
		if fires {
			subject := r.paramErrorSubject(e)
			if indirect != nil && indirect.set {
				// The resolved name, as the nounset refusal above names it:
				// `n=(a b); ${(P)n:?boom}` is `a b: boom`.
				subject = indirect.text
			}
			r.fatalParamError("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				subject, r.paramErrorWord(e, set)))
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
		// `(B)`, `(E)`, `(N)` and `(R)` join `(M)` here, and they
		// accumulate rather than replace: written together they report one
		// match from several sides, in a fixed order that is not the order
		// they were written. So the five leave through one function holding
		// one span — see interp/reportflags.go — and `(M)` alone still
		// takes that route, which is what keeps the pair from drifting.
		//
		// `(S)` is the other flag this operator reads, and the node carries
		// it rather than a second `take` here: it changes *which* match the
		// trim found and not what is reported about it, so the flags
		// compose without either knowing about the other. See
		// interp/searchflag.go.
		if want := matchReportFlags(e); want != "" {
			for i, w := range words {
				words[i] = r.trimReport(w, pattern, e, want)
			}
			break
		}
		for i, w := range words {
			words[i] = r.trimWith(w, pattern, e)
		}
	case syntax.ParamReplace:
		pattern := r.patternOf(e.Arg)
		for i, w := range words {
			words[i] = r.replaceWith(w, pattern, e)
		}
	case syntax.ParamSubstring:
		if isList {
			// A range whose segment is a modifier list applies to **each
			// element** rather than slicing the list. Measured on zsh 5.9.2,
			// 2026-09-18 with `b=(/a/x.c /b/y.c 'p q')`: `${b:t}` is three
			// words `x.c y.c 'p q'`, `${b:q}` is three with the last one
			// escaped, and `${b:1}` is still the two elements from index one.
			// So which of the two a range is decides before the elements are
			// touched, exactly as it does for a scalar.
			if out, ok := r.modifiedElements(words, e); ok {
				return out, true, true, false
			}
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

// characterForCode is the `(#)` flag over one word's value: the character
// the number is the code of. Under a locale whose characters are counted that
// is the character's UTF-8 encoding — and the encoding's original six-byte
// reach, not only what Unicode assigns, since the value is not checked — and
// otherwise the one byte. Measured 2026-10-02 on zsh 5.9.2:
//
//	65   A        233  é, and 351 under LC_ALL=C     128  302 200
//	55296          ed a0 80, a surrogate encoded as it stands
//	1114112        f4 90 80 80, past the last code point
//	2147483647     fd bf bf bf bf bf
//	4294967361     A: the value is taken as 32 bits
//	-1  ff   -200  38   -256  00: a negative value is its low byte
//	abc            NUL: the word is an expression, and an unset name is 0
//
// characterForCodeReporting is that over a word not yet evaluated. An
// expression that will not evaluate is no character at all, and no complaint
// either unless the `X` flag asks for one: measured, `x='1+'; print -n ${(#)x}`
// and the same over `1/0` write nothing at status 0, and `a=(65 '1+' 66);
// print ${(#)a}` writes `A B`. The complaint is the sentence `$(( ))` would
// write — a failure to read the expression and a failure to evaluate it are
// worded apart, as there. See reportsFlagErrors.
func (r *Runner) characterForCodeReporting(w string) (string, string) {
	tree, text, err := r.arithTreeOver(nil, w, arithTextArrived)
	if err != nil {
		return "", r.diag().ParseFailure(err)
	}
	n, err := r.evalNum(tree)
	if err != nil {
		return "", r.arithFailure(text, err)
	}
	if code := n.asInt(); code >= 0x80 && r.sem().MultibyteEncodingIsHonored == Yes && r.localeIsASCII() {
		// Only the `X` flag reports it; without one the byte is written.
		// A code the locale has no character for. Measured 2026-10-03 on
		// zsh 5.9.2 under `-f`: `${(#X):-0x80}` is `character not in range`
		// under LC_ALL=C, LANG=POSIX, LC_CTYPE=C or no locale at all, where
		// `(#)` alone writes the byte, `nomultibyte` takes any byte, and
		// en_US.ISO8859-1 and any UTF-8 locale have one (#5151, a chunk of
		// D04parameter.ztst).
		return r.characterForCode(code), "character not in range"
	}
	return r.characterForCode(n.asInt()), ""
}

// localeIsASCII reports whether the character locale in force is C or POSIX,
// a locale nothing names included.
func (r *Runner) localeIsASCII() bool {
	for _, name := range localeVariables {
		if v, _ := r.getVar(name); v != "" {
			return v == "C" || v == "POSIX"
		}
	}
	return true
}

// reportsFlagErrors is the `X` flag: a failure the `Q`, `e` and `#` flags
// would pass over in silence is reported, and the expansion fails. Measured
// 2026-10-02 on zsh 5.9.2 (`-f`, `LC_ALL=C`), each ending the line where the
// unflagged spelling goes on:
//
//	foo='unmatched "';  ${(QX)foo}   unmatched "      ${(Q)foo} is the value
//	foo="a'b";          ${(QX)foo}   unmatched '
//	foo='$(x';          ${(QX)foo}   parse error in parameter value
//	foo='${x';          ${(QX)foo}   closing brace expected
//	foo='`x';           ${(QX)foo}   unmatched `
//	foo=1+;             ${(X#)foo}   bad math expression: operand expected at
//	                                 end of string       ${(#)foo} is empty
//	foo='$(';           ${(Xe)foo}   parse error
//	foo='a\';           ${(QX)foo}   a                 a trailing backslash
//	                                                     is no failure
//
// A pattern operator's bad pattern is reported with or without it, in that
// shell and here (#5151).
func reportsFlagErrors(e *syntax.ParamExpr) bool {
	return strings.ContainsRune(e.Flags, 'X')
}

func (r *Runner) characterForCode(n int) string {
	if n < 0 || !r.countsTheLocalesCharacters() {
		return string([]byte{byte(n)})
	}
	u := uint32(n)
	if u < utf8.RuneSelf {
		return string([]byte{byte(u)})
	}
	// The original UTF-8 scheme: a lead byte saying how many continuation
	// bytes follow, each carrying six bits. Past 31 bits the lead is the
	// one that says five and the two bits left over go into it — measured,
	// 2147483648 is fe 80 80 80 80 80 and 3221225472 is ff 80 80 80 80 80.
	var lead byte
	var conts int
	switch {
	case u < 0x800:
		lead, conts = 0xc0, 1
	case u < 0x10000:
		lead, conts = 0xe0, 2
	case u < 0x200000:
		lead, conts = 0xf0, 3
	case u < 0x4000000:
		lead, conts = 0xf8, 4
	case u < 0x80000000:
		lead, conts = 0xfc, 5
	default:
		lead, conts = 0xfe, 5
	}
	out := make([]byte, conts+1)
	for i := conts; i > 0; i-- {
		out[i] = 0x80 | byte(u&0x3f)
		u >>= 6
	}
	out[0] = lead | byte(u)
	return string(out)
}

// capitalizedRuns is the `(C)` flag over one word: every run of letters and
// digits begins with its first character in upper case and goes on in lower
// case, and everything else separates the runs and is left alone. The runs
// are not the words splitting makes. Measured 2026-10-02 on zsh 5.9.2 under
// `LC_ALL=en_US.UTF-8`:
//
//	hELLO wORLD foo_bar 3abc a1b2 x-y    Hello World Foo_Bar 3abc A1b2 X-Y
//	l'état c'est moi                     L'État C'Est Moi
//	ǆemal ßtraße                         Ǆemal ßtraße — the upper case, not the
//	                                     title case, and ß has none
//
// and under `LC_ALL=C` `éCOLE` is `éCole`: a character past ASCII is not a
// letter there, so it separates and is not changed — the case maps' own
// locale rule, caseFoldReachesBeyondASCII. A byte the locale cannot decode
// separates too and is kept as it was.
func (r *Runner) capitalizedRuns(v string) string {
	wide := r.caseFoldReachesBeyondASCII(v)
	var b strings.Builder
	b.Grow(len(v))
	inRun := false
	for i := 0; i < len(v); {
		c, size := utf8.DecodeRuneInString(v[i:])
		if (c == utf8.RuneError && size <= 1) || (!wide && c >= utf8.RuneSelf) {
			b.WriteString(v[i : i+size])
			inRun = false
			i += size
			continue
		}
		switch {
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			inRun = false
			b.WriteRune(c)
		case inRun:
			b.WriteRune(unicode.ToLower(c))
		default:
			inRun = true
			b.WriteRune(unicode.ToUpper(c))
		}
		i += size
	}
	return b.String()
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
	expand := func(st PromptStyle, text string, f PromptResolver, q PromptQuantityResolver) (string, string, bool) {
		// Not the exported ExpandPromptStyle: the visual state a rendering
		// leaves behind is the shell's, and a flag that started from an
		// empty one would lose a color the rendering before it set. See
		// promptVisualState in interp/prompt.go.
		return expandPromptStyle(st, text, f, q, &r.promptVisual)
	}
	if subst {
		expand = func(st PromptStyle, text string, f PromptResolver, q PromptQuantityResolver) (string, string, bool) {
			// What the substitution pass says is the shell's, not the
			// builtin's that asked for it: measured 2026-10-04 on zsh
			// 5.9.2, `setopt promptsubst; print -P '${x?boom}'` is
			// `zsh:1: x: boom`, and a bad substitution, a math failure and
			// a command not found inside it name no builtin either.
			outer := r.inBuiltin
			r.inBuiltin = ""
			defer func() { r.inBuiltin = outer }()
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
	// Text an `eval` is running is a unit of its own and is named for
	// itself, ahead of the frame it was called from — the same rule, read
	// through the same pair of fields, that a diagnostic's location already
	// keeps. See Runner.locationNameAndLine, where the eval question is
	// asked first for the same reason.
	//
	// Measured 2026-09-28 on zsh 5.9.2, and the second row is the control
	// that says the rule is about the *text* rather than about depth:
	//
	//	eval 'print -P %N'                     (eval)
	//	f() { print -P %N }; eval 'f'          f       — entering a function
	//	                                               leaves the eval behind
	//	f() { eval 'print -P %N' }; f          (eval)  — and an eval inside
	//	                                               one wins over it
	//
	// It reaches the **trace prefix** as well as a written prompt, because
	// this dialect's default PS4 is `+%N:%i>` and is drawn through this
	// table: `setopt xtrace; eval true` is `+(eval):1> true` there and was
	// `+zsh:1> true` here (#5017).
	if d := r.diag(); d.LocationNamesTheEvalText && d.EvalSourceName != "" &&
		r.locationIsInsideEvalText() {
		return d.EvalSourceName
	}
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
//
// doubled is the writer's half of the option that reads a doubled quote
// inside a single-quoted run as one literal quote. It reaches the two styles
// that *choose single quotes* and no others — measured, `${(q)}`, `${(qqq)}`,
// `${(qqqq)}` and `${(q-)}` are byte-identical in both states of it, the
// first and last writing an embedded quote with a backslash outside any
// quoting and the middle two not being single-quoted spellings at all.
func quoteFlagged(v string, count int, mod byte, nothing, doubled bool, hist historyQuoting) string {
	switch mod {
	case '-':
		return quoteMinimal(v, hist.plain)
	case '+':
		return quoteExtended(v, doubled, hist.plain)
	}
	switch count {
	case 1:
		return quoteWithBackslashes(v, nothing, hist.plain)
	case 2:
		if doubled {
			return singleQuoted(v, "''", false)
		}
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	case 3:
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(v); i++ {
			if strings.IndexByte("\\`\"$", v[i]) >= 0 || hist.plain != 0 && v[i] == hist.plain {
				b.WriteByte('\\')
			}
			b.WriteByte(v[i])
		}
		b.WriteByte('"')
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("$'")
		eachQuotableByte(v, func(i int, c byte) {
			switch {
			case c == '\'':
				b.WriteString(`\'`)
			case c == '\\':
				b.WriteString(`\\`)
			case hist.dollar != 0 && c == hist.dollar:
				b.WriteByte('\\')
				b.WriteByte(c)
			case c < 0x20 || c >= 0x7f:
				b.WriteString(controlEscapeBefore(c, v[i+1:]))
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
func quoteWithBackslashes(v string, nothing bool, hist byte) string {
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
			b.WriteString("$'" + controlEscapeBefore(c, v[i+1:]) + "'")
		case quotableByteWith(i, c, hist):
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

// controlEscapeBefore is controlEscape for a byte with rest behind it, which
// spells a NUL as `\0` wherever the octal digit that would lengthen it does
// not follow. Measured 2026-10-02 on zsh 5.9.2: `${(q)x}` over a NUL then `b`
// is `$'\0'b`, over a NUL then `1` is `$'\000'1`, over a NUL then `8` is
// `$'\0'8`, and `$'\1'` stays `$'\001'` — so it is the NUL alone, and the
// digit counts even outside the quotes the escape stands in (#5336).
func controlEscapeBefore(c byte, rest string) string {
	if c == 0 && (rest == "" || rest[0] < '0' || rest[0] > '7') {
		return `\0`
	}
	return controlEscape(c)
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

// modifiedElements is a `${a:…}` range read as a modifier list over a list of
// values, and false where the range is not one.
//
// The three shapes are substringRange's three, which is why this reads them
// rather than restating them: a list of modifiers, an offset and then a list,
// and an offset, a length and then a list. What differs is only what the list
// is applied *to* — every element, one at a time, rather than one value.
//
// Measured, and it is the elements and not the joined string: `${b:1:t}` on
// `(/a/x.c /b/y.c 'p q')` is `y.c` and `p q`, so the offset slices the list
// and the tail is taken of each of what is left. Inside double quotes the
// list has already become one word by the time anything asks, which is why
// `"${b:t}"` is the tail of the whole joined string and goes the other way.
func (r *Runner) modifiedElements(words []string, e *syntax.ParamExpr) ([]string, bool) {
	segs, sliced, ok := r.rangeModifiers(words, e)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(sliced))
	savedHead := r.liveMarksAtHead
	defer func() { r.liveMarksAtHead = savedHead }()
	for i, w := range sliced {
		if i > 0 {
			// Every element after the first is a field of its own, and so
			// at a head, whatever stood in front of the expansion. See
			// headTokenMark: x${a:s/A/~/} over (A A) is x~ and then $HOME.
			r.liveMarksAtHead = true
		}
		got, applied := r.applyModifiers(w, segs, e)
		if !applied {
			// The list is refused as a whole, the way it is for a scalar:
			// a modifier that names nothing is the same complaint whichever
			// element reached it first.
			return nil, true
		}
		out = append(out, got)
	}
	return out, true
}

// rangeModifiers reads a range as a modifier list: the segments, and the
// elements they apply to once any offset and length have taken their part.
func (r *Runner) rangeModifiers(
	words []string, e *syntax.ParamExpr,
) (segs, sliced []string, ok bool) {
	reads := func() bool {
		return r.ask(r.sem().SubstringRangeReadsModifiers,
			"a substring range beginning with a letter being a modifier list")
	}
	switch {
	case rangeReadsAsAModifier(e.Arg, e.ArgText):
		if !reads() {
			return nil, nil, false
		}
		return r.modifierSegments(
			modifierSource(e.ArgText, e.Arg),
			modifierSource(e.Arg2Text, e.Arg2), e.Arg2 != nil,
		), words, true
	case e.Arg2 != nil && rangeReadsAsAModifier(e.Arg2, e.Arg2Text):
		if !reads() {
			return nil, nil, false
		}
		from := &syntax.ParamExpr{Name: e.Name, Op: e.Op, Arg: e.Arg}
		return r.modifierSegments(modifierSource(e.Arg2Text, e.Arg2), "", false),
			sliceElems(words, r.numOf(e.Arg, e, nil), from, r), true
	}
	lenWord, mods, split := splitLengthFromModifiers(e.Arg2)
	if !split || !reads() {
		return nil, nil, false
	}
	from := &syntax.ParamExpr{Name: e.Name, Op: e.Op, Arg: e.Arg, Arg2: lenWord}
	return mods, sliceElems(words, r.numOf(e.Arg, e, lenWord), from, r), true
}

// bareEmptyMarks appends to marks one entry per word, true where the word is
// empty, and answers the result.
//
// It is how flaggedWords keeps the **bare** empty fields a split made apart
// from empty values, which nothing after the split can tell apart because both
// are the empty string. The single-`q` style writes an empty value as a pair
// of single quotes and a bare field as nothing, the same distinction
// substitutedNothing draws for an operator that substituted nothing. Measured
// on zsh 5.9.2 with IFS=:, `u=a::b:`, `d=:` and `e=` (#5281):
//
//	${(@q)=u}               a, nothing, b, nothing
//	${(@q)=d}               nothing, nothing
//	${(@q)=e}               a quoted empty: a word that was already empty
//	${(@qq)=u}, ${(@q-)=u}  each empty quoted: only the one style
//	${(@q%)=u}              each empty quoted: prompt escapes make values
//
// Unquoted, a bare field is an empty word, which the command line keeps only
// where the split keeps its empty fields.
//
// The same holds for the elements of a *nested* list — `"${(@q)${b[@]}}"`
// leaves an empty element of `b` bare — and not for a nested scalar under
// `(@)`, which the nesting cannot tell apart from a list by its strings alone;
// see nestedElementsAreBare, which asks the inner as written (#5299).
func bareEmptyMarks(words []string, marks []bool) []bool {
	for _, w := range words {
		marks = append(marks, w == "")
	}
	return marks
}

// emptiesAllBare reports whether every empty word is marked bare and there is
// at least one, which is when the marks can be made again from the words.
func emptiesAllBare(words []string, bare []bool) bool {
	any := false
	for i, w := range words {
		if w == "" {
			if !bare[i] {
				return false
			}
			any = true
		}
	}
	return any
}

// nestedElementsAreBare reports whether the inner of a nested expansion hands
// back the elements of a list — an array's, or what a split made — rather than
// a value: the words whose empty members are bare. See flaggedWords, where the
// measurement is.
//
// Asked of the inner as written, because the fields alone cannot say it: a
// scalar under `(@)` comes back looking exactly like a list of one, and its
// empty value is a value.
func (r *Runner) nestedElementsAreBare(e *syntax.ParamExpr) nestedBareKind {
	span, _ := r.nestedInnerSpan(e)
	p := span.Param
	if span.Kind != syntax.ParamExp || p == nil || p.Inner != nil || p.Length || p.Indirect {
		return nestedBareNone
	}
	if strings.ContainsAny(p.Flags, splitFlagLetters) {
		return nestedBareFromAList
	}
	if p.SplitFlags%2 == 1 {
		return nestedBareFromAnIFSSplit
	}
	if !r.paramIsAList(p) {
		return nestedBareNone
	}
	if p.Name == "@" || p.Name == "*" {
		return nestedBareFromAList
	}
	if _, ok := r.arrayElems(p.Name); ok {
		return nestedBareFromAList
	}
	if isAssoc := r.assocDeclared(p.Name); isAssoc {
		return nestedBareFromAList
	}
	return nestedBareNone
}

// nestedBareKind is what nestedElementsAreBare finds a level down.
type nestedBareKind int

const (
	// nestedBareNone is a value: nothing in it is bare.
	nestedBareNone nestedBareKind = iota
	// nestedBareFromAList is an array's elements, or a letter split's
	// fields, whose empty members are gone unquoted and bare in quotes.
	nestedBareFromAList
	// nestedBareFromAnIFSSplit is an `=` split's fields, whose empty members
	// are bare in either context and kept unquoted.
	nestedBareFromAnIFSSplit
)

// operandTokenWords is the word a `-` or `+` substituted under a flag group,
// as the words it comes to with its own pattern characters still live, where
// the group runs on each word and the matching waits for the end. Measured
// 2026-10-03 on zsh 5.9.2 under `-f`, in a directory holding `xay` and `xby`:
//
//	${(U):-{b,c}}   B C, two words     ${(j:-:):-{b,c}}   b-c
//	${(qq):-{b,c}}  'b' 'c'            the braces make two words first
//	${(U):-x*}      no matches found: X*    matched after the case flag
//	${(qq):-x*}     'x*'               quoted before any matching, and so
//	${(q+):-x?y}    'x?y'              every quoting style but one
//	${(q):-xa*}     xay                the single `q` leaves `*` live
//	${(q):-=}  ${(q):-#a}  ${(q):-a^b}   =  #a  a^b, and `=`, `#`, `^` too
//	${(q):-'xa*'}   xa*                a quoted one is text
//
// Only for the expansion the word loop is expanding, unquoted, and under the
// letters that carry the marks through; the rest join the word as before.
func (r *Runner) operandTokenWords(e *syntax.ParamExpr, quoted bool) ([]string, bool) {
	if quoted || e.Arg == nil || r.liveMarksFor != e || strings.Trim(e.Flags, "ULCoOqj@") != "" {
		return nil, false
	}
	fields := r.expandWordEscaped(e.Arg)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, markOperandTokens(f))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out, true
}

// operandTokenBytes are the characters an operand writes unquoted that stay
// pattern syntax through a flag group.
const operandTokenBytes = "*?[]()|#^=" //nolint:gosec // G101: pattern bytes, not a token

// markOperandTokens turns one escaped field into its text, with liveMark in
// front of each pattern character no backslash protected.
func markOperandTokens(esc string) string {
	var b strings.Builder
	for i := 0; i < len(esc); i++ {
		c := esc[i]
		switch {
		case c == '\\' && i+1 < len(esc):
			i++
			b.WriteByte(esc[i])
		case strings.IndexByte(operandTokenBytes, c) >= 0:
			b.WriteString(liveMark)
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// quoteAroundLiveMarks is the single `q` over a word holding live marks: the
// text between them is quoted and each marked character is left as it is.
func quoteAroundLiveMarks(w string, empty bool, hist byte) string {
	var b strings.Builder
	for {
		i := strings.Index(w, liveMark)
		if i < 0 || i+len(liveMark) >= len(w) {
			if rest := stripLiveMarks(w); rest != "" || b.Len() == 0 {
				b.WriteString(quoteWithBackslashes(rest, empty, hist))
			}
			return b.String()
		}
		if i > 0 {
			b.WriteString(quoteWithBackslashes(w[:i], false, hist))
		}
		b.WriteString(w[i : i+len(liveMark)+1])
		w = w[i+len(liveMark)+1:]
	}
}
