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

// arrayStoreRefusalStatus is what a refused whole-array store through `set
// -A`, `set +A` or `read -A` leaves behind, given what the give-up itself
// left. See Semantics.ArrayStoreRefusalThroughSetOrReadLeavesZero for the
// rows, and the two sites for which refusals reach it: the freeze and the
// fixed slot's kind, which answer alike.
//
// One door for the three spellings, through the same refusalLeavesZero the
// bad name and the declaration's store go through, so the raisers are the
// ones measured for those rather than a copy (#6486).
func (r *Runner) arrayStoreRefusalStatus(status int) int {
	return r.refusalLeavesZero(r.sem().ArrayStoreRefusalThroughSetOrReadLeavesZero,
		"a refused whole-array store through `set -A` or `read -A` leaving 0 behind", status)
}

// RefusesAWholeArrayStore reports whether a registered builtin's store of a
// whole array — or, with table set, a whole association — into name is
// refused, having written the refusal; status is what the builtin returns.
//
// The checks `set -A` and `read -A` make in front of their own whole-array
// store, offered to a builtin outside this package so that it asks the same
// ones rather than a copy: the freeze, and for an indexed array a slot of a
// fixed scalar kind (interp/privatekindfixed.go). The number is
// arrayStoreRefusalStatus's, so a fatal refusal leaves 0 from `-c` in the
// column that answers so. Measured 2026-10-15 on zsh 5.9.2, `-f`, `env -i
// PATH=/usr/bin:/bin`, `-c` then a script file:
//
//	typeset -r v=1; zparseopts -a v a     read-only variable: v   0   1
//	typeset -r v=1; zparseopts -A v a     the same                0   1
//	typeset -rA m;  zparseopts -A m a     the same                0   1
//	zparseopts -a HOME a                  HOME: attempt to assign array
//	                                      value to non-array      0   1
//	typeset -r v=1; ( zparseopts -A v a ); echo $?            0 by both
//
// SetAssoc consults no freeze at all, which is how `zparseopts -A` over a
// readonly name was taken silently (#6513).
//
// **An association over a slot of a fixed kind** is the third check, and it
// comes first: the shell's own parameters and a call's private refuse a whole
// table with the kind sentence a `-A` letter gets, ahead of the freeze.
// Measured 2026-10-15 on zsh 5.9.2, `-f`, `env -i PATH=/usr/bin:/bin`, `-c`
// then a script file (#6554):
//
//	zparseopts -A HOME a           HOME: can't change type of a special
//	                               parameter                    0   1
//	zparseopts -A path a           the same, for path, fpath, IFS, RANDOM,
//	                               SECONDS, argv, status and PATH
//	readonly HOME; zparseopts -A HOME a      the kind sentence, not the freeze's
//	f(){ private v=1; zparseopts -A v a; }   f: v: the kind sentence
//	f(){ private -r v=1; zparseopts -A v a; }  the same
//	f(){ local HOME; zparseopts -A HOME a; }   the same
//
// and the controls take the table: `options` and `commands`, which are
// associations already; `ZSH_NAME`; `f(){ private -A v; … }`; `f(){ local -h
// HOME; … }`, a hidden shadow that is an ordinary name. A script's tie is
// taken too, and is **untied**, the partner going as an `unset` takes it —
// `typeset -T FOO=a:b foo; zparseopts -A FOO a` leaves FOO an association and
// foo gone, and with `typeset -r foo` standing the store is `read-only
// variable: foo` instead. That is what a `-A` letter does to the same tie, so
// it is asked through the letter's own rule rather than a copy of it.
func (r *Runner) RefusesAWholeArrayStore(name string, table bool) (status int, refused bool) {
	if table && r.fixedSlotRefusesATable(name) {
		return r.arrayStoreRefusalStatus(r.status), true
	}
	if r.refuseReadonly(name, assignedByDeclaration) {
		return r.arrayStoreRefusalStatus(1), true
	}
	if !table && r.fixedSlotRefusesAnArrayValue(name) {
		return r.arrayStoreRefusalStatus(r.status), true
	}
	if table && r.kindLetterUntiesAScriptTie(name, declareFlags{letters: "A", letterSigns: "-"}) {
		return r.arrayStoreRefusalStatus(r.status), true
	}
	return 0, false
}
