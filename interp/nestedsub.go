// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A subscript applied to a nested expansion — `${${(P)h}[2]}`.
//
// One grammar in the panel has the construct, so what it means here is that
// shell's answer, measured with the binary and written down in
// docs/spec/grammar/parameter-expansion.md. Neither half is new on its own:
// the subscript machinery and the nesting were both built, and what was
// missing was applying the one to the result of the other.
//
// It is not a corner. `if (( ${${(P)hook}[(I)$fn]} == 0 ))` is how
// `add-zsh-hook` asks whether a function is already on a hook, and
// `add-zsh-hook` is how essentially every plugin installs one.
//
// There are two readings, and the split is measured rather than tidy:
//
//	`${(P)h}` is a *name*, so the subscript reads the parameter it names.
//	Every other inner expansion is a *value*, so the subscript reads what
//	that value came to.
//
// The first is nestedParamReference. It matters because a parameter is more
// than its fields: measured on zsh 5.9.2, `h=m` naming an association makes
// `${${(P)h}[k]}` the value under `k`, `arr=('' b c)` makes
// `${${(P)h}[1]}` the empty first element where the same array's *fields*
// have dropped it, and `arr=(hello)` makes `${${(P)h}[2]}` nothing at all
// where a one-character-per-index reading would answer `e`. All three follow
// from the subscript reaching the parameter, and none of them follows from
// the fields.

// nestedSubscript answers `${${…}[sub]}`.
//
// The value and the set-ness the caller wants are the same two the rest of
// nestedWords reports, so a missing element is *unset* — measured,
// `a=(x y); ${${a}[9]-none}` is `none` rather than empty — and an element
// holding the empty string is set.
func (r *Runner) nestedSubscript(e *syntax.ParamExpr) (words []string, set bool) {
	span := e.Inner.Spans[0]
	if name, isRef := r.nestedParamReference(span); isRef {
		// The subscript, the flag group and the written text, on the name
		// the inner resolved to: `${${(P)h}[(I)x]}` with `h=arr` is
		// `${arr[(I)x]}`, and Src is carried so a refusal from inside names
		// what the script wrote rather than what it resolved to.
		ref := &syntax.ParamExpr{
			Name: name, Index: e.Index, IndexFlags: e.IndexFlags, Src: e.Src,
		}
		elems, _ := r.arraySubscript(ref)
		return r.nestedSubscriptResult(e, elems)
	}
	src, ok := r.nestedSubscriptSource(e, span)
	if !ok {
		return []string{""}, false
	}
	if e.IndexFlags != nil {
		search, sok := r.subscriptSearch(e)
		if !sok {
			// A letter the group does not carry, refused by name inside.
			return []string{""}, false
		}
		if search != 0 {
			elems, _ := r.searchSubscript(e, search, src)
			return r.nestedSubscriptResult(e, elems)
		}
	}
	elems, _ := r.subscriptOver(e, src)
	return r.nestedSubscriptResult(e, elems)
}

// nestedSubscriptResult is what the subscript came to, as the value and the
// set-ness of the expansion around it.
func (r *Runner) nestedSubscriptResult(e *syntax.ParamExpr, elems []string) ([]string, bool) {
	if len(elems) == 0 {
		// The subscript named nothing. One empty field and *unset*, which is
		// what lets the outer `-` and `:-` substitute their word.
		return []string{""}, false
	}
	if len(elems) > 1 {
		// `${${a}[@]}` and `${${a}[1,2]}` are a list in the shell with the
		// grammar, and a list standing where a nested expansion's value goes
		// is the other shape this surface has not built — refused by its own
		// name, next door, and refused here in the same words rather than
		// joined into one plausible field.
		r.diagf("${%s}: a nested expansion of a list is not implemented\n", e.Src)
		r.expandErr = true
		return []string{""}, false
	}
	return elems, true
}

// nestedParamReference reports the name a `${(P)…}` inner resolves to.
//
// `(P)` reads the value it is given as a further *name*, so an inner carrying
// one is a parameter reference and the subscript belongs to that parameter.
//
// **The name is the inner expansion with `P` struck out of its group**, which
// is measured rather than assumed and is the whole content of this function.
// The other letters transform the *name* and not what the name holds, and
// with `ARR=(x y)`, `arr=(hello)` and `h=arr`:
//
//	${(UP)h}          HELLO   the value, uppercased, with no subscript
//	${${(UP)h}[1]}    x       but subscripted it is `${ARR[1]}`
//	g=h; ${${(UP)${g}}[2]}    `${H[2]}`, through the group's own base
//	${${(P)h:-d}[2]}          `${arr[2]}`, the operator running on the name
//
// So the resolution is the whole flag pipeline over the base, minus the one
// letter the subscript takes over — which is also why the base is expanded
// here and nowhere else: `${(P)$(cmd)}` runs its command once.
//
// A count and a set test are names too, which is the sharp end of "the whole
// pipeline": measured with `set -- abc def` and `hh=zz`, `${${(P)#hh}[1]}` is
// `d` — `${#hh}` is 2, and `${2[1]}` is the first character of the second
// parameter. A guard excluding them looked obviously right and was wrong.
//
// Quoted as well as unquoted, measured: `${"${(P)h}"[2]}` on `arr=(a b c)` is
// `b`, the same element the bare spelling answers with, where quoting an
// inner that is a *value* joins its fields first.
func (r *Runner) nestedParamReference(span syntax.Span) (string, bool) {
	if span.Kind != syntax.ParamExp || span.Param == nil {
		return "", false
	}
	e := span.Param
	if e.Bad || !e.HasFlags || !strings.ContainsRune(e.Flags, 'P') {
		return "", false
	}
	ref := *e
	ref.Flags = strings.ReplaceAll(e.Flags, "P", "")
	words, _, ok := r.flaggedWords(&ref, splitNever, false)
	if !ok {
		// The group failed, and it has said so.
		return "", false
	}
	return strings.Join(words, " "), true
}

// nestedSubscriptSource is what the subscript reads when the inner is a value
// rather than a name: the fields the inner came to, and whether they are one
// string.
//
// ok is false for the shapes this does not carry, each refused by name.
func (r *Runner) nestedSubscriptSource(e *syntax.ParamExpr, span syntax.Span) (subscriptSource, bool) {
	if span.Kind != syntax.ParamExp || span.Param == nil {
		// A command substitution and an arithmetic one stand in this
		// position, and both are a *list* there even when they come to one
		// word: measured, `${$(echo abc)[2]}` and `${$((6*7))[2]}` are both
		// empty where a string would have answered `b` and `2`. This tree
		// does not field-split an unquoted substitution in the name position
		// yet (#976), so the fields it would count are not the shell's, and
		// counting characters instead would answer a plausible one.
		r.diagf("${%s}: a subscript on a nested %s is not implemented\n", e.Src, span.Kind)
		r.expandErr = true
		return subscriptSource{}, false
	}
	inner, _ := r.nestedInnerSpan(e)
	words := r.nestedInnerFields(e)
	if r.nestedResultIsAList(inner.Param, words, inner.Quoting != syntax.Unquoted) {
		return subscriptSource{elems: words}, true
	}
	return subscriptSource{elems: []string{strings.Join(words, "")}, scalar: true}, true
}

// nestedResultIsAList reports whether the fields an inner expansion came to
// are a *list*, where a subscript counts elements, rather than one string,
// where it counts characters.
//
// This is the one question the construct adds, and it is not answered by the
// field count. Measured on zsh 5.9.2, with `[2]` on a result of one field:
//
//	arr=(hello); ${${arr}[2]}      empty  — a one-element list
//	x=abc;       ${${(f)x}[2]}     b      — a split that found nothing to
//	                                        split leaves a string
//	arr=(aa bb); ${${(j.,.)arr}[2]} a     — a join leaves one, too
//	arr=(hello); ${${(U)arr}[2]}   empty  — a flag that does neither keeps
//	                                        the list it was given
//	arr=(a b c); ${${arr:+def}[2]} e      — a substituted word is its own
//	                                        value, and this one is a string
//	arr=(hello); ${${x:+$arr}[2]}  empty  — while this one is a list
//	arr=(a b c); ${${#arr}[1]}     3      — a count is a string
//	s=hello;     ${${(A)s}[2]}     empty  — and `(A)` says outright that
//	                                        what it made is an array
//
// So the shape is the inner expansion's, asked of the node rather than of
// what it produced, and a split or a join is the one thing that overrides it.
func (r *Runner) nestedResultIsAList(e *syntax.ParamExpr, words []string, quoted bool) bool {
	if len(words) > 1 {
		return true
	}
	if e.Length {
		return false
	}
	if e.HasFlags && strings.ContainsRune(e.Flags, 'A') {
		return true
	}
	if strings.ContainsAny(e.Flags, "fsj") || e.SplitFlags != 0 {
		return false
	}
	if quoted {
		// Quoting joins the fields of everything but the spellings that keep
		// them — `@` itself, an `[@]` subscript and the `(@)` flag — so with
		// quotes around it the shape is that one predicate and nothing else.
		// Measured with `a=(p q r)`: `"${${a}[2]}"` is the *space* in
		// `p q r` and `"${${a[@]}[2]}"` is `q`, and `"${${*}[2]}"` is a
		// space where `"${${@}[2]}"` is `q`.
		return r.flagKeepsFields(e)
	}
	return r.paramIsAList(e)
}

// paramIsAList reports whether an expansion stands for a list of values
// rather than for one.
//
// A question about the *node*, answered without expanding it: what it names,
// what its subscript selects, and — for the four conditionals — whether the
// word or the parameter is what it came to. That last one is read from
// yieldsTheArray, which is the same source expandAtList already uses for it,
// so the two cannot come to different answers.
func (r *Runner) paramIsAList(e *syntax.ParamExpr) bool {
	if e.Inner != nil || e.Bad || e.Length || e.Indirect {
		// A nested inner is a value: measured, `${${${s}}[2]}` on `hello` is
		// `e`, so the depth does not make a list of a string. An inner that
		// came to a list is refused a level down, by name.
		//
		// The nesting clause is also what keeps this from *asking*: the four
		// conditionals below read yieldsTheArray, which expands a nested
		// inner again to find out whether the test fired. No answer changes
		// — a nested inner is not a list either way — so a mutation that
		// drops it shows nothing but one more run of the inner's command
		// substitution, which is #1404's subject rather than this one's.
		return false
	}
	if e.Prefix != 0 {
		// `${!pre@}` is the names, one field each, where `${!pre*}` is one
		// joined field — the same difference `$@` has from `$*`.
		//
		// No grammar reaches this today: the one dialect with an expansion
		// in the name position has no `${!…}` at all, so nothing can write
		// the pairing and no test can tell this line from its absence. It
		// stays for the reason bareArrayAsList keeps its own unreachable
		// guard — what it prevents is a different construct's answer rather
		// than a different field count.
		return e.Prefix == '@'
	}
	if e.Index != nil {
		// `${a[@]}` and a range are a list; a subscript naming one element
		// is that element, and measured `${${a[1]}[2]}` on `(hello)` is `e`
		// — the element read as a string.
		return r.subscriptYieldsAList(e)
	}
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamError, syntax.ParamAlternate:
		if !r.yieldsTheArray(e) {
			// The word is what it came to, so the word's own fields decide:
			// `${${x:+$arr}[2]}` is a list and `${${x:+"$arr"}[2]}` is the
			// joined string, measured.
			return r.wordIsAList(e.Arg)
		}
	}
	return r.nameIsAList(e.Name)
}

// nameIsAList reports whether a name holds several values rather than one.
//
// Asked of subscriptTarget, which is the function a subscript on that name
// would have reached: its scalar flag is exactly this question — false for
// the positional parameters, a stored array and a produced one, true for a
// value read as an array of one — so the two cannot come to different
// answers about the same name.
//
// The association is the one source that function does not answer for, since
// arraySubscript takes an association down its own path before reaching it.
func (r *Runner) nameIsAList(name string) bool {
	if _, isAssoc := r.assocFor(name); isAssoc {
		return true
	}
	_, scalar, held := r.subscriptTarget(&syntax.ParamExpr{Name: name})
	return held && !scalar
}

// wordIsAList reports whether a word contributes several fields, which is
// what an unquoted list-shaped expansion inside it does and what quoting it
// stops.
func (r *Runner) wordIsAList(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, s := range w.Spans {
		if s.Quoting != syntax.Unquoted {
			continue
		}
		if s.Kind == syntax.ParamExp && s.Param != nil && r.paramIsAList(s.Param) {
			return true
		}
	}
	return false
}
