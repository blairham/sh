// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// braceFanHold is the work one word's expansions did, kept for the other
// names its braces made.
//
// Brace expansion turns one word into several, and every name it makes is the
// word with one alternative substituted into it — so every name carries the
// *same* `$( … )`, the same `$(( … ))` and the same parameter expansions as
// its siblings. Expanding each name on its own therefore runs all of that
// once per name, which is a side effect and not only a count: `i=0; echo
// {x,y,w}$((i++))` left `i` at 3 and `: > {x,y}$(f)` ran `f` twice to make
// two files it could have made from one run.
//
// The panel splits, which is why [Semantics.BraceFanExpandsEachNameOnItsOwn]
// is an axis: bash 5.3.20 and 3.2.57 repeat the word for every name, and zsh
// 5.9.2 and ksh93u+ run it once. The *values* agree in every column — the two
// runs produce the same text — so the count and the variable a run moved are
// the whole of the tell.
//
// The hold is on the substitution rather than on either caller, for the
// reason [subscriptSubstHold] is: an argument and a redirection target reach
// the same expanders by different roads, and a fix installed on one road
// leaves the other doubling. It is keyed by where the span sits in the
// script, and it is put down for the body of a command substitution — the
// program in there is a parse of its own whose positions are its own, and a
// `$( … )` ten bytes into that text is not the one ten bytes into this
// script (#4694).
type braceFanHold struct {
	// armed says a fan of more than one name is running. A hold nothing
	// armed answers for nothing, which leaves every word that is not being
	// fanned on exactly the road it had before.
	armed bool
	// asked and each are the axis, put once for the whole fan. Putting it
	// per name would write the unanswered sentence once per name, which is
	// the doubling this file is about wearing the other face.
	asked bool
	each  bool
	// refused records that the axis had no answer, so the fan stops and the
	// command does not run rather than acting on either reading.
	refused bool
	// vals is the spans already run, keyed by position — a handful at the
	// outside, so a slice searched linearly is the whole of it.
	vals []braceFanVal
}

// braceFanVal is one span's run: where it was written, and what it wrote.
type braceFanVal struct {
	pos syntax.Pos
	val string
}

// eachBraceName runs fn for every name a word's braces made, deciding once
// whether the word's expansions are repeated for each name or run once and
// shared.
//
// One helper for both roads on purpose. An argument and a redirection target
// fan the same way — the argument is what says this belongs to the braces and
// not to the redirection — and the shape this repository keeps finding is the
// second copy that did not get the change.
//
// A *failure* ends the fan, and that is core rather than an axis: `echo
// {x,y}$((1/0))` is one diagnostic in bash 5.3.20, bash 3.2.57, zsh 5.9.2 and
// ksh93u+ alike, whichever way each of them answers the axis. Measured
// against the flags as they stood when the fan began, so a command that had
// already failed is not read as this fan failing.
func (r *Runner) eachBraceName(made []*syntax.Word, fn func(*syntax.Word)) {
	if len(made) < 2 {
		// One name is not a fan: there is nobody to share with and nobody
		// to repeat for, so the hold is not armed and the axis is not put.
		for _, bw := range made {
			fn(bw)
		}
		return
	}
	prev := r.braceFan
	r.braceFan = braceFanHold{armed: true}
	defer func() { r.braceFan = prev }()

	failedBefore := r.expandErr || r.ctl != controlNone
	for _, bw := range made {
		fn(bw)
		if !failedBefore && (r.expandErr || r.ctl != controlNone) {
			return
		}
	}
}

// braceFanRepeatsTheWork puts the axis, once for the fan.
//
// It is put **at the hit** rather than between names, which is what keeps it
// to the place it decides something. A fan whose word has no expansion at all
// never reaches here; nor does one whose expansion sits in a single
// alternative, where no second name carries the span and both readings run it
// once — `{1..$(f),5}` is exactly that, and asking between names refused it
// over a disagreement it does not have.
//
// An unanswered axis abandons the word here rather than on the way out, so
// that nothing runs a second time while the script is being refused.
func (r *Runner) braceFanRepeatsTheWork() bool {
	if !r.braceFan.asked {
		r.braceFan.asked = true
		// Read across the call rather than after it: an earlier axis this
		// script already tripped over leaves the flag set, and this fan is
		// refused only for a question *it* put.
		was := r.unspecified
		r.braceFan.each = r.askBrace(r.sem().BraceFanExpandsEachNameOnItsOwn,
			"a brace fan expanding the word again for each name it made")
		if !was && r.unspecified {
			r.braceFan.refused = true
			r.expandErr = true
		}
	}
	return r.braceFan.each && !r.braceFan.refused
}

// heldBraceFanWork is what this span already wrote for the fan this word is a
// name of — and the one place the axis is decided, because a span found in
// here is a span a second name is about to run again.
func (r *Runner) heldBraceFanWork(pos syntax.Pos) (string, bool) {
	if !r.braceFan.armed {
		return "", false
	}
	for _, v := range r.braceFan.vals {
		if v.pos == pos {
			if r.braceFanRepeatsTheWork() {
				return "", false
			}
			return v.val, true
		}
	}
	return "", false
}

// holdBraceFanWork keeps what one span wrote for the other names of this fan.
//
// It appends and never overwrites, and the lookup takes the first match, so
// the value a position answers with is the one the **first** name put there.
// Replacing instead was tried and taken out: it made a collision between an
// outer span and one inside a substitution's body come out right by accident,
// which is a hazard hidden rather than a hazard fixed — suspendBraceFan is
// what fixes it.
func (r *Runner) holdBraceFanWork(pos syntax.Pos, val string) {
	if !r.braceFan.armed {
		return
	}
	r.braceFan.vals = append(r.braceFan.vals, braceFanVal{pos: pos, val: val})
}

// suspendBraceFan puts the hold down for a program of its own and returns the
// call that picks it up again.
//
// A command substitution's body is a separate parse, so its spans are at
// positions of its own and a hold keyed by position would answer the wrong
// span for them. It also runs commands, which can fan braces of their own and
// must get a hold of their own to do it.
func (r *Runner) suspendBraceFan() func() {
	if !r.braceFan.armed {
		return func() {}
	}
	was := r.braceFan
	r.braceFan = braceFanHold{}
	return func() { r.braceFan = was }
}
