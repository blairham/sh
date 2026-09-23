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
// this path.
//
// **An operator written around it comes with it**, and that is the half #3243
// filed. The operator acts on what the target came to, so a target that is a
// list is a list for the operator too — it maps over the elements exactly as
// it does when the target is written out. Measured 2026-09-18 on bash 5.3.20
// with `set -- p q r`, `a=(A B C)`, `at='@'` and `ea='a[@]'`, counting fields:
//
//	${!ea#A}    3:<><B><C>          ${!ea/A/z}   3:<z><B><C>
//	${!ea%C}    3:<A><B><>          ${!ea//A/z}  3:<z><B><C>
//	${!ea@Q}    3:<'A'><'B'><'C'>   ${!ea,,}     3:<a><b><c>
//	${!at@Q}    3:<'p'><'q'><'r'>   ${!ea-D}     3:<A><B><C>
//	${!at#p}    3:<><q><r>          ${!ea+S}     1:<S>
//
// Every one of those was **one joined field** here, so `${!ea#A}` trimmed an
// `A` off the front of a joined string instead of off each element and three
// fields came back as one. bash 3.2.57 agrees on the trims and drops the field
// an operator emptied where 5.3 keeps it, which is a second and much smaller
// question; `@Q` arrived after 3.2 and that column's refusal is its own answer
// rather than a disagreement.
//
// The **slice** is the one shape the two columns really part on — `${!at:1}`
// is three fields in 5.3 and one substring of the join in 3.2 — and it needs no
// axis of its own here: once the node is the target, `${!at:1}` *is* `${@:1}`
// and Semantics.SubstringOfPositionalsSlicesTheList is already the field that
// decides it. Answering it anywhere else would be a second copy of a question
// this shell has.
//
// Length and the prefix listing stay out. `${!name@}` is a different
// construct that never resolves a target at all, and a length is answered
// before any of this.
//
// The axis is read rather than asked here. An unanswered
// Semantics.IndirectionYieldsName has to be reported once, and the scalar path
// is where it is reported: asking twice would write the line twice for one
// expansion.
func (r *Runner) indirectAimedAtAList(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if !e.Indirect || e.Inner != nil || e.Index != nil || e.Length || e.Prefix != 0 {
		return nil, false
	}
	if r.sem().IndirectionYieldsName != No {
		// The dialect that answers with the name never reads the target, and
		// one that has not chosen is the scalar path's to report.
		return nil, false
	}
	if _, cycle, is := r.namerefWalk(e.Name); is || cycle {
		// A name reference answers this spelling with the name it points at
		// and is not a double read at all. The scalar path holds that
		// measurement; here it only has to stay out of the way.
		//
		// **A reference that comes back to itself is the same answer**, and
		// leaving it out of this test was one read too many: the walk finds
		// no target, so the value was read here to see whether it named a
		// list — and the read walks the loop and warns about it, where the
		// shell being measured says nothing at all and refuses the
		// indirection. Measured 2026-09-18 on bash 5.3.20, `r=OUTER; f(){
		// local -n r=r; echo "${!r}"; }; f`: the declaration's own two
		// warnings and then none. The scalar path refuses it without reading
		// either (#3122).
		return nil, false
	}
	text, set, _ := r.paramSource(e)
	if !set || text == "" {
		return nil, false
	}
	if !r.indirectNamesAList(text) {
		return nil, false
	}
	node := r.indirectTargetNode(e, text)
	if e.Op == syntax.ParamNone {
		return node, true
	}
	return aimedWithTheOperator(e, node), true
}

// aimedWithTheOperator is the outer expansion with its **parameter** replaced
// by the target the indirection resolved to, and everything else left exactly
// as it was written.
//
// This way round rather than the other, and that is the point. Copying the
// operator onto the target's node would mean listing every field an operator
// owns — the word, the second word, the two raw texts, the global flag, the
// anchor, the transform letter, the colon, the unreadable operand — and a
// field added to that set later would silently stop traveling. The set this
// lists instead is what says *which parameter* the expansion is about, which
// is small, and a new member of it is a new way to name a parameter rather
// than a new operator.
//
// The target's node is never written to. It is held for the rest of the span
// (see Runner.indirectTargetNode), so a mutation here would reach the next
// reader of the same text.
func aimedWithTheOperator(e *syntax.ParamExpr, node *syntax.ParamExpr) *syntax.ParamExpr {
	aimed := *e
	aimed.Indirect = false
	aimed.Name = node.Name
	aimed.Index = node.Index
	aimed.IndexText = node.IndexText
	aimed.IndexFlags = node.IndexFlags
	aimed.IndexRange = node.IndexRange
	aimed.IndexDots = node.IndexDots
	aimed.Leading = node.Leading
	aimed.BareIndexText = nil
	return &aimed
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
//
// **Only a name is ever undeclared.** The first sentence is about a *name*
// nothing ever brought into being, and a positional parameter is not one: it
// is unset or it is not, and an unset one is the ordinary unset road with the
// `!` on its subject. Measured 2026-09-21, bash 5.3.20 from a script file
// with no positional parameters at all:
//
//	                        bash 5.3.20                here, before
//	set -u; ${!1}           !1: unbound variable       1: invalid indirect expansion
//	set -u; ${!9}           !9: unbound variable       9: invalid indirect expansion
//	${!1}                   [] at 0                    1: invalid indirect expansion
//	set -u; ${!1-D}         D at 0                     1: invalid indirect expansion
//	set -u; ${!1?msg}       !1: msg                    1: invalid indirect expansion
//	set -- ""; ${!1}        : invalid variable name    the same
//	set -- nope; set -u     !1: unbound variable       the same
//	${!nodecl}              nodecl: invalid …          the same
//
// So the refusal that was reached for a digit cost the operators their word,
// cost `set -u` its own sentence, and refused a line bash runs in silence.
// The row that says this is the *undeclared* sentence and not the whole road
// is the fourth: `${!1-D}` is `D`, where a name nothing declared is refused
// through the operator (#3985).
// aimed is the **target text** of an aimed reference, empty where the written
// name is not one. The declaration the refusal asks after is that text's and
// not the reference's: `declare -n foo=bar` with nothing ever declaring `bar`
// makes `${!foo[2]}` `foo[2]: invalid indirect expansion` in bash 5.3.20,
// while `declare -a bar` beside it is silent — so what decides is the cell the
// reference points at, and asking the reference's own name finds a declaration
// that was never the one in question, `foo` being declared by the very line
// that made it a reference. The *written* word is still what is named,
// subscript and all; only the question moves.
//
// The text is asked **whole**, subscript and all, rather than reduced to the
// cell it lives in — and that is measured rather than tidy. A reference aimed
// at an element, with a subscript written over the top of it, is refused by
// bash however well declared the array is: `declare -a bar=(p q r); declare -n
// foo='bar[1]'` makes `${!foo[2]}` and `${!foo[0]}` both `invalid indirect
// expansion`. Asking after `bar` answers that it is declared and goes silent,
// which is a row this change got wrong before the row was measured.
func (r *Runner) refuseIndirection(e *syntax.ParamExpr, value string, set bool, aimed string) bool {
	d := r.diag()
	asked := e.Name
	if aimed != "" {
		asked = aimed
	}
	switch {
	case !set && d.IndirectionUndeclared != "" && isNameLike(e.Name):
		if _, declared := r.declarationOf(asked); declared || r.nameIsSet(asked) {
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
