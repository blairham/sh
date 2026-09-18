// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// refusalZeroIsRaisedBackToOne reports whether the shell is somewhere that
// turns a give-up's **0** back into 1.
//
// One dialect leaves 0 behind when a store through an operand refuses, where
// every other refusal it makes leaves 1 — and that 0 is not always what a
// caller reads. Three places raise it, and all three were measured 2026-09-17
// on zsh 5.9.2, `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=$d`, stdin
// /dev/null, a fresh directory, each program run twice — once as a script
// file and once as one `-c` string — with the refusal wrapped in `( … )` so
// the number is readable at all:
//
//	a=(1 2 3)
//	( <the refusal> )
//	echo "next=$?"
//
//	                                        next=   file   -c
//	( typeset "a[b c]"=v )                            0     0
//	( typeset "a[b c]"=v && echo yes )                1     1
//	( typeset "a[b c]"=v || echo no )                 0     0
//	( ! typeset "a[b c]"=v )                          0     0
//	( if typeset "a[b c]"=v; then echo t; fi )        0     0
//	( while typeset "a[b c]"=v; do break; done )      1     1
//	( while :; do typeset "a[b c]"=v; break; done )   1     1
//	( until typeset "a[b c]"=v; do break; done )      0     0
//	( for i in 1; do typeset "a[b c]"=v; done )       1     1
//	( { typeset "a[b c]"=v; } )                       0     0
//	( { typeset "a[b c]"=v; } && echo yes )           1     1
//	( true && typeset "a[b c]"=v )                    0     0
//	( typeset "a[b c]"=v; echo tail )                 0     0
//	( echo z | typeset "a[b c]"=v )                   0     0
//	( case x in x) typeset "a[b c]"=v;; esac )        0     0
//	( ( typeset "a[b c]"=v ) )                        0     0
//
// and the **top level of a script file** is the third, which is what the
// process exit status shows: the same refusal alone at the top level exits 1
// from a file and 0 from `-c`, and so does `set -A 1bad v`, which is a
// different refusal in a different builtin and answers every row above the
// same way.
//
// So the three raisers are: the left operand of an `&&` list, a loop, and a
// script file's own top level. `||` and `until` continue on a *failure* and
// leave the 0 standing, which is what makes the first of those about `&&`
// rather than about and-or lists.
//
// **The route was never the rule**, which is what
// StoreRefusalOfADeclaredElementLeavesZero used to say: #1770 measured two
// rows, both alone at the top level of a file and of a `-c` string, where the
// file/`-c` difference is real — and read it as a rule about how the program
// reached the shell. A subshell in a **file** leaves 0 there, and an `&&` list
// under **`-c`** leaves 1, so both halves of the old condition were wrong
// (#3504).
//
// One function for the three sites that ask — the declaration's store, `set
// -A`'s bad name and a `printf -v` refusal — because it is one measurement and
// a second copy that omitted a raiser is how this repository keeps re-finding
// the same bug.
func (r *Runner) refusalZeroIsRaisedBackToOne() bool {
	if !r.inSubshell && r.Route != RouteCommandString {
		// A script file's top level. The shell is ending either way and the
		// number is the process's, which that route reports as 1 whatever
		// the refusal left.
		return true
	}
	if r.loopDepth > r.subshellLoopFloor {
		return true
	}
	return r.andLeftOperand > 0
}

// refusalLeavesZero is the whole of what a site does with the answer: ask the
// axis, check the raisers, and write both the runner's status and the number
// the builtin returns, which have to agree for the reason setArrayOperands
// gives — controlExit is what Run reports and the builtin's own return value
// is read separately.
func (r *Runner) refusalLeavesZero(a Answer, what string, status int) int {
	if r.ctl != controlExit || !r.ask(a, what) || r.refusalZeroIsRaisedBackToOne() {
		return status
	}
	r.status = 0
	return 0
}

// storeRefusalStatus is what a refused store through a **builtin's operand**
// leaves behind, given what the give-up itself left.
//
// One column parts the two builtins that reach this store: a refused `printf
// -v` leaves 0 there and a refused `read` leaves 1, at the unevaluable
// subscript and at the empty one alike. See
// Semantics.StoreRefusalThroughPrintfLeavesZero for the rows.
//
// Keyed by the builtin because the answer is the builtin's, which is how the
// bad-name machinery next door is keyed for the same pair — and one door for
// both refusals, because they are one measurement and a second copy that
// answered only the arithmetic one would leave `printf -v 'a[]'` behind.
func (r *Runner) storeRefusalStatus(builtin string, status int) int {
	if builtin != "printf" {
		return status
	}
	return r.refusalLeavesZero(r.sem().StoreRefusalThroughPrintfLeavesZero,
		"a refused store through `printf -v` leaving 0 behind", status)
}
