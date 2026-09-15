// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A **name reference**: a parameter whose value is another parameter's *name*,
// through which every read, every write and every `unset` reaches that other
// parameter.
//
// Measured 2026-09-15 against bash 5.3.15 and ksh93u+ 2012-08-01, `env -i`
// with a scratch HOME and no startup files. The two shells agree on all of
// this, which is why it is the core's and not an axis — what they disagree
// about is in the two fields at the bottom of this file.
//
//	v=1; typeset -n r=v; echo "$r"            1
//	v=1; typeset -n r=v; r=2; echo "$v"       2
//	v=1; typeset -n r=v; r+=x; echo "$v"      1x
//	v=1; typeset -n r=v; echo "${!r}"         v
//	v=(a b c); typeset -n r=v; echo "${r[1]}" b
//	v=(a b c); typeset -n r=v; r[1]=Z         a Z c
//	typeset -A m=([k]=1); typeset -n r=m      ${r[k]} is 1
//	typeset -n r=v; v=1; (( r++ )); echo "$v" 2
//	v=1; typeset -n r=v; read r <<< zz        v is zz
//	v=1; typeset -n r=v; unset r              **v** is gone
//	v=1; typeset -n r=v; unset -n r           r is gone and v is 1
//	typeset -n r="a[2]"; a=(x y z)            $r is z, and r=Q writes a[2]
//
// # It is a redirect, not a copy
//
// The target is resolved **when the name is used** and never when the
// reference is made: `typeset -n r=v; v=5; echo "$r"` writes 5, and the
// declaration happens before `v` exists at all. That is why this is a table
// consulted by the stores rather than anything written into one.
//
// # Three things that are *not* a read through the reference
//
//   - **`typeset -p r` lists the reference**, `declare -n r="v"` — the name
//     it points at, not the value it reaches. So the listing asks the table
//     directly and never goes through resolution.
//   - **`${!r}` is the target's name.** Not an indirection through the
//     reference's value, which is what the same spelling means for an
//     ordinary parameter: `v=1; typeset -n r=v; echo "${!r}"` is `v` in both
//     shells, where a double read would have looked for a parameter called
//     `1`.
//   - **`for r in x y` re-points the reference** rather than writing through
//     it. Measured in both: after the loop `typeset -p r` is `declare -n
//     r="y"` and `v` is untouched. It is the same rule as the one below —
//     the loop's assignment is an assignment — read from the other side.
//
// # An assignment to a reference with no target sets the target
//
// `typeset -n r; r=v` makes `r` a reference to `v`, and `typeset -p r` then
// writes `declare -n r="v"`. So a reference that points nowhere is not a
// reference to the empty name: the first assignment is what aims it. With a
// target already set the same assignment writes *through*, which is the two
// halves of namerefAssignment.
//
// # Where the resolution happens
//
// At the **stores**, not at the expansion. Every reader of a parameter in
// this package goes through a small number of funnels — varValue for a
// scalar, arrayElems and assocFor for the two containers, setVarAs and the
// element stores for the writes, unsetName for the removal — and putting the
// redirect there is what makes `${r:-d}`, `${#r}`, `${r/a/b}`, `case $r`,
// `[[ -v r ]]`, `$(( r + 1 ))` and every other reader correct by
// construction rather than one at a time. The alternative — resolving in the
// expansion — would have left every builtin that takes a *name* operand
// (`read`, `getopts`, `printf -v`) reaching around it.

// namerefDepth bounds how far a chain is followed.
//
// A reference to a reference is real — `typeset -n r=v; typeset -n s=r`
// reads `v` through both — so the chain has to be walked, and a cycle is a
// state both shells can be talked into: bash takes `typeset -n a=b; typeset
// -n b=a` at status 0 and warns only when the name is read. A depth is what
// makes that a finite answer rather than a hang, and it is deliberately not a
// visited-set: the two shells report a cycle and carry on rather than
// refusing, so what this needs is a stop and not a diagnosis.
const namerefDepth = 32

// namerefTarget follows the chain to the name a read or a write really lands
// on. The second result is false when the name references nothing, in which
// case the name itself is the answer and no caller has to special-case it.
func (r *Runner) namerefTarget(name string) (string, bool) {
	target, _, aimed := r.namerefWalk(name)
	return target, aimed
}

// namerefWalk is namerefTarget with the third thing a walk learns: whether it
// came back to where it started, which is the cycle the warning is about.
func (r *Runner) namerefWalk(name string) (target string, cycle, aimed bool) {
	start := name
	t, ok := r.nameref[name]
	if !ok {
		return name, false, false
	}
	for i := 0; ok && i < namerefDepth; i++ {
		if t == "" {
			// A reference aimed at nothing, which is what `typeset -n r`
			// leaves before its first assignment has aimed it.
			return name, false, false
		}
		if t == start {
			// Round the loop and back to the name the read started from.
			return start, true, false
		}
		name = t
		t, ok = r.nameref[name]
	}
	return name, false, true
}

// A target may be an **element** rather than a name: `typeset -n r="a[2]"`
// reads and writes that element in both shells. The split is
// [Runner.indirectElement], which the `(P)` flag already needed for the same
// shape — one helper, because a second one is how the fix for a subscript
// that is live text rather than a literal reaches one caller and not the
// other.

// throughNameref is the funnels' one line: the name the store should use.
//
// A target carrying a subscript comes back *whole*, because the two callers
// that can act on one — the scalar read and the scalar write — are the two
// that split it, and a funnel that could not would rather hold a name it will
// not find than an element it would read as a variable literally called
// `a[2]`.
func (r *Runner) throughNameref(name string) string {
	target, _ := r.namerefTarget(name)
	return target
}

// throughNamerefName is throughNameref for the one caller that may not
// evaluate a subscript: the scalar read.
//
// [Runner.varValue] is reached from the locale's own parameter read, which is
// reached from `echo`, which is in the builtin table — so a reference from
// there to the arithmetic a subscript needs closes a package initialization
// cycle. The element reading is done in paramSource instead, which is not on
// that path, and this leaves a reference aimed at an element resolving to
// itself rather than to a name it would not find.
func (r *Runner) throughNamerefName(name string) string {
	target, cycle, is := r.namerefWalk(name)
	if cycle {
		r.warnAboutACycle(name)
		return name
	}
	if !is {
		return name
	}
	if _, _, element := r.indirectElement(target); element {
		return name
	}
	return target
}

// namerefReadsAnElement answers the expansion's half of the split above: the
// value a reference aimed at `a[2]` reads, and false for every other
// reference.
//
// Called from paramSource, which is where a `${r}` becomes a value. A builtin
// that takes a *name* operand — `read`, `getopts` — does not come this way,
// so a reference aimed at an element is not one of those builtins' targets
// yet; a reference aimed at a plain name is, because that half is resolved at
// the store.
func (r *Runner) namerefReadsAnElement(name string) (string, bool, bool) {
	target, is := r.namerefTarget(name)
	if !is {
		return "", false, false
	}
	base, sub, element := r.indirectElement(target)
	if !element {
		return "", false, false
	}
	v, set := r.readThroughNamerefElement(base, sub)
	return v, set, true
}

// readThroughNamerefElement reads the one element a reference to `a[2]` is
// aimed at, through whichever of the two containers the base is.
func (r *Runner) readThroughNamerefElement(base, sub string) (string, bool) {
	if r.assocDeclared(base) {
		a, ok := r.assocFor(base)
		if !ok {
			return "", false
		}
		v, there := a[sub]
		return v.Str, there
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		return "", false
	}
	elems, ok := r.arrayElems(base)
	if !ok {
		return "", false
	}
	return r.elemAt(base, elems, idx)
}

// storeThroughNamerefElement is the other half, and it is written the way the
// `(P)` flag's assignment is for the same reason: the two questions a
// subscript raises — which container, and what the text evaluates to — have
// one answer each and both are already settled elsewhere.
func (r *Runner) storeThroughNamerefElement(base, sub, value string) {
	if r.assocDeclared(base) {
		r.setAssocElem(base, sub, value)
		return
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(sub, err))
		return
	}
	r.setArrayElem(base, idx, sub, value)
}

// namerefAssignmentTarget answers what an assignment to `name` really does.
//
// write is false where the value **aimed** the reference instead of being
// written through it, which the assignment's caller then has nothing left to
// do about. Measured in bash 5.3.15 and ksh93u+ alike: `typeset -n r; r=v`
// leaves `declare -n r="v"` and creates no parameter called `r`, where
// `typeset -n r=v; r=x` leaves `v` holding `x`. One rule read twice — a
// reference with nothing to point at is aimed by its first value — and it is
// also what `for r in x y` does, which is why the loop re-points the
// reference in both shells rather than writing through it.
func (r *Runner) namerefAssignmentTarget(name, value string) (target string, write bool) {
	if !r.isNameref(name) {
		return name, true
	}
	target, aimed := r.namerefTarget(name)
	if aimed {
		return target, true
	}
	r.setNameref(name, value)
	return name, false
}

// isNameref reports whether a name is a reference, which is what a listing
// and `${!r}` ask.
func (r *Runner) isNameref(name string) bool {
	_, ok := r.nameref[name]
	return ok
}

// setNameref aims a reference.
func (r *Runner) setNameref(name, target string) {
	if r.nameref == nil {
		r.nameref = map[string]string{}
	}
	r.nameref[name] = target
}

// namerefSelfReference reports whether aiming `name` at `target` would make a
// reference that reaches itself, which both shells complain about.
//
// The walk is over the *existing* table with the new edge laid on top, which
// is what catches `typeset -n a=b; typeset -n b=a`: neither edge is a self
// reference on its own.
func (r *Runner) namerefSelfReference(name, target string) bool {
	if target == name {
		return true
	}
	seen := target
	for i := 0; i < namerefDepth; i++ {
		next, ok := r.nameref[seen]
		if !ok {
			return false
		}
		if next == name {
			return true
		}
		seen = next
	}
	return true
}

// namerefTargetIsAName reports whether a word may be aimed at: a name, or a
// name with a subscript. Both shells refuse anything else at the declaration.
func (r *Runner) namerefTargetIsAName(target string) bool {
	if isNameLike(target) {
		return true
	}
	base, _, bracketed := strings.Cut(target, "[")
	return bracketed && isNameLike(base) && strings.HasSuffix(target, "]")
}

// declareNameref is one operand of a declaration carrying the `n` letter.
//
// The letter does not store anything: `typeset -n r=v` records that `r` is a
// reference to `v`, and `typeset -n r` records that `r` is a reference with
// nothing to point at yet — which the first assignment through it then aims.
// Both shells list the second as `declare -n r` and `typeset -n r`.
//
// Two refusals, and they are per dialect rather than shared because the
// wording and the reach differ. Both shells refuse a target that is not a
// name, and both complain about a reference that reaches itself; what they do
// about the second one is [Semantics.NamerefCycleIsRefused], where the
// measurements are.
func (r *Runner) declareNameref(builtin, name, target string, hasValue bool) int {
	if !hasValue {
		if !r.isNameref(name) {
			r.setNameref(name, "")
		}
		// A second `typeset -n r` over a reference that is already aimed
		// leaves it aimed, measured in both: the letter says what the name
		// *is*, and saying it twice says nothing new.
		return 0
	}
	d := r.diag()
	if !r.namerefTargetIsAName(target) {
		return r.refuseNameref(builtin, Wording(d.NamerefBadTarget,
			"%[1]s: invalid variable name for name reference", target))
	}
	if target == name {
		// A reference aimed straight at itself, which **both** shells refuse
		// — measured, `typeset -n r=r` is `nameref variable self references
		// not allowed` in bash 5.3.15 and `invalid self reference` in
		// ksh93u+ — so it is the core's answer and not the axis below.
		return r.refuseNameref(builtin, Wording(d.NamerefSelfReference,
			"%[1]s: invalid self reference", name))
	}
	if r.namerefSelfReference(name, target) {
		if r.ask(r.sem().NamerefCycleIsRefused, "a name reference that reaches itself") {
			return r.refuseNameref(builtin, Wording(d.NamerefSelfReference,
				"%[1]s: invalid self reference", name))
		}
		if r.unspecified {
			return r.status
		}
		// The other answer: the reference is made, silently, and the
		// complaint arrives when something reads through it. Measured on
		// bash 5.3.15 — `declare -n a=b; declare -n b=a` is a silent 0 and
		// `echo "$a"` is then `warning: a: circular name reference` followed
		// by an empty line — so nothing is said here. See warnAboutACycle,
		// which is the read's half.
	}
	r.setNameref(name, target)
	return 0
}

// unsetNameref takes the reference attribute off a name, which is what
// `unset -n` does and what an ordinary `unset` does not: the plain spelling
// removes what the reference points at and leaves the reference aimed at a
// name that is now gone.
func (r *Runner) unsetNameref(name string) { delete(r.nameref, name) }

// warnAboutACycle is the read's half of Semantics.NamerefCycleIsRefused's
// second answer: the dialect that lets a cycle be built says so when
// something reads through one.
//
// Spoken as the *shell* and not as a builtin, measured: `bash: line 1:
// warning: a: circular name reference`, where the refusals on the same
// builtin carry `declare:` in front of them. The read still answers — an
// empty value, which is what a name resolving to itself already gives — so
// this adds a sentence and changes no value.
func (r *Runner) warnAboutACycle(name string) {
	if w := r.diag().NamerefCircularWarning; w != "" {
		r.DiagnoseAsTheShellf("%s\n", Wording(w, "warning: %[1]s: circular name reference", name))
	}
}

// refuseNameref reports a declaration this shell will not make, and gives up
// the script where the dialect gives one up.
//
// The fatality is BadNameToDeclarationFatal and not an axis of its own,
// because that is the question being asked: both refusals here are about the
// *name* on a declaration's operand — one is not a name at all and the other
// is a name that cannot stand where it was written — and ksh93 counts
// `typeset` among its special builtins, so both end the script there and
// neither does in bash. Measured 2026-09-15: `typeset -n r=r; echo st=$?`
// writes `st=1` in bash 5.3.15 and nothing at all in ksh93u+.
func (r *Runner) refuseNameref(builtin, wording string) int {
	r.diagf("%s: %s\n", builtin, wording)
	if r.ask(r.sem().BadNameToDeclarationFatal, "a declaration's bad name ending the script") {
		r.status = 1
		r.fatalUsageQuiet()
		return r.status
	}
	if r.unspecified {
		return r.status
	}
	return 1
}

// What is measured and deliberately **not** modeled: the `n` letter's
// company.
//
// The two shells refuse different pairs and in different words, and neither
// refusal is the one they give a bad option. Measured 2026-09-15:
//
//	declare -in r=v    bash  status 1 and **no diagnostic at all**
//	declare -an r=v    bash  status 0
//	declare -rn r=v    bash  status 0
//	typeset -in r=v    ksh93 typeset's whole usage block, and the script ends
//	typeset -an r=v    ksh93 `-an: invalid variable name`
//	typeset -rn r=v    ksh93 the usage block again
//
// So a shell that refused the pairs one of them refuses would take a line the
// other runs, and bash's silent 1 has no wording to carry. Both shells refuse
// `-i` beside `-n` and that much could be shared; the rest could not, and a
// rule built from the one row they agree on would be a rule about the letter
// rather than about either shell. Left as it is: `-n` is read and so is
// whatever stands beside it, which is bash's answer for four of the six rows
// above and ksh93's for none. It is the corner of a corner — no script in the
// wild writes a reference and an attribute on one word — and it is written
// down here so the next reader does not have to measure it again.
