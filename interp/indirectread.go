// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The **target** of a `${!v}` indirection: what the text `v` came to names.
//
// It is a *parameter reference* and not a variable name. Measured 2026-09-16,
// `env -i` with a scratch HOME and no startup files, from a script file, with
// `set -- p q r`, `a=(A B C)` and the text on the left held in `v`:
//
//	v holds   bash 5.3.20   bash 3.2.57   here, before
//	@         <p><q><r>     <p><q><r>     <>
//	*         <p q r>       <p q r>       <>
//	#         <3>           <3>           <>
//	0         <./s.sh>      <./s.sh>      <>
//	1         <p>           <p>           <>
//	?         <0>           <0>           <>
//	a[1]      <B>           <B>           <>
//	a[@]      <A><B><C>     <A><B><C>     <>
//	a[*]      <A B C>       <A B C>       <>
//	_ok       <>            <>            <>   the control
//
// The two bash columns agree on every row, so this is bash's language and not
// an age. Nothing else on the panel reaches the question: ksh93u+ yields the
// *name* it was written on (Semantics.IndirectionYieldsName), and zsh 5.9.2,
// dash 0.5.12 and BusyBox ash 1.37.0 have no such expansion at all — `bad
// substitution` in the first two and a syntax error in the third.
//
// Reading the text with getVar, which is the plain-name store, answered every
// one of those with the empty string at status 0 — a wrong *value* and not a
// missing diagnostic, which is the shape this repository minds most. `${!v}`
// over `$1`, `$@` and `$#` is how a bash function reaches its caller's
// arguments through a name it was handed (#3213).
//
// **The subscript is live text and is evaluated exactly once.** Measured on
// the same day with `a=(x y z); i=0; v='a[i++]'`: `${!v}` is `x` and leaves
// `i` at 1 in both bash columns, two spellings in one word leave it at 2, and
// `d='a[$(echo SUB >&2; echo 1)]'` writes SUB once. So this read is a read and
// not a name lookup, and the once is what readParamSource being called once
// per expansion gives — the same guarantee #3104 wrote down for the source and
// the subscript of an ordinary expansion.

// indirectTargetNode is the node the resolved text of a `${!v}` spells: a
// reference where the text carries a subscript, a plain name otherwise.
//
// referenceNode is what builds it, which is the point — the `(P)` flag reads
// its resolved text through the same function, so a subscript this shell
// answers on one road is answered on the other rather than by a second copy
// that would drift. No outer node is handed over: the operators written around
// a `${!v}` act on what the target came to and not on how it was reached, so
// the node carries the reference's own subscript and nothing else.
func (r *Runner) indirectTargetNode(e *syntax.ParamExpr, text string) *syntax.ParamExpr {
	// Parsed once for the span. The brackets of a resolved target are built
	// here rather than written in the script, so a second parse hands the
	// next reader a *different* subscript word — and a substitution in it
	// runs again, which is the count the written spelling no longer pays.
	// See subscriptSubstHold (#3240); interp/indirectreference_test.go pins
	// the two spellings against each other for exactly this.
	if held, ok := r.heldReferenceNode(text); ok {
		return held
	}
	node := r.referenceNode(text, nil, e.Src)
	r.holdReferenceNode(text, node)
	return node
}

// indirectTargetValue is the value behind a resolved indirection, and whether
// the target was set at all.
//
// readParamSource rather than paramSource, because the hold is keyed by the
// node an expansion was *written* as and this node was built here: holding it
// would put the target of one `${!v}` in front of the next one's read. The
// once-per-expansion guarantee comes from this being the only caller in the
// expansion instead.
func (r *Runner) indirectTargetValue(e *syntax.ParamExpr, text string) (string, bool) {
	value, set, _ := r.readParamSource(r.indirectTargetNode(e, text))
	return value, set
}

// indirectAimedAtAList is the half of the indirection that the **list** path
// has to answer: a target naming the positional parameters or the whole of an
// array keeps its fields, exactly as writing it out would.
//
// Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57 alike, with `set -- p q
// r` and `a=(A B C)`: `v='@'` makes `"${!v}"` three fields and `v='a[@]'`
// makes it three, where `v='*'` and `v='a[*]'` are one joined field each — the
// same three-way split `"$@"`, `"${a[@]}"`, `"$*"` and `"${a[*]}"` make when
// they are written directly. A scalar read cannot carry that: it joins, and an
// element holding a space would have been lost.
//
// So the node *becomes* the target and everything below answers it, which is
// the rewrite namerefAimedAtTheWholeArray and bareArrayAsList already make on
// this path. Only for the plain spelling — no operator, no length, no
// subscript of its own — which is the shape the two bash columns agree about.
// An operator on a list-valued target is three questions and none of them is
// this one: the trims map over the elements in both columns and join here,
// `@Q` arrived after bash 3.2, and a slice takes the list in bash 5.3 and a
// substring of the join in bash 3.2 — `${!v:1}` on `v='a[@]'` is `B C`
// against ` B C` — which wants an axis before either is written down. The
// table is in #3243.
//
// The axis is read rather than asked here. An unanswered
// Semantics.IndirectionYieldsName has to be reported once, and the scalar path
// is where it is reported: asking twice would write the line twice for one
// expansion.
func (r *Runner) indirectAimedAtAList(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if !e.Indirect || e.Inner != nil || e.Index != nil || e.Length ||
		e.Prefix != 0 || e.Op != syntax.ParamNone {
		return nil, false
	}
	if r.sem().IndirectionYieldsName != No {
		// The dialect that answers with the name never reads the target, and
		// one that has not chosen is the scalar path's to report.
		return nil, false
	}
	if _, is := r.namerefTarget(e.Name); is {
		// A name reference answers this spelling with the name it points at
		// and is not a double read at all. The scalar path holds that
		// measurement; here it only has to stay out of the way.
		return nil, false
	}
	text, set, _ := r.paramSource(e)
	if !set || text == "" {
		return nil, false
	}
	if !r.indirectNamesAList(text) {
		return nil, false
	}
	return r.indirectTargetNode(e, text), true
}

// indirectNamesAList reports whether a resolved text names the positional
// parameters or the whole of an array — the four spellings whose fields
// survive a quoted expansion.
//
// Asked of the **text** and not of the parsed node, which is the same care
// referenceKeepsFields takes and for the same reason: wholeArrayIndex reads
// the subscript, and a subscript holding a command substitution would run it
// here and again where the reference is read. Measured, `a=(x y z); d='a[$(
// echo IND >&2; echo 1)]'; ${!d}` writes IND once in bash 5.3.20, and asking
// the node instead added a run this road did not have before.
func (r *Runner) indirectNamesAList(text string) bool {
	if text == "@" || text == "*" {
		return true
	}
	base, sub, ok := r.subscriptOperand(text)
	return ok && isNameLike(base) && (sub == "@" || sub == "*")
}

// indirectSubject is the parameter an indirection's refusal names.
//
// **The `!` is written back**, and the name is the one the *source* holds
// rather than the target the text resolved to. Measured 2026-09-16 under
// `set -u` from a script file, with `a=(x y z)`, `v=nope`, `w='a[9]'` and
// `q=1` with no positional parameters:
//
//	                bash 5.3.20                    bash 3.2.57
//	${!v}           v: invalid indirect expansion  !v: unbound variable
//	${!q}           !q: unbound variable           !q: unbound variable
//	${!w}           !w: unbound variable           !w: unbound variable
//	${!a[9]}        !a[9]: unbound variable        !a[9]: unbound variable
//	${!v?msg}       !v: msg                        !v: msg
//	${!a[9]?msg}    !a[9]: msg                     !a[9]: msg
//
// so a subscript that was written is written back inside the `!` and the
// resolved target is never named. The two columns part on one row only — the
// outer name unset, which is 5.3's own sentence and a separate question
// (#2891) — and this shell says `unbound variable` there, which is 3.2's.
//
// The dialect that yields the *name* writes no `!`, and that is measured too:
// ksh93u+ 2012 refuses `${!v}` with `v: parameter not set` and
// `${!nosucharr[9]}` with `nosucharr[9]: parameter not set`. So the sigil goes
// with the indirection being *taken*, which Semantics.IndirectionYieldsName
// already decides — no second axis, and no column that could disagree with the
// derivation, since the other four have no `${!…}` at all.
func (r *Runner) indirectSubject(e *syntax.ParamExpr) string {
	written := e.Name
	if e.Index != nil {
		written += "[" + r.unboundSubscript(e) + "]"
	}
	if r.sem().IndirectionYieldsName == Yes {
		return written
	}
	return "!" + written
}

// refuseIndirection is the refusal a `${!v}` makes of what `v` came to before
// any operator is considered, and whether it made one.
//
// Two sentences, and which one is about the **source**. Measured 2026-09-16
// from a script file, one expansion per line:
//
//	                               bash 5.3.20                         bash 3.2.57
//	v never declared               v: invalid indirect expansion       [], 0
//	v=""                           : invalid variable name             [], 0
//	v=-3, v='x y', v='a[1]b'       -3: invalid variable name (etc.)    [], 0
//	declare v, local v, a=()       [], 0                               [], 0
//	v=1, v=@, v='a[1]', v=_        the target read                     the same
//
// So a name that exists and holds nothing is not refused — `declare x` and an
// empty array are the control rows — and a name nothing ever declared is,
// written with the subscript the script gave it (`${!nosuch[3]}` is
// `nosuch[3]: invalid indirect expansion`, where a declared `a` answers
// `${!a[9]}` in silence). The operators do not save either: `${!u-D}`,
// `${!u?msg}` and `${!v:-D}` over `v=-3` are the same refusals, and under
// `set -u` the refusal is made instead of `unbound variable`.
//
// What it costs is the same as a bad substitution in that dialect — the line
// is given up, which is Semantics.FailedExpansionAbandonsTheLine, and bash as
// `sh` writes the same two sentences. An empty wording is the reading with no
// refusal, which is bash 3.2's and the core's (#2891, #3215).
func (r *Runner) refuseIndirection(e *syntax.ParamExpr, value string, set bool) bool {
	d := r.diag()
	switch {
	case !set && d.IndirectionUndeclared != "":
		if _, declared := r.declarationOf(e.Name); declared || r.nameIsSet(e.Name) {
			return false
		}
		written := e.Name
		if e.Index != nil {
			written += "[" + r.unboundSubscript(e) + "]"
		}
		r.diagf("%s\n", Wording(d.IndirectionUndeclared, "", written))
	case set && d.IndirectionNotAName != "" && !indirectTextIsAReference(value):
		r.diagf("%s\n", Wording(d.IndirectionNotAName, "", value))
	default:
		return false
	}
	r.expandErr = true
	return true
}

// indirectTextIsAReference is whether the text a `${!v}` came to is one of the
// parameter references the table at the top of this file reads: a name, a
// positional parameter, a special parameter, or a name with one balanced
// subscript closing the text. Nothing is trimmed — ` a` and `a ` are refused.
func indirectTextIsAReference(text string) bool {
	if len(text) == 1 && strings.ContainsRune("@*#?-$!0", rune(text[0])) {
		return true
	}
	if text == "" {
		return false
	}
	if allDigits(text) {
		return true
	}
	name := text
	if open := strings.IndexByte(text, '['); open > 0 {
		if !strings.HasSuffix(text, "]") || !subscriptBracketsBalance(text[open:]) {
			return false
		}
		name = text[:open]
	}
	if name == "" || isDigit(name[0]) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; c != '_' && !isLetter(c) && !isDigit(c) {
			return false
		}
	}
	return true
}
