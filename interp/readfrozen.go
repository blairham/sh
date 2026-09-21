// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A `read` whose name is frozen, which is the same species as `getopts`
// writing OPTARG and the same defect #3147 fixed there — reached by a
// different door, so a second change rather than that one.
//
// The write went through setVar, which is the path a *bare assignment* takes,
// and a bare assignment a freeze refuses gives up the rest of what the shell
// was running. So `q=1; readonly q; printf 'z\n' | read q; echo "st=$?"` never
// printed the status in any of the five dialects, and in the two whose
// refusal is fatal it ended the script. Every shell on the panel but zsh runs
// both.
//
// Measured 2026-09-16, a script file under `env -i`, `a=A; b=B; readonly a;
// printf 'x y\n' | { read a b; echo "st=$?"; printf '[%s][%s]' "$a" "$b"; }`:
//
//	                     sentence                      st  a    b    tail
//	bash 5.3.20          a: readonly variable           2  A    B    runs
//	bash as `sh`         the same                       2  A    B    runs
//	bash 3.2.57          the same                       0  A    y    runs
//	ksh93u+ 2012-08-01   read: warning: a: is read only 1  A    y    runs
//	dash 0.5.12          read: a: is read only          2  A    B    runs
//	BusyBox ash 1.37.0   read: line N: a: is read only  2  A    B    runs
//	zsh 5.9.2            read-only variable: a          —  —    —    **ends**
//
// Three things are separable in that table and each has an answer of its own
// below: whether the refusal ends the *builtin* — b is filled in ksh93 and
// not in the other three — what status it leaves, and whether it ends the
// *script*, which is ReadonlyRefusalInABuiltinIsFatal and zsh alone.

// readRefusedStatus is what a `read` reports when a freeze stopped one of its
// writes and the builtin gave up there.
//
// Two of the three that stop answer 2 whatever happened; bash answers 1 when
// there was no name left to fill, which is the reading
// ReadRefusedWriteIsOneOnTheLastName carries. A `read` with no operands at
// all has no last name for that to be about and answers 2 there — measured,
// `REPLY=R; readonly REPLY; printf 'x\n' | { read; echo "st=$?"; }` is 2 in
// bash 5.3.20 and in 3.2.57 alike.
const readRefusedStatus = 2

// readMayWrite reports whether a freeze lets `read` fill one of its names,
// having written the refusal where it does not.
//
// The freeze is asked of the name the value would *land* on: a subscripted
// operand is refused by its array's freeze and named by it — measured,
// `a=1; readonly a; read 'a[0]'` is `a: readonly variable` in bash 5.3.20,
// the base name and not the operand as written.
func (r *Runner) readMayWrite(name string) bool {
	target := r.frozenReadName(name)
	if !r.readonly[target] {
		return true
	}
	fatal := r.ask(r.sem().ReadonlyRefusalInABuiltinIsFatal,
		"a builtin's refused write to its own output parameter ending the script")
	// The *target* in the sentence and not the operand as written, which is
	// the same name the freeze was asked about: measured, `a=1; readonly a;
	// read 'a[0]'` is `a: readonly variable` in bash 5.3.20 and `read:
	// warning: a: is read only` in ksh93 — the array, not `a[0]`.
	//
	// assignedByBuiltin, the form #3147 added: the builtin names itself where
	// the dialect names one — `read: a: is read only` in dash and BusyBox
	// ash, `read: warning: a: is read only` in ksh93 — and the rest of the
	// line is not given up, which is what this fixes.
	r.reportReadonlyRefusal(target, assignedByBuiltin, fatal)
	return false
}

// frozenReadName is the name a `read` operand's freeze is asked about: the
// array a subscript belongs to, or whatever a reference points at.
func (r *Runner) frozenReadName(name string) string {
	if base, sub, ok := r.subscriptOperand(name); ok && isPlainName(base) {
		if r.operandEmptySubscript(sub) {
			// The brackets name no element in this column, so the array's
			// freeze is not what the operand is refused by: measured
			// 2026-09-17, `a=(1 2 3); readonly a; read 'a[]'` is the
			// bad-name refusal in bash 5.3.20 and `not an identifier: a[]`
			// in zsh 5.9.2, neither of them `a: readonly variable`, where
			// ksh93 reads element zero and does refuse it by the freeze. The
			// store answers it a step below; see storeOperandEmptySubscript.
			return r.assignmentLandsOn(name)
		}
		return base
	}
	return r.assignmentLandsOn(name)
}

// readRefusalEndsTheBuiltin reports whether a refused write stops `read` where
// it stands, leaving the names after it alone.
//
// Asked only after a refusal, so an ordinary `read` over unfrozen names
// reaches no axis at all.
func (r *Runner) readRefusalEndsTheBuiltin() bool {
	return r.ask(r.sem().ReadRefusedWriteEndsTheBuiltin,
		"a freeze on one of `read`'s names ending the builtin rather than only being reported")
}

// readFrozenStatus is the status a refused write leaves, given how many names
// were still to come and whether the name was one the script wrote.
func (r *Runner) readFrozenStatus(left int, written bool) int {
	if left == 0 && written && r.ask(r.sem().ReadRefusedWriteIsOneOnTheLastName,
		"a freeze on `read`'s last name reporting 1 rather than 2") {
		return 1
	}
	return readRefusedStatus
}
