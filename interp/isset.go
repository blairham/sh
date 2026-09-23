// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// parameterIsSet answers `[[ -v name ]]` and `test -v name`: whether the
// parameter that name names is set, which is a question about the parameter
// and never about its value. A name holding the empty string is set.
//
// The two constructs are one function because the three shells that have the
// operator give them the same answer — `x=1; [ -v x ]` and `x=1; [[ -v x ]]`
// agree in bash 5.3, ksh93u+ and zsh 5.9.2, and so does every row below.
// They differ only in how the operand is obtained, which is their callers'
// business.
//
// **The set-ness itself is paramSource's**, the same call `${name+word}`
// makes. That is deliberate rather than convenient: an array element, an
// associative key, and the base an index counts from are all decided there
// already, so `[[ -v 'a[2]' ]]` cannot come to a different answer from
// `${a[2]+s}` — and it is why zsh's `a[0]` is unset here while bash's is set,
// with no knowledge of array bases in this file.
//
// Measured 2026-09-07 across bash 5.3.15, ksh93u+ and zsh 5.9.2, each probe
// under `-c`. Unanimous, and therefore not modeled here at all:
//
//	x=1                 set        x=                  set
//	unset x             unset      never assigned      unset
//	a=(1 2 3); a[1]     set        a[9]                unset
//	h[k] present        set        h[k] absent         unset
//	whole array `a`     set        PATH                set
//	`` (empty name)     unset      `a b`, `x `         unset
//	`@`                 unset      3 past `$#`         unset
//
// One row looked like a fourth disagreement and is not, which is the reason
// it is written down: `typeset x` with no value answers *set* in zsh and
// unset in bash and ksh93. That is not this operator — zsh's `typeset x`
// really assigns the empty string, and `${x+s}` says so in the same three
// columns. Reading it as an axis here would have modeled a declaration rule
// as a set-ness rule, which is the shape of #989.
func (r *Runner) parameterIsSet(name string) (bool, error) {
	base, sub, subscripted := r.subscriptOperand(name)
	if !subscripted {
		base = name
	}
	return r.elementIsSet(base, sub, subscripted), nil
}

// testParameterIsSet answers `test -v name` and `[ -v name ]`, which is
// parameterIsSet with one round in front of it.
//
// A function of its own rather than a flag on parameterIsSet, because the two
// operators genuinely part here and only here: bash's `[[ -v 'g[$k]' ]]`
// finds the key `x y` whatever `shopt -s assoc_expand_once` says and its
// `test -v 'g[$k]'` stops finding it, so a shared route would have to pin one
// of the two answers. Everything after the round is the same lookup, which is
// the half the two operators do share — see parameterIsSet.
//
// Semantics.TestIsSetExpandsAFlatSubscript is whether this shell rounds at
// all and Runner.ExpandsAnOperandsSubscriptAgain whether the session still
// permits it; operandSubscriptText reads both. Where nothing rounds, this is
// parameterIsSet exactly.
func (r *Runner) testParameterIsSet(operand string) (bool, error) {
	base, raw, sub, subscripted := r.subscriptOperandParts(operand, false)
	if !subscripted {
		return r.parameterIsSet(operand)
	}
	sub = r.operandSubscriptText(base, raw, sub, r.sem().TestIsSetExpandsAFlatSubscript,
		"`test -v` expanding a subscript that reached it as text")
	if r.unspecified {
		return false, nil
	}
	return r.elementIsSet(base, sub, true), nil
}

// elementIsSet is the lookup both routes end at, once the operand has been
// split into a base and a subscript by whichever of the two readings applies.
func (r *Runner) elementIsSet(base, sub string, subscripted bool) bool {
	if !r.isSetNameKind(base) {
		return false
	}
	e := &syntax.ParamExpr{Name: base}
	if subscripted {
		e.Index = literalWord(sub)
	}
	// A set-ness test and nothing else, so a value's producer is not run for
	// it — measured, `[[ -v x ]]` fires no `.get` where `${x}` fires one.
	// See Runner.askTheStoreOnly (#3121).
	defer r.askTheStoreOnly()()
	_, set, _ := r.paramSource(e)
	if subscripted && wholeArraySubscript(sub) {
		// `a[@]` and `a[*]`, which is the one shape where this operator does
		// **not** answer what `${a[@]+s}` answers — see
		// wholeArraySubscriptIsSet.
		return r.wholeArraySubscriptIsSet(base, sub, e, set)
	}
	return set
}

// wholeArraySubscriptIsSet answers `[[ -v a[@] ]]` and `[[ -v a[*] ]]`, which
// is the one operand shape where the operator parts company with the
// conditional expansion that otherwise decides it.
//
// Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, over a script file (#3436):
//
//	                      bash 5.3.20  zsh 5.9.2   ksh93u+
//	e=(); -v e[@]         no           no          `@: arithmetic syntax error`
//	f=(x); -v f[@]        yes          no          the same
//	typeset -A n; n[k]=v
//	  -v n[@]             no           no          no
//	  -v n[k]             yes          yes         yes
//	s=plain; -v s[@]      yes          yes         —
//	${e[@]+S}             unset        —           —
//	${n[@]+S}             **set**      —           —
//
// The last two rows are what say this cannot be `${a[@]+s}` read through
// another operator: in bash the expansion calls a one-key table set and the
// operator calls it unset, measured on the same line of the same script.
//
// So the two readings are about what the subscript **names**. One reads it as
// the array itself, and the answer is then the parameter's own set-ness —
// which for an array with no elements is Semantics.EmptyArrayIsSet's, which
// is why `e` and `f` part there and not here. The other reads it as an
// *element*, and no array has one called `@` or `*`, so a name holding an
// array is never set through it while a scalar — which has no elements to
// name — answers for itself.
//
// **A table is the exception under the first reading**, and it is measured
// rather than carved out: a table's subscript is a string, so `@` is the key
// `@`. `typeset -A n; n[@]=x; [[ -v n[@] ]]` is *yes* in bash, which is the
// row that says the operand is being looked up and not refused.
//
// ksh93's indexed rows are not modeled: that shell evaluates the subscript
// arithmetically and `@` is a syntax error there, which ends the script. Its
// table row is the one this reading is set from.
func (r *Runner) wholeArraySubscriptIsSet(base, sub string, e *syntax.ParamExpr, set bool) bool {
	table, isTable := r.assocFor(base)
	switch r.sem().ConditionWholeArraySubscript {
	case ConditionWholeArraySubscriptNamesTheParameter:
		if !isTable {
			return r.emptyWholeArrayIsSet(e, set)
		}
	case ConditionWholeArraySubscriptNamesAnElement:
		if _, isArray := r.Arrays[base]; !isArray && !isTable {
			// No elements to name, so the subscript names the parameter by
			// default and its own set-ness stands.
			return set
		}
	default:
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a whole-array subscript in `[[ -v … ]]`")))
		r.status, r.unspecified = 2, true
		return false
	}
	// The key lookup both readings end at: a table holding the literal `@`
	// or `*` answers yes, and an array has no such element.
	_, held := table[sub]
	return held
}

// condParameterIsSet answers `[[ -v w ]]`, where the operand is still a word
// and not yet a string.
//
// That is the whole difference from `test -v`, and it is a real one: the word
// still carries which of its brackets were *written* unquoted and which
// arrived inside quotes or out of an expansion, and one shell in the panel
// reads the subscript by the first of those rather than the second. See
// Semantics.ConditionIsSetReadsTheWrittenSubscript for the row, including the
// row on which bash's own two spellings of this operator disagree.
//
// s is the operand already expanded, because the caller needed it anyway for
// the trace and because it is what the other reading works from.
func (r *Runner) condParameterIsSet(w *syntax.Word, s string) (bool, error) {
	base, sub, ok := writtenSubscript(w)
	if !ok {
		return r.flatConditionIsSet(w, s)
	}
	name := strings.Join(r.expandWordNoSplit(base), "")
	key := strings.Join(r.expandWordNoSplit(sub), "")
	// Asked at the disagreement and nowhere else. A subscript whose brackets
	// survive expansion unchanged reaches the same element under either
	// reading, and the overwhelming majority of them do — so the axis is not
	// consulted for `[[ -v a[1] ]]`, and a dialect that has never been
	// measured on a bracketed key is not recorded as having an answer.
	//
	// The second half of the guard is the *flat* reading's own round, which
	// is what keeps "the two readings agree" true: with a key that another
	// expansion would move, they no longer reach the same element even when
	// the two spellings of it match here. Read off the characters rather
	// than performed, so deciding which reading applies runs no substitution
	// — see subscriptTextCouldExpand.
	if flat, fkey, fok := r.subscriptOperand(s); fok && flat == name && fkey == key &&
		!subscriptTextCouldExpand(fkey) {
		return r.elementIsSet(name, key, true), nil
	}
	if !r.ask(r.sem().ConditionIsSetReadsTheWrittenSubscript,
		"`[[ -v a[k] ]]` reading the subscript the script wrote rather than the one left after expansion") {
		return r.flatConditionIsSet(w, s)
	}
	return r.elementIsSet(name, key, true), nil
}

// flatConditionIsSet is `[[ -v ]]` over an operand read as text: the brackets
// were quoted, or they came out of a value, so what stands between them has
// not been expanded by anything yet.
//
// Semantics.ConditionIsSetExpandsAFlatSubscript is whether this shell expands
// it now. Where it does not — and where there is nothing in the text for a
// round to do, which is nearly every operand — the answer is the plain
// lookup the builtin's route makes, under the key as it stands.
//
// Not shared with `test -v`, which takes the same text and reaches
// parameterIsSet directly. bash expands there too by default, but that
// surface moves with `shopt -s assoc_expand_once` and this one does not, so
// routing the builtin through here would pin an option's state as a
// dialect's answer. See #3298.
func (r *Runner) flatConditionIsSet(w *syntax.Word, s string) (bool, error) {
	base, sub, subscripted := r.subscriptOperand(s)
	if !subscripted || !subscriptTextCouldExpand(sub) {
		return r.parameterIsSet(s)
	}
	// A bracket written inside double quotes was still *written*, in the
	// column that reads written brackets — so there is no second round to
	// make, the one expansion the operand already had is the subscript's,
	// and this is the same axis answering a spelling writtenSubscript cannot
	// read. Measured 2026-09-19 with `k='x y'`, `kk='$k'` and the key `x y`
	// in the table: `[[ -v "m[$kk]" ]]` and `[[ -v m"[$kk]" ]]` are unset on
	// bash 5.3.20 where `[[ -v 'm[$k]' ]]` and the same operand out of a
	// value are set, and all four are set on zsh 5.9.2, which reads no
	// written bracket anywhere.
	if doubleQuotedBracketWritten(w) &&
		r.ask(r.sem().ConditionIsSetReadsTheWrittenSubscript,
			"`[[ -v a[k] ]]` reading the subscript the script wrote rather than the one left after expansion") {
		return r.parameterIsSet(s)
	}
	if r.unspecified {
		return false, nil
	}
	if !r.ask(r.sem().ConditionIsSetExpandsAFlatSubscript,
		"`[[ -v ]]` expanding a subscript that reached it as text") {
		return r.parameterIsSet(s)
	}
	key, again := r.expandedSubscriptText(s)
	if !again {
		return r.parameterIsSet(s)
	}
	return r.elementIsSet(base, key, true), nil
}

// doubleQuotedBracketWritten reports whether the operand wrote a `[` inside
// double quotes, which is the one spelling writtenSubscript declines and the
// column that reads written brackets still reads.
//
// Single quotes and a backslash are deliberately not here, and that is the
// measurement rather than a simplification: `[[ -v 'm[$k]' ]]` finds the key
// `x y` on bash 5.3.20 and `[[ -v "m[$kk]" ]]` does not, for the same operand
// text and the same table.
//
// An unquoted bracket is not here either, because a word holding one has
// already been through writtenSubscript — either it read the subscript, or
// the brackets did not balance and nothing downstream will find one.
func doubleQuotedBracketWritten(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, sp := range w.Spans {
		if sp.Kind == syntax.Literal && sp.Quoting == syntax.DoubleQuoted &&
			strings.ContainsRune(sp.Value, '[') {
			return true
		}
	}
	return false
}

// writtenSubscript splits a condition operand into its base and its subscript
// by the brackets the script wrote, before quote removal and expansion.
//
// Only an unquoted literal bracket counts. A bracket inside quotes, one
// behind a backslash — the lexer gives that a span of its own — and one that
// arrives out of a substitution are all content, so `a["x]"]`, `a[x\]]` and
// `a[$k]` each have exactly one subscript however many brackets the value
// holds.
//
// The structure is the same one Runner.subscriptOperand requires of a flat
// operand and for the same recorded reason: the brackets have to balance and
// the closing one has to be the word's last character, so `a[x]]` has a
// bracket nobody opened and `a[q[r]` one nobody closed, and neither is a
// subscripted name. Only the stage differs.
func writtenSubscript(w *syntax.Word) (base, sub *syntax.Word, ok bool) {
	if w == nil {
		return nil, nil, false
	}
	var open, close position
	depth := 0
	for i, sp := range w.Spans {
		if sp.Kind != syntax.Literal || sp.Quoting != syntax.Unquoted {
			continue
		}
		for j := 0; j < len(sp.Value); j++ {
			switch sp.Value[j] {
			case '[':
				if depth == 0 {
					if i == 0 && j == 0 {
						// Nothing before the bracket, so nothing the
						// subscript could be a subscript of.
						return nil, nil, false
					}
					open = position{i, j}
				}
				depth++
			case ']':
				if depth == 0 {
					return nil, nil, false
				}
				depth--
				if depth == 0 {
					close = position{i, j}
				}
			}
		}
	}
	if depth != 0 || (close == position{}) {
		return nil, nil, false
	}
	if close.span != len(w.Spans)-1 || close.off != len(w.Spans[close.span].Value)-1 {
		// The subscript closed before the word ended, so what follows it is
		// neither name nor subscript.
		return nil, nil, false
	}
	return spansBefore(w, open), spansBetween(w, open, close), true
}

// position is one byte of one span: which span, and how far into its value.
type position struct {
	span, off int
}

// spansBefore is the word up to a position, exclusive.
func spansBefore(w *syntax.Word, at position) *syntax.Word {
	out := &syntax.Word{Start: w.Start, Stop: w.Stop}
	out.Spans = append(out.Spans, w.Spans[:at.span]...)
	if head := w.Spans[at.span]; at.off > 0 {
		head.Value = head.Value[:at.off]
		out.Spans = append(out.Spans, head)
	}
	return out
}

// spansBetween is the word strictly between two positions.
func spansBetween(w *syntax.Word, from, to position) *syntax.Word {
	out := &syntax.Word{Start: w.Start, Stop: w.Stop}
	if from.span == to.span {
		one := w.Spans[from.span]
		one.Value = one.Value[from.off+1 : to.off]
		out.Spans = append(out.Spans, one)
		return out
	}
	head := w.Spans[from.span]
	head.Value = head.Value[from.off+1:]
	if head.Value != "" {
		out.Spans = append(out.Spans, head)
	}
	out.Spans = append(out.Spans, w.Spans[from.span+1:to.span]...)
	if tail := w.Spans[to.span]; to.off > 0 {
		tail.Value = tail.Value[:to.off]
		out.Spans = append(out.Spans, tail)
	}
	return out
}

// isSetNameKind reports whether this shell lets `-v` ask about a name of this
// kind at all. A name it will not ask about is *unset* rather than an error:
// all three answer `[[ -v 'a b' ]]` with a plain false.
//
// Two classes are where the panel splits, and both are the shell declining to
// treat the name as a parameter rather than looking and finding nothing —
// `${1+s}` and `${?+s}` are `s` in every one of the three, so the parameters
// are there and it is the operator that does not see them.
//
//	           `-v 1`, `-v 0`   `-v ?`, `-v #`, `-v $`, `-v !`, `-v *`, `-v -`
//	bash 5.3   set              unset
//	ksh93u+    unset            unset
//	zsh 5.9.2  set              set
//
// `@` is the exception that keeps the second column from being "the specials":
// it is unset in all three, with positional parameters set and in the shell
// that answers for every other one. It is excluded by being absent from
// isSpecialParamName and by nothing else — an `@` guard stood here as well
// for a while, and the pair was mutually redundant in the way that hides a
// bug rather than guards against one: with the guard present, *adding* `@` to
// the specials set changed no answer and no test could see it. One place, so
// one mutant.
func (r *Runner) isSetNameKind(name string) bool {
	if isPositionalName(name) {
		return r.sem().ParameterIsSetSeesPositionals
	}
	if isSpecialParamName(name) {
		return r.sem().ParameterIsSetSeesSpecials
	}
	// isNameLike rather than a second name test of this file's own. It
	// trims blanks before it looks, which this question does not want — but
	// the trim cannot leak an answer, because the *lookup* below is on the
	// untrimmed name: `[[ -v "x " ]]` passes the shape test and then finds
	// nothing under `x `, which is the unset all three shells report.
	//
	// A dotted name is a name where the dialect says so, which is how a
	// compound variable's member is asked about: measured 2026-09-13,
	// `c=(a=1); [[ -v c.a ]]` is true on ksh93u+ and `[[ -v c.zz ]]` is
	// false, so the operator looks and finds rather than declining the
	// spelling. Not an axis — the other four dialects never read a `.` as a
	// name character, so the question cannot arise there. See
	// Runner.dottedName.
	return isNameLike(name) || r.dottedName(name)
}

// isPositionalName reports whether a name is a positional parameter or `$0`,
// which is a run of digits and nothing else.
func isPositionalName(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return name != ""
}

// isSpecialParamName reports whether a name is one of the parameters spelled
// as a single punctuation character. `@` is deliberately not here; see
// isSetNameKind.
func isSpecialParamName(name string) bool {
	return len(name) == 1 && strings.IndexByte("?#$!*-", name[0]) >= 0
}

// literalWord is one unquoted literal span as a word, for handing text that
// has already been expanded to something that wants a syntax.Word.
func literalWord(s string) *syntax.Word {
	return &syntax.Word{Spans: []syntax.Span{{Kind: syntax.Literal, Value: s}}}
}
