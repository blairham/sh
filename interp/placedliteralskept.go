// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A declaration's array literal **placed** ahead of a utility that then
// refuses.
//
// Where Semantics.DeclarationArrayOperandsAreStoredInTurn says so, the walk
// over a declaration's literals places each one at its turn (#6798). Placed
// is stored: what the utility then does — refuse the word, refuse another
// operand — does not take the store back. Measured 2026-11-02 on bash 5.3.20
// from script files under `env -i PATH=/usr/bin:/bin`:
//
//	local -a q=(1 2); declare -p q          `local: can only be used in a
//	                                        function`, then
//	                                        declare -a q=([0]="1" [1]="2")
//	local -A m=([k]=v)                      declare -A m=([k]="v" )
//	local -i n=(1+1)                        declare -ai n=([0]="2")
//	local -l u=(AB Cd)                      declare -al u=([0]="ab" [1]="cd")
//	local -x q=(1 2), local -r w=(1 2)      plain arrays, neither exported
//	                                        nor frozen
//	local -a s=(1) t=2                      `s` the array, `t` not found
//	readonly r=1; declare -a c=(5) r d=(6)  `declare: r: readonly variable`
//	                                        at 1, `c` and `d` holding 5 and 6
//	f(){ declare -a c=(5) 'x-y' d=(6); }    the call's `c` and `d` hold 5
//	                                        and 6, and are gone after it
//	readonly r=1; f(){ declare -a c=(5) r=(b) d=(6); }
//	                                        the same, with `r` refused at its
//	                                        own turn — see
//	                                        Runner.settleALiteralOperand
//
// bash 3.2.57 keeps the top-level `q` the same way, and crashes on the last
// row. This shell held each literal while the operands behind it expanded,
// put it back before the utility ran, and stored it only after a utility
// that succeeded, so every row above came out empty (#7103).
//
// So the store after the utility makes the placed literals whatever the
// utility's status, with the value letters the line carries where the utility
// declared nothing at all — the letters a refused letter leaves the literals
// too, see interp/refusedlineliterals.go. A literal left to the store rather
// than placed — an element literal, one through a tie — keeps the store's own
// rule and is skipped when the utility refuses.
//
// The walk that stores every operand in turn is a different walk, and none of
// this reaches it: what a utility's refusal leaves there is
// DeclarationOperandsAreStoredInTurn's question, and unmodeled.

// literalsPlacedOutliveTheUtility reports whether the running declaration
// placed any of its array literals at their turn, and the utility that then
// refused left the shell carrying on — so the store after it still makes them.
func (r *Runner) literalsPlacedOutliveTheUtility() bool {
	if r.ctl != controlNone || r.unspecified {
		return false
	}
	for _, op := range r.arrayOperands {
		if op.placed {
			return true
		}
	}
	return false
}

// arrayOperandLeftOut reports whether the store after the utility leaves this
// literal operand alone: one the walk refused at its turn, whatever the
// utility did, and with placedOnly one the walk did not place.
func (r *Runner) arrayOperandLeftOut(a *syntax.Assign, placedOnly bool) bool {
	for _, op := range r.arrayOperands {
		if op.assign == a {
			return op.refused || (placedOnly && !op.placed)
		}
	}
	return placedOnly
}

// literalOperandRefusedInTurn reports whether the running declaration's
// literal operand on name was refused at its turn, so the store has spoken
// already and does not speak again beside the utility's own refusal. See
// Runner.storeSpeaksBeforeTheBuiltin.
func (r *Runner) literalOperandRefusedInTurn(name string) bool {
	for _, op := range r.arrayOperands {
		if op.refused && op.assign.Name == name {
			return true
		}
	}
	return false
}

// declarePlacedLiteralsAlone declares the literals the walk placed, where the
// utility refused before declaring anything: written is the line's words
// after the utility's own and known the letters the utility takes. They are
// declared on the shell's own cell with the letters that shape their values,
// and the store after the utility then makes them.
func (r *Runner) declarePlacedLiteralsAlone(written []string, known string) {
	if r.ctl != controlNone || r.unspecified {
		return
	}
	var names []string
	for _, op := range r.arrayOperands {
		if op.placed {
			names = append(names, op.assign.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	opts, _, _ := r.literalValueOptions(written, known)
	r.declareLiteralsAlone(opts, names, false)
}
