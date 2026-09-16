// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// subscriptSubstHold is one subscript's substitutions, run once and kept for
// the rest of that expansion.
//
// #3104 gave a subscript one *arithmetic* evaluation per expansion, so
// `a=(x y z); i=0; ${a[i++]-D}` leaves `i` at 1. The *word expansion* of the
// same brackets was not part of that, and it was done once per reader: the
// text road (subscriptText, through expandWordNoSplit) and the as-written
// road (subscriptAsWritten, through searchOperand) each expand the subscript
// word themselves, and the several questions one expansion asks — does the
// subscript name the whole array, is it a range, what does it evaluate to —
// walk both. A command substitution written into the brackets therefore ran
// four times for `${a[$(f)]}` and ten for `${a[$(f)-D]}`.
//
// Every reference shell on the panel that has arrays runs it **once**:
// measured 2026-09-16 with `$(printf "@" >>"$C"; printf "MARK\n" >&2; echo 1)`
// as the subscript, bash 5.3.20, ksh93u+ 2012 and zsh 5.9.2 write one mark for
// every one of the eight readings, and bash 3.2.57 writes one for five of them
// and two for the three that re-read the value (`#`, `/` and `:off:len`).
// The values agreed throughout, so the count was the only tell — the same
// shape #3104 had.
//
// So the hold is not on either road. It is on the *substitution*, keyed by
// where the span sits in the script, and armed for exactly the word a
// subscript is being read from. Both roads reach [Runner.commandSubst], so
// caching there is what lets one run serve a reader that only ever asks for
// the rendered text and a reader that asks for it with its quotes still on —
// which is why equalizing the two roads separately would have left two runs
// rather than one.
//
// An arithmetic substitution written into the brackets is held the same way
// and for a stronger reason: there the repeated evaluation was not only a
// count. `a=(x y z); i=0; ${a[$((i++))]-D}` left `i` at **10** and substituted
// the word where bash 5.3.20 and ksh93u+ 2012 both answer `x` with `i` at 1 —
// the silent shape #3104 found for a bare `a[i++]`, in the one spelling of it
// that goes through the word expansion rather than through the subscript's own
// arithmetic. Only a successful evaluation is held: a failed one has already
// said so and set the expansion's error flag, and the word is abandoned.
//
// Scope is the enclosing expansion of the node, armed in [Runner.expandAt]:
// one hold per expansion of one `${…[…]…}`, restored on the way out so a
// subscript nested inside another's body gets its own. A `for` loop expands
// its node once per pass and each pass arms afresh, which is the rule
// sourceHold and subscriptHold are already kept to.
type subscriptSubstHold struct {
	// armed says a span's expansion opened this hold. A hold that nothing
	// opened answers for nothing, which is what keeps a reader that reaches
	// a subscript from outside an expansion on the road it had before.
	armed bool
	// word is the written subscript word this hold belongs to, where the
	// expansion has one.
	word *syntax.Word
	// refText and refNode are a resolved indirection's target, parsed once
	// for this span. `${!d}` with `d='a[$(f)]'` reads its target through a
	// node built from that text, and building it again for each of the four
	// readers gave each of them brackets of its own — a fresh word, which
	// no hold keyed on a word can recognize. See referenceNode.
	refText string
	refNode *syntax.ParamExpr
	// vals is the spans already run, keyed by position. A subscript holds a
	// handful of spans at the outside, so a slice searched linearly is the
	// whole of it — and it keeps the order a reader would see.
	vals []subscriptSubstVal
}

// subscriptSubstVal is one span's run: where it was written, and what it
// wrote.
type subscriptSubstVal struct {
	pos syntax.Pos
	val string
}

// armSubscriptSubsts arms the hold for one subscript word and returns the
// call that gives the previous one back.
//
// The previous hold is restored rather than cleared, because a substitution
// in a subscript can hold an expansion with a subscript of its own, and the
// outer brackets are still mid-read when the inner ones finish.
func (r *Runner) armSubscriptSubsts(s syntax.Span) func() {
	if s.Kind != syntax.ParamExp || s.Param == nil {
		// Not an expansion, so nothing here reads a subscript. The enclosing
		// hold is left where it is: the spans of a subscript word are
		// themselves expanded by a word loop, and a reset there would empty
		// the hold the brackets were opened with.
		return func() {}
	}
	sub := s.Param.Subscript()
	if r.subscriptSubsts.armed && r.subscriptSubsts.word == sub && sub != nil {
		// Already armed by an enclosing reader of the same brackets. The
		// list half of an expansion arms and then falls through to the
		// scalar half, which arms again; re-installing there would empty the
		// hold between two readers of one expansion, which is the bug.
		return func() {}
	}
	prev := r.subscriptSubsts
	r.subscriptSubsts = subscriptSubstHold{armed: true, word: sub}
	return func() { r.subscriptSubsts = prev }
}

// armedForSubscript reports whether the hold speaks for these brackets: the
// written subscript of the expansion that opened it, or the subscript of the
// target a resolved indirection in it parsed.
func (r *Runner) armedForSubscript(w *syntax.Word) bool {
	if w == nil || !r.subscriptSubsts.armed {
		return false
	}
	if r.subscriptSubsts.word == w {
		return true
	}
	ref := r.subscriptSubsts.refNode
	return ref != nil && ref.Subscript() == w
}

// heldReferenceNode is the target node a resolved indirection already parsed
// in this span, if the text is the one it was parsed from.
func (r *Runner) heldReferenceNode(text string) (*syntax.ParamExpr, bool) {
	h := r.subscriptSubsts
	if !h.armed || h.refNode == nil || h.refText != text {
		return nil, false
	}
	return h.refNode, true
}

// holdReferenceNode keeps a resolved indirection's target node for the rest of
// this span, so every reader of it reads one set of brackets.
func (r *Runner) holdReferenceNode(text string, e *syntax.ParamExpr) {
	if !r.subscriptSubsts.armed {
		return
	}
	r.subscriptSubsts.refText, r.subscriptSubsts.refNode = text, e
}

// heldSubscriptSubst is what this span already wrote in this expansion, if
// the span belongs to the armed subscript and has already run.
func (r *Runner) heldSubscriptSubst(w *syntax.Word, pos syntax.Pos) (string, bool) {
	if !r.armedForSubscript(w) {
		return "", false
	}
	for _, v := range r.subscriptSubsts.vals {
		if v.pos == pos {
			return v.val, true
		}
	}
	return "", false
}

// holdSubscriptSubst keeps what one span wrote for the rest of this
// expansion.
func (r *Runner) holdSubscriptSubst(w *syntax.Word, pos syntax.Pos, val string) {
	if !r.armedForSubscript(w) {
		return
	}
	r.subscriptSubsts.vals = append(r.subscriptSubsts.vals, subscriptSubstVal{pos: pos, val: val})
}
