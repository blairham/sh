// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A call's assignment prefix standing over a **name reference**.
//
// The prefix's write follows the reference, so the cell the command is shown
// is the *target's* — and everything the prefix does to a name it therefore
// does to the target: the save, the fresh cell, the export attribute and the
// take-back alike. A prefix that stored through the reference while saving the
// reference saved the wrong cell, so the write landed on a variable the call
// never named and there was nothing to put back (#4110).
//
// Measured 2026-09-21 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, `target=T; declare -n foo=target` and `ff() {
// declare -p foo; }`:
//
//	                       bash 5.3.20            ksh93u+ 2012
//	declare -p foo inside  declare -n foo="target"  typeset -n foo=target
//	declare -p target      declare -x target="bar"  target=bar
//	target afterwards      declare -- target="T"    target=bar
//	the child's env        target=bar               target=bar
//
// So the reference itself is untouched in both — it keeps the `n` letter and
// takes no `x` — and it is the **target** that carries the prefix's value, the
// export attribute, and the entry the child is handed.
//
// # The panel does not want an axis for this
//
// The two columns part only on whether the write survives, and that is the
// axis this shell already has: ksh93 is the one column where a prefix in front
// of a POSIX-form function persists at all, and `plain=P; ff() { :; };
// plain=bar ff` leaves `plain` at `bar` there with no reference in sight. So
// the reference row is AssignmentPrefixPersistsAfterAFunction reaching its
// target rather than a question of its own, and nothing here asks a new one.
//
// bash 3.2.57 cannot be asked: `declare -n` is `invalid option` there, so the
// outer declaration never happens and the row measures nothing. zsh 5.9.2 has
// no `-n` letter either — `typeset -n` is `bad option` — so this is a
// bash-and-ksh question, and the two of them answer it the same way.
//
// # Where the reference is its own target
//
// Two shapes point at no name a write can land on, and for them
// Runner.assignmentLandsOn answers with the reference itself:
//
//	declare -n foo            a reference with nothing to point at
//	declare -n foo="a[1]"     a reference aimed at an *element*
//
// Both then reach the fresh cell carrying the `n` letter, and the fresh cell
// takes it off — which is what makes the store that follows an ordinary store
// to `foo` rather than a write through the reference. That is measured, and it
// is the whole of why these two rows part between the columns without a second
// question being asked:
//
//	                                    bash 5.3.20        ksh93u+ 2012
//	declare -n foo; foo=bar ff          declare -x foo="bar"  typeset -n foo=bar
//	a=(x y z); declare -n foo="a[1]";
//	  foo=bar ff, ${a[*]} inside        x y z                 x bar z
//
// bash makes the fresh cell and so writes a plain scalar; ksh93 does not and
// so aims the reference in the first row and writes the element in the second.
// Both fall out of Semantics.AssignmentPrefixMakesAFreshCell, which is already
// Yes in the one column and No in the other — see interp/prefixfreshcell.go.

// prefixEntryName is the name a call's assignment prefix makes its entry on,
// which is where its value lands and not the word the script wrote.
//
// The rule itself is Runner.assignmentLandsOn — deliberately the same
// function a plain assignment's freeze asks, and not a second copy of the
// walk, because the two answers to "where does this write go" have to be one
// answer. A reference the prefix followed while the take-back saved the name
// as written is exactly the bug this is here to stop.
func (r *Runner) prefixEntryName(name string) string {
	return r.assignmentLandsOn(name)
}
