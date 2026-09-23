// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A `-g` declaration whose value is an **array literal**, which reaches the
// shell as a command operand assigned after the builtin has returned rather
// than as part of the declaration.
//
// The letter's rule is the same one the scalar spelling asks —
// Semantics.DeclareGlobalReachesPastALocal — and the route never put the
// question. The scalar goes through setGlobalVar, which walks the scopes; the
// literal goes through Runner.assignOperands and the ordinary assignment,
// which writes the cell that is *visible*. So a declaration standing under a
// local, or under a call's assignment prefix, wrote over what it was supposed
// to write under.
//
// Measured 2026-09-18 on bash 5.3.20 and zsh 5.9.2, from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and no
// `set -o posix`:
//
//	y=(9 9); f(){ local y=(5); declare -ga y=(1 2); echo "[${y[@]}]"; }; f
//	                                  bash  inside [5]      top [1 2]
//	                                  zsh   inside [1 2]    top [9 9]
//
//	z=(9 9); g(){ declare -ga z=(1 2); }; g
//	                                  bash  top [1 2]       zsh top [1 2]
//
//	w=(9 9); h(){ declare -ga w=(1 2); }; w=7 h
//	                                  bash  top [1 2]       zsh top [9 9]
//
// The middle row is the control and is what says the letter reaches this
// route at all: with nothing standing on the name both columns write the
// shell's own cell, and so did this engine. The first and third are the two
// things that can stand in the way, and this engine answered zsh in both —
// which is the axis going *unasked* rather than answered `No`, since zsh's
// `inside [1 2]` and bash's `inside [5]` are exactly the two readings it
// records for a scalar.
//
// The seam is the one the container letters already use: what the builtin
// read off its option word is recorded on the runner as it reads it, and the
// assignments that run afterwards read the record. See
// Runner.indexedLetterHere, whose reason for existing is the same sentence —
// the parser hands the utility a bare name and the operand assignment cannot
// see the letters.
//
// The lift is the one interp/staticscope.go already builds for a sealed call:
// the shadow comes off, the assignment runs against the cell underneath, and
// what it left there is written into the scope's own saved copy so the return
// gives it back. A swap and not a copy, for the reason recorded there.

// globalOperandsRunOnTheShellsOwnCell takes the local shadows and the
// enclosing calls' assignment prefixes off every name a `-g` declaration is
// about to assign an array literal to, and hands back the function that puts
// them on again.
//
// Only names carrying **both** the letter and a literal operand, so an
// ordinary `declare -g s=1` — which setGlobalVar already answers, and answers
// at the store rather than around it — never reaches this and never asks the
// axis twice.
func (r *Runner) globalOperandsRunOnTheShellsOwnCell() func() {
	var names []string
	for name := range r.globalLetterHere {
		if r.literalOperands[name] {
			names = append(names, name)
		}
	}
	if names == nil {
		return func() {}
	}
	// The prefix half first and by the same helper the declaration itself
	// uses, so the two routes cannot come to disagree about which frame holds
	// the shell's own cell. It answers for the names a call's prefix is
	// standing on and leaves the rest to the walk below.
	// The running command's own prefix first and the enclosing call's second,
	// which is the order the declaration itself lifts them in and for the
	// same reason: the inner one is standing on top of the outer.
	putOwnPrefixBack := r.globalDeclarationRunsUnderItsOwnPrefix(names)
	putPrefixesBack := r.globalDeclarationRunsOnTheShellsOwnCell(names)
	var lifted []liftedShadow
	for _, name := range names {
		sc := r.outermostScopeHolding(name)
		if sc == nil {
			continue
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal,
			"`declare -g` with an array literal writing past a local of the same name") {
			// The column that writes what is visible, which is what this
			// route did for every name before the question was asked here.
			continue
		}
		lifted = append(lifted, liftedShadow{name: name, holder: sc, held: r.captureBinding(name)})
		r.installBinding(name, bindingFromScope(sc, name))
	}
	return func() {
		for i := len(lifted) - 1; i >= 0; i-- {
			l := lifted[i]
			writeBindingToScope(l.holder, l.name, r.captureBinding(l.name))
			r.installBinding(l.name, l.held)
		}
		putPrefixesBack()
		putOwnPrefixBack()
	}
}

// globalStoreRunsOnTheShellsOwnCell is the same lift for the one name a
// **quoted** array literal is about to be stored under, and it exists because
// that operand never reaches the route above.
//
// The machinery next door is keyed on Runner.literalOperands, which is filled
// from the parser's `c.Assigns` — and a literal whose parentheses the quoting
// hid is not an assignment the parser saw. `declare -ga "a=( 1 2 )"` is one
// ordinary word, re-read by interp/quotedarrayliteral.go and stored by
// assignArrayLiteral, which writes the cell that is *visible*. So the letter
// was read, recorded and then had nothing to act on: a local of the same name
// took the value and the shell's own cell was left alone.
//
// Measured 2026-09-23 on bash 5.3.20 from a script file, a local standing on
// the name in each row:
//
//	declare -ga "a=( x y )"     bash  inside []   top [x y]
//	declare -ga  a=( x y )      bash  inside []   top [x y]
//	declare -g  "s=x"           bash  inside []   top x
//
// The middle row is the control and is the one this engine already answered:
// unquoted, the parser sees the literal and the route above lifts the shadow.
// The third is the control for the other half — the scalar spelling goes
// through setGlobalVar, which walks the scopes, and was right all along. Only
// the quoted literal had no counterpart, and it wrote the local in every
// shape asked: `-gA` with a table, `+=` appending, `typeset` for the name,
// and the name arriving from an expansion (#4163).
//
// One name rather than a set, because this is called from inside the operand
// loop where the name being stored is known — the route above runs after the
// builtin has returned and has to rediscover them.
func (r *Runner) globalStoreRunsOnTheShellsOwnCell(name string) func() {
	sc := r.outermostScopeHolding(name)
	if sc == nil {
		return func() {}
	}
	if !r.ask(r.sem().DeclareGlobalReachesPastALocal,
		"`declare -g` with an array literal writing past a local of the same name") {
		// The column that writes what is visible, which is what this route
		// did for every name before the question reached it.
		return func() {}
	}
	l := liftedShadow{name: name, holder: sc, held: r.captureBinding(name)}
	r.installBinding(name, bindingFromScope(sc, name))
	return func() {
		writeBindingToScope(l.holder, l.name, r.captureBinding(l.name))
		r.installBinding(l.name, l.held)
	}
}

// liftedShadow is one name's local shadow, taken off for the length of a
// global declaration's operand assignment.
type liftedShadow struct {
	name string
	// holder is the scope whose saved copy *is* the shell's own cell while
	// the shadow stands, and which is handed what the assignment wrote.
	holder *scope
	// held is the shadow itself, put back when the assignment is done.
	held staticBinding
}

// outermostScopeHolding is the scope whose saved copy is the shell's own cell
// for a name — the **oldest** one that shadowed it, which is the value the
// last return will put back.
//
// Outermost and not innermost, which is the opposite of what a sealed call
// wants and is right for the opposite reason: a seal is reading past *one*
// call's declaration, and this is writing past all of them. setGlobalVar
// walks the stack the same way for the scalar spelling, so the two routes
// name the same cell.
func (r *Runner) outermostScopeHolding(name string) *scope {
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; saved {
			return sc
		}
	}
	return nil
}
