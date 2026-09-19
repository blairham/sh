// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/blairham/sh/syntax"
)

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

// namerefAimedAtTheWholeArray answers a reference aimed at **all** of an
// array — `typeset -n r=a[@]`, `typeset -n r=a[*]` — with the node that
// expansion stands for, so that every reading of the name goes on to be the
// reading of `${a[@]}` or `${a[*]}`.
//
// Distinct from Runner.namerefReadsAnElement one function down, which answers
// a reference aimed at *one* element and answers it with a string. A whole
// array is not a string in any of the ways that matter: it is one field per
// element under `[@]` even inside quotes, one joined field under `[*]`, and
// `${#r}` is the element count rather than a length. Reading it as a scalar
// meant Runner.subscriptValue was handed `@`, refused it as arithmetic, and
// the reference came back **empty at status 0** — a silent wrong answer, and
// under `set -u` a fatal one, which is what left `nameref.tests` of bash's
// own suite exiting 127 where bash exits 0 (#2299).
//
// Measured on bash 5.3.20, 2026-09-16, with `a=(x y z)`:
//
//	typeset -n r=a[@]; printf "<%s>" "$r"      <x><y><z>   was <>
//	typeset -n r=a[*]; printf "<%s>" "$r"      <x y z>     was <>
//	typeset -n r=a[*]; printf "<%s>" $r        <x><y><z>   was <>
//	typeset -n r=a[@]; echo ${#r}              3           was 0
//	typeset -A m=([k]=v [j]=w); typeset -n r=m[@]; echo "$r"
//	                                           v w         was empty
//
// and under `set -u` with `a` never set, `typeset -n r=a[@]; : "$r"` is
// silent at 0 there, because `${a[@]}` on an unset array is not an unbound
// parameter in bash and never was.
//
// The rewrite rather than a reading of its own, for the reason
// Runner.bareArrayAsList is a rewrite: what a whole-array subscript means is
// already written once, and a second copy of it would drift. The parse comes
// from Runner.reference, which is the same door the `(P)` flag opens on the
// same text.
//
// Not for `${!r}`, which is the *target's own text* — `a[@]` — in bash and
// already answers that way, and not for a node that carries a subscript of
// its own: `${r[1]}` and `${#r[@]}` subscript the reference, which is not an
// array, and bash answers both with nothing.
func (r *Runner) namerefAimedAtTheWholeArray(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if e == nil || e.Index != nil || e.Indirect || e.Prefix != 0 || e.Inner != nil {
		return nil, false
	}
	target, is := r.namerefTarget(e.Name)
	if !is {
		return nil, false
	}
	ref, ok := r.reference(target)
	if !ok || !r.wholeArrayIndex(ref) {
		return nil, false
	}
	aimed := *e
	aimed.Name, aimed.Index, aimed.IndexFlags = ref.Name, ref.Index, ref.IndexFlags
	return &aimed, true
}

// namerefSplicesItsArray is namerefAimedAtTheWholeArray asked by the two
// callers that decide **fields** rather than a value, and it is where the
// spelling parts: only the bare `$r` splices the array, and `${r}` is a
// scalar read of the reference whose value is the elements joined.
//
// Measured 2026-09-19 on bash 5.3.20, `a=(aa bb cc)` and `declare -n r=a[@]`,
// `env -i PATH=/usr/bin:/bin LC_ALL=C` under `-c`, a script file and standard
// input alike:
//
//	printf "<%s>" "$r"     <aa><bb><cc>   the splice
//	printf "<%s>" "${r}"   <aa bb cc>     one field
//	IFS=-; "${r}"          <aa-bb-cc>     joined by IFS, not by a space
//	${r:0:2}               aa             a substring of the join, where
//	                                      ${a[*]:0:2} is `aa bb` — so the
//	                                      braced spelling is not `[*]` either
//	${r@Q}                 'aa bb cc'     one quoted string
//	set -u, a never set    r: unbound variable, where `$r` is silent
//
// Four discriminators and no two of them are the same operator, which is what
// says this is the *spelling* rather than a rule about any one of them. The
// first three are wrong **values** at status 0, which is the shape the
// earlier reading of this construct kept producing (#2299) and the reason it
// is worth a line rather than a diagnostic.
//
// `${#r}` is the exception and it is measured, not conceded: it is the
// element **count** in that shell — `a=(aaa bbb); ${#r}` is `2` where the
// join is seven characters long — so the length block wants the splice under
// either spelling and asks for it here.
//
// One caller-side test rather than two copies of the rewrite: this returns
// what namerefAimedAtTheWholeArray returns and cannot come to a different
// node than the scalar path does.
//
// bash 5.3 is the only column that can be asked — zsh has no `-n` letter at
// all, bash 3.2 has none, and ksh93 refuses this target at the declaration
// (#3124) — so this is the core's reading rather than an axis.
func (r *Runner) namerefSplicesItsArray(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if e != nil && !e.Bare && !e.Length {
		return nil, false
	}
	return r.namerefAimedAtTheWholeArray(e)
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
		if r.arithNounsetNamedTheParameter {
			// `set -u` naming a name the *subscript* read, which is the one
			// failure here that is not this reference's: measured
			// 2026-09-18, `set -u; declare -n r=a[b]; : "$r"` is `b: unbound
			// variable` in bash 5.3.20 and `r: unbound variable` for the
			// same line with a literal subscript, which is the control. The
			// sentence is written here because this is the site that read
			// the expression, and checkNounset says nothing further about
			// `r` once it has been (#3574, #3125).
			r.diagf("%s\n", err)
			return "", false
		}
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
func (r *Runner) namerefAssignmentTarget(name, value string, form assignForm) (target string, write bool) {
	if !r.isNameref(name) {
		return name, true
	}
	target, aimed := r.namerefTarget(name)
	if aimed {
		return target, true
	}
	if !r.namerefTargetIsAName(value) {
		r.refuseNamerefAim(value, form)
		return "", false
	}
	r.setNameref(name, value)
	return name, false
}

// refuseNamerefAim reports a value that cannot aim a reference, and is the
// **assignment's** half of the check the declaration makes in
// [Runner.declareNameref]: a reference with nothing to point at is aimed by
// its first value, and a value that is no possible name aims it nowhere.
//
// Measured 2026-09-17 on bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, each over `declare -n q` and then
// one write:
//
//	q=/                  `` `/': not a valid identifier ``, 1, and the
//	                     *rest of the input line is given up*
//	read q <<< /         `read: `/': not a valid identifier``, 1, and the
//	                     rest of the line runs
//	declare q=/          `declare: `/': not a valid identifier``, 1, rest
//	                     of the line runs
//	q=a, q=a[0], q=a[@]  taken, and the reference is aimed there
//
// So the sentence is the one a bad *target* earns everywhere else, the
// builtin that made the write names itself where there was one, and the
// reference is left unaimed: `declare -p q` is `declare -n q` afterwards and
// the name never becomes the text. This shell aimed the reference at whatever
// it was handed, so `${!q}` answered `/` and every later write through it
// went to a parameter no shell can name.
//
// Giving up the rest of the line for the bare form and not for a builtin's is
// the split [assignForm] already draws for the readonly refusal, and it is
// the same observation: `declare -n t; t=/ ; echo after` never prints
// `after`, where the `read` and `declare` spellings both do. See
// selfNamerefAssignment, whose two lines these are.
//
// The wording is written out rather than taken from [Diagnostics] for
// namerefArrayLiteralTarget's reason: only a dialect that spells references
// reaches it, and of those only bash reaches it *here* — ksh93 refuses the
// aim at the declaration and ends the script, so it has no assignment left
// to make.
func (r *Runner) refuseNamerefAim(value string, form assignForm) {
	// Who made the write, which is what the sentence names. Two routes reach
	// here with no builtin running and neither was named: `(( r = 1 ))`,
	// where the *construct* names itself as it names its own arithmetic —
	// measured 2026-09-17, bash 5.3.20 writes ``((: `1': not a valid
	// identifier`` — and the descriptor a `{name}>` redirection stores, where
	// the command word the redirection belongs to speaks (#3491).
	speaker := r.inBuiltin
	if speaker == "" {
		speaker = r.fdVarSpeaker
	}
	switch {
	case speaker != "":
		r.diagf("%s: `%s': not a valid identifier\n", speaker, value)
	case r.arithCommand > 0:
		r.diagf("%s\n", r.diag().arithConstructFailure("((",
			fmt.Sprintf("`%s': not a valid identifier", value)))
	default:
		r.diagf("`%s': not a valid identifier\n", value)
	}
	r.status, r.assignFailed = 1, true
	if form == assignedAlone {
		r.abandonTheCommand()
	}
}

// namerefArrayLiteralTarget answers where an array literal assigned through a
// reference goes, and reports whether it goes anywhere at all.
//
// Measured 2026-09-17 on bash 5.3.20, script files under `env -i`, with
// `a=(Z Y)`:
//
//	declare -n v=a; v=(n1 n2)          a becomes (n1 n2)
//	declare -n w=a; w+=(app)           a becomes (Z Y app)
//	f(){ declare -n p=$1; p+=(x); }    the caller's array grows
//	declare -A M=([x]=1)
//	  declare -n n=M; n=([y]=2)        M becomes ([y]=2) — the target's
//	                                   own replace rule, not the reference's
//	declare -n u; u=(a b)              `warning: u: removing nameref
//	                                   attribute`, and u itself is the array
//	declare -n e=q[0]; e=(a b)         `` `q[0]': not a valid identifier ``
//	                                   and nothing is written
//
// Every one of those wrote nowhere here: the literal branches of
// [Runner.assign] read the name as itself, so an assignment through a
// reference silently reached no container at all. The scalar path already
// went through [Runner.namerefAssignmentTarget]; this is its literal half,
// and the three answers above are its three returns.
func (r *Runner) namerefArrayLiteralTarget(name string) (target string, write bool) {
	if !r.isNameref(name) {
		return name, true
	}
	aimed, is := r.namerefTarget(name)
	if !is {
		// Nothing to point at: the reference gives the attribute up and the
		// name takes the array itself.
		if w := r.diag().NamerefArrayLiteralDropsTheAttribute; w != "" {
			r.DiagnoseAsTheShellf("%s\n", Wording(w, "warning: %[1]s: removing nameref attribute", name))
		}
		r.unsetNameref(name)
		return name, true
	}
	if !isNameLike(aimed) {
		// Aimed at an element. A whole array cannot go into one, and the
		// shell says so rather than writing the first word into it.
		r.diagf("`%s': not a valid identifier\n", aimed)
		// Reported and carried on, at 1: measured, the next line runs and
		// `echo "after $?"` is `after 1` in bash 5.3.20.
		r.assignFailed = true
		r.status = 1
		return "", false
	}
	return aimed, true
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
// adopts says a value already standing under the name may aim the reference —
// false where *this declaration* created the cell, which is a binding nothing
// has written to whatever the name meant outside it. Measured 2026-09-17 on
// bash 5.3.20: `outer=good` and then `f(){ local -n outer; }` or `f(){
// declare -n outer; }` leave `declare -n outer` unaimed, where `f(){ declare
// -gn outer; }` — no new cell — aims at `good`, and `f(){ local x=good;
// local -n x; }` aims at it too.
func (r *Runner) declareNameref(builtin, name, target string, df declareFlags,
	hasValue, frozen, adopts bool, held declarationHeld, fresh bool,
) int {
	// A refusal this declaration **reports** leaves the name the way the
	// operand found it, letters and all — see declarationtakenback.go, where
	// the rows are. Through one door because there are eight of them and they
	// are the same answer: a `refuse(…)` that forgot the take-back would be a
	// letter left standing under one wording alone.
	refuse := func(wording string) int {
		code := r.refuseNameref(builtin, wording)
		r.takeTheDeclarationBack(name, held, fresh)
		return code
	}
	// The readonly refusal a **frozen reference** makes, in the one place its
	// order against the other two is measured. bash 5.3.20 puts the bad
	// target ahead of it and nothing else: `v=1; declare -rn r=v` then
	// `declare -n r=1x` is ``declare: `1x': invalid variable name for name
	// reference`` where `declare -n r=ok` is `declare: r: readonly
	// variable`. So it sits behind the target's own check and in front of
	// everything that changes the name.
	//
	// A frozen name that is **not** a reference refuses a `-n` declaration in
	// bash too — `readonly v=1; declare -n v=w` is `declare: v: readonly
	// variable` at 1 there and a silent 0 here — and that half is left alone
	// deliberately. It is not this change's claim, and refusing on it turns a
	// *second* defect into a diagnostic the reference shell does not write:
	// three of the panel's shells unset OPTARG when `getopts` runs out of
	// options, which takes a freeze on it away, and this shell keeps both.
	// See #3146. Every row below is a reference this shell made frozen
	// itself, so this shell's own record is what it consults.
	refuseFrozen := func() bool {
		if !frozen || !r.isNameref(name) {
			return false
		}
		r.refuseReadonly(name, assignedByDeclaration)
		return true
	}
	if !hasValue {
		if !r.isNameref(name) {
			// The valueless form over a name carrying an array, which both
			// shells refuse on the attribute alone and in the same words —
			// no axis, and no other refusal to race, because there is no
			// target here to be a bad name or a self reference. See
			// [NamerefArrayRefusal].
			if r.namerefArrayAttribute(name) {
				return refuse(Wording(r.diag().NamerefCannotBeAnArray,
					"%[1]s: reference variable cannot be an array", name))
			}
			if refuseFrozen() {
				return 1
			}
			// **A value the name already holds aims the reference.** The
			// letter on its own does not always leave a reference with
			// nothing to point at: measured 2026-09-17 on bash 5.3.20 and
			// ksh93u+ 2012-08-01, script files under `env -i`, `good=G;
			// r=good; typeset -n r` lists back as a reference to `good` in
			// both and `$r` reads `G`. Only a name nothing has set becomes
			// the unaimed reference below.
			//
			// Every value is adopted, not only the ones that read as names:
			// `a=(p q); t=a[1]; declare -n t` aims at the element and `$t`
			// is `q`. What is *not* a possible target is refused in the
			// declaration's own words — `e=""; declare -n e` and `b=/;
			// typeset -n b` both report the bad target at 1 and leave the
			// name the plain scalar it was, in bash and in ksh93 alike
			// (ksh93 ends the script over it, which refuseNameref already
			// asks).
			//
			// The self-reference refusals the valued form makes are
			// deliberately **not** asked here, and that is measured rather
			// than an omission: `declare -n s=s` is `nameref variable self
			// references not allowed` at 1 in bash 5.3.20, while `s=s;
			// declare -n s` is a silent 0 that lists as `declare -n s="s"`
			// and warns `circular name reference` at the first read. So the
			// value route goes through the target's validity and nothing
			// else.
			if held, set := r.getVar(name); set && adopts {
				if !r.namerefTargetIsAName(held) {
					return refuse(Wording(r.diag().NamerefBadTarget,
						"%[1]s: invalid variable name for name reference", held))
				}
				r.namerefEmptiesTheCell(name, df)
				r.setNameref(name, held)
				return 0
			}
			r.namerefEmptiesTheCell(name, df)
			r.setNameref(name, "")
			return 0
		}
		// A second `typeset -n r` over a reference that is already aimed
		// leaves it aimed, measured in both: the letter says what the name
		// *is*, and saying it twice says nothing new — unless the reference
		// is frozen, where saying it again is refused.
		if refuseFrozen() {
			return 1
		}
		r.namerefKeepsOnlyThisLinesFolding(name, df)
		return 0
	}
	d := r.diag()
	// Resolved once, before anything is reported, because the answer decides
	// which of three refusals speaks for this line — and asked only where the
	// name really carries an array, so an ordinary `typeset -n r=v` over a
	// scalar never reaches an unanswered field. See [NamerefArrayRefusal].
	arrayed := r.namerefArrayAttribute(name)
	var shape NamerefArrayRefusal
	if arrayed {
		shape = r.namerefArrayRefusal()
		if r.unspecified {
			return r.status
		}
		if shape == NamerefArrayCheckedFirstOnTheContents && r.namerefArrayContents(name) {
			return refuse(Wording(d.NamerefCannotBeAnArray,
				"%[1]s: reference variable cannot be an array", name))
		}
	}
	// The one shape the late answer still reaches: a bare attribute under the
	// contents reading is *not* an array, so the late check must not fire on
	// it either.
	refusedLate := arrayed && shape == NamerefArrayCheckedLastOnTheAttribute
	if !r.namerefTargetIsAName(target) {
		return refuse(Wording(d.NamerefBadTarget,
			"%[1]s: invalid variable name for name reference", target))
	}
	aim, aimIsAName := r.namerefAim(target, df)
	if !aimIsAName {
		// The letter shaped the name into something that is not one, which
		// is refused **in silence** — see namerefAim. Behind the check above
		// and not folded into it, because the two are worded differently and
		// the difference is measured: the word as written gets the sentence,
		// what the letters made of it gets nothing at all.
		//
		// And it leaves behind what the reported refusals do not: the letters
		// stand on a binding that was already there or that this call made
		// local, and only a name brought into being at the top level goes.
		// See takeBackTheNameItMade.
		r.takeBackTheNameItMade(name, held, fresh)
		return 1
	}
	// And what the letters made of it is what a dialect that settles the
	// target here settles: the fold shapes the word and the settling reads
	// the shaped one. The two never meet in a real dialect — the column that
	// settles refuses an `n` letter in company at all, see
	// Semantics.NamerefLetterStandsAlone — so the order is the one the two
	// rules read in rather than a measurement.
	if settled, ok := r.namerefTargetSettledHere(aim); ok {
		aim = settled
	} else if r.unspecified || r.badSubscript {
		// The subscript would not evaluate, which in the column that
		// evaluates it here is this declaration's failure rather than a
		// later read's. badSubscriptToADeclaration has already reported it
		// and decided how much of the input goes with it.
		return r.status
	}
	if refuseFrozen() {
		return 1
	}
	if target == name || aim == name {
		// A reference aimed straight at itself, and **where it is written
		// decides what happens to it**.
		//
		// At the top level both shells refuse — measured, `typeset -n r=r`
		// is `nameref variable self references not allowed` in bash 5.3.20
		// and `invalid self reference` in ksh93u+ — so that much is the
		// core's answer and no axis is asked. There is nowhere for the name
		// to refer *out* to, which is the whole of why it is refused.
		//
		// Inside a function there is somewhere, and `local -n r=r` is the
		// ordinary spelling of "give me a handle on the outer variable of
		// the same name". bash takes it, says so twice, and resolves it
		// outward; ksh93 refuses it in the same words it uses at the top
		// level and ends the script. That is exactly the split
		// [Semantics.NamerefCycleIsRefused] already records for the pair
		// `typeset -n a=b; typeset -n b=a` — refuse at the declaration, or
		// make it and complain at the read — so it is asked here rather than
		// given a second field of its own (#3048).
		if len(r.scopes) == 0 ||
			r.ask(r.sem().NamerefCycleIsRefused, "a name reference that reaches itself") {
			return refuse(Wording(d.NamerefSelfReference,
				"%[1]s: invalid self reference", name))
		}
		if r.unspecified {
			return r.status
		}
		// Between the two halves of the warning, and that is measured rather
		// than convenient: `f(){ typeset r=(a b); typeset -n r=r; }` in bash
		// 5.3.20 writes the *builtin's* `warning: r: circular name
		// reference` and then the array refusal at 1, and never the shell's
		// second copy. So the array check sits inside the warning rather
		// than before or after it.
		r.warnAboutASelfReferenceOnTheBuiltin(builtin, name)
		if refusedLate {
			return refuse(Wording(d.NamerefCannotBeAnArray,
				"%[1]s: reference variable cannot be an array", name))
		}
		r.warnAboutACycle(name)
		// The cell is emptied here too, and it is not the one the reference
		// reads: `local -n r=r` refers *out*, to the copy the oldest scope
		// that shadowed the name is holding, which is untouched by this
		// (#3048). What goes is the value standing in the live table under
		// the name this binding just took over.
		r.namerefEmptiesTheCell(name, df)
		r.setNameref(name, aim)
		return 0
	}
	if r.namerefSelfReference(name, aim) {
		if r.ask(r.sem().NamerefCycleIsRefused, "a name reference that reaches itself") {
			return refuse(Wording(d.NamerefSelfReference,
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
	if refusedLate {
		return refuse(Wording(d.NamerefCannotBeAnArray,
			"%[1]s: reference variable cannot be an array", name))
	}
	r.namerefEmptiesTheCell(name, df)
	r.setNameref(name, aim)
	return 0
}

// namerefAim is what the letters on a `-n` declaration make of the name the
// reference is aimed at.
//
// **A reference's own cell holds the target's name**, so a letter that shapes
// a value shapes that name — the same letter, doing the same thing, to the
// one string the cell has. Measured 2026-09-18 on bash 5.3.20, `env -i` with
// a scratch HOME, from a file, with `v=1` and `V=BIGV`:
//
//	declare -nu r=v     declare -nu r="V"     and `$r` reads BIGV
//	declare -nl r=V     declare -nl r="v"     and `$r` reads 1
//	declare -nu r=a[1]  declare -nu r="A[1]"  the element of `A`, not of `a`
//	declare -ni r=v     refused at 1, in silence, and nothing is made
//
// The last row is the same rule reaching its end: the integer letter makes
// what the cell holds a **number**, and a number is not a name. So the
// declaration has nowhere to aim and is refused — with no sentence at all,
// which is measured and is the one thing here that is not simply the letter
// doing its job. The word as *written* still gets the ordinary complaint:
// `declare -ni r=1` is “declare: `1': invalid variable name for name
// reference“ in that shell, because `1` fails the check on the written word
// before this is reached, where `declare -ni r=v` passes it and fails here.
//
// The fold runs **in front of** the self-reference check and the written word
// is still tested too, which is two measurements rather than one: `declare
// -nl r=R` is `nameref variable self references not allowed` — the fold is
// what made it one — and `declare -nu r=r` is the same refusal, where the
// fold alone would have aimed it at `R` and let it through. `declare -nu r=R`
// is taken at 0, which is the control that says the comparison is not merely
// case-blind.
//
// Core rather than an axis for the reason the rest of interp/nameref.go is:
// bash is the only column that reads the two letters together at all. The
// other shell that spells a reference refuses the pair at the option parser
// and never arrives — see Semantics.NamerefLetterStandsAlone.
//
// Read off the declaration's own letters rather than off the name's, because
// the two answer different questions: an attribute the name was already
// carrying is discarded by this very declaration and shapes nothing. See
// namerefEmptiesTheCell.
func (r *Runner) namerefAim(target string, df declareFlags) (string, bool) {
	if df.integer && !df.integerOff {
		return "", false
	}
	switch {
	case df.lower:
		return r.caseChanged(target, unicode.ToLower), true
	case df.upper:
		return r.caseChanged(target, unicode.ToUpper), true
	}
	return target, true
}

// namerefTargetSettledHere is the target a reference records where the
// dialect settles it **at the declaration** rather than at every read.
//
// Two shapes and one rule. A subscript is arithmetic, so it is evaluated now
// and the number is what the reference holds — which is why a later change to
// the variable the subscript named moves nothing. And a target that is itself
// a reference is followed to the end of its chain now, so re-aiming the
// middle link afterwards leaves this one where it was. See
// Semantics.NamerefTargetResolvedWhenAimed, where the panel is.
//
// A **keyed table's** subscript is a key rather than an expression and is
// left as written, in that column as much as in the other:
// `typeset -A m=([k]=1); typeset -n r=m[k]` lists `m[k]` in both. The same
// question a declaration's own subscripted operand asks, answered through the
// same function, so the two cannot come to read one set of brackets two ways.
//
// A **negative** subscript is resolved too, against the array as it stands:
// `a=(x y z); typeset -n r=a[-1]` records `a[2]`. Which is the position the
// element store would have used, so it is the store's own arithmetic and not
// a second copy.
//
// The second result is false where nothing was settled — either because the
// dialect does not settle here, or because the subscript would not evaluate,
// which in this column is the declaration's own failure and is reported as
// one.
func (r *Runner) namerefTargetSettledHere(target string) (string, bool) {
	base, sub, element := r.indirectElement(target)
	chained := !element && r.isNameref(target)
	if !element && !chained {
		// A plain name is the same declaration under either answer, so the
		// axis is not put to a dialect that only ever writes one.
		return "", false
	}
	if !r.ask(r.sem().NamerefTargetResolvedWhenAimed,
		"a name reference's target being settled at the declaration") {
		return "", false
	}
	if chained {
		end, _ := r.namerefTarget(target)
		return end, true
	}
	if _, _, isKey := r.subscriptedOperandKey(base, sub, r.assocDeclared(base)); isKey {
		// A key, not an expression — and not evaluated in either column.
		return target, !r.unspecified
	}
	if r.unspecified {
		return "", false
	}
	// The same put-aside declareElement makes for the same complaint: the
	// sentence is the *language's* — `typeset: @: arithmetic syntax error` —
	// so the builtin leaves the location and is recorded for the one reader
	// that still names it. Without it the subscript this declaration cannot
	// evaluate was reported with an empty name where the element store one
	// function over reports the builtin's.
	outer := r.inBuiltin
	r.inBuiltin, r.declarationSpeaker = "", outer
	defer func() { r.inBuiltin, r.declarationSpeaker = outer, "" }()
	idx, err := r.subscriptValueOfReference(sub)
	if err != nil {
		r.badSubscriptToADeclaration(sub, err)
		return "", false
	}
	if idx < 0 {
		// Counted forwards from the end the array has *now*, which is the
		// whole of what settling it here means for a negative.
		if pos, in := r.elemPos(r.Arrays[base], idx); in {
			idx = pos + r.arrayBase()
		}
	}
	return base + "[" + itoa(idx) + "]", true
}

// namerefEmptiesTheCell is what a `-n` declaration does to whatever the name
// was holding: the name becomes a **reference**, and a reference has no value
// under it. Both shells discard it, and the discard is not a fact about scope
// — `r=OUTER; typeset -n r=v; unset -n r` reads the name as unset at the top
// level, inside a function, and through `-g` alike, in bash 5.3.20 and in
// ksh93u+ 2012 (#3084). The reference hides the discard everywhere else,
// because a read of the name goes through it; `unset -n` is what takes the
// reference away and asks the cell underneath what it holds.
//
// The value is the only thing that goes of what the *name* holds. Most
// attributes stay, because the same declaration applied them a few lines
// earlier and `unset`'s clearing would take them straight back off; and the
// reference itself is recorded after this, in the nameref table rather than in
// a parameter one.
//
// **The three attributes that fold a value go with the value**, and that is
// measured rather than derived from the sentence above. With `tgt=T`,
// `typeset -i k; typeset -n k=tgt` lists as `declare -n k="tgt"` in bash
// 5.3.20 with no `i` left in it, and so do the `-l` and `-u` spellings, while
// `typeset -x k` keeps its `x` — `declare -nx k="tgt"`. The behavior agrees
// with the listing in both shells: `typeset -i k; typeset -n k=tgt; k=3+4`
// leaves `tgt` holding the text `3+4` in bash 5.3.20 and ksh93u+ alike, so
// the attribute is gone rather than merely unlisted. A reference holds no
// value, and an attribute that exists to fold one has nothing to fold.
//
// Only what the *declaration* discards. An attribute that arrives afterwards
// is the reference's own and is kept: `typeset -n y; typeset -i y` lists as
// `declare -in y` there.
//
// **And only what it found already standing.** A letter written on the `-n`
// line itself is the reference's own from the start and survives: measured
// 2026-09-18 on bash 5.3.20, `v=1; declare -nu r=v` lists as `declare -nu
// r="V"` where `declare -u r; declare -n r=v` lists as `declare -n r="v"` —
// the same letter, kept in one and discarded in the other, and the only
// difference is which line wrote it. That is why df is read here rather than
// the maps: by the time this runs applyAttributes has already put this
// line's letters in them, so the maps can no longer say which line they came
// from. See namerefAim for what the kept letter then does to the name the
// reference is aimed at.
//
// The **array attribute goes with it**, and that is measured rather than
// assumed. Almost no array reaches here at all — a `-n` declaration over one
// is refused, see [NamerefArrayRefusal] — but one shape does: ksh93 takes
// `typeset -a r; typeset -n r=v`, because a bare indexed attribute is not yet
// an array there. Its own listing then writes `typeset -n r=v` with no `-a`
// left in it, and `unset -n r` afterwards leaves `typeset -p r` with nothing
// to print and `${r-GONE}` reading GONE (#3103). So the attribute is
// discarded with the value rather than surviving under the reference, which
// is also the only reading that keeps `${#r[@]}` from answering for a name
// that reads as unset.
//
// Nothing here runs on a refusal. A `-n` declaration the shell will not make
// leaves the name exactly as it found it — measured, `r=OUTER` followed by
// `typeset -n r=NOT_A_NAME` or by a refused self reference reports 1 and
// leaves `r` holding OUTER in both shells — so every caller is on a path that
// has already decided the reference is going to be made.
func (r *Runner) namerefEmptiesTheCell(name string, df declareFlags) {
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	r.namerefKeepsOnlyThisLinesFolding(name, df)
	// hideVar rather than a bare delete: a name that came from the
	// environment is not in Vars to begin with, so deleting nothing would
	// leave the inherited value answering every read of the cell the
	// reference has just taken over.
	r.hideVar(name)
}

// namerefKeepsOnlyThisLinesFolding takes the three value-shaping attributes
// off a name a `-n` declaration has just made a reference of, except the ones
// that declaration wrote itself.
//
// Its own function because a *second* `typeset -n r` over a reference that is
// already aimed changes nothing else at all and still does this: measured
// 2026-09-18 on bash 5.3.20, `declare -nu r=v` lists as `declare -nu r="V"`
// and a bare `declare -n r` after it lists as `declare -n r="V"` — the letter
// gone, the name it had already folded left standing. So the discard belongs
// to the `n` letter rather than to the emptying of the cell, and the two
// paths through the declaration share it.
func (r *Runner) namerefKeepsOnlyThisLinesFolding(name string, df declareFlags) {
	if !df.integer {
		delete(r.integer, name)
	}
	if !df.lower {
		delete(r.lowered, name)
	}
	if !df.upper {
		delete(r.uppered, name)
	}
}

// namerefAttributeRemoved is `typeset +n r`: the reference goes and **the name
// it pointed at stays behind as the value**.
//
// Measured 2026-09-17 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, with `v=bar; typeset -n foo=v`:
//
//	                          bash 5.3.20            ksh93u+ 2012-08-01
//	typeset +n foo            declare -- foo="v"     foo=v
//	then `echo "$foo"`        v                      v
//	typeset -n g; typeset +n g   declare -- g        the name, with no value
//	h=1; typeset +n h         declare -- h="1"       h=1
//
// Unanimous across the two shells that spell a reference, so no axis is
// asked. The value is the **target's name** and not what the target holds,
// which is the row that says the reference was not followed: `$foo` reads `v`
// afterwards and not `bar`.
//
// Here the letter did nothing at all: `f.nameref = !f.remove` recorded a plus
// as "no `n` letter on this line", so `typeset +n foo` left the reference
// standing and every later read still went through it. That is a silent
// no-op on a declaration a script wrote on purpose — and worse than nothing,
// because the redirect this is asked in front of would then have carried the
// rest of the operand off to the target.
//
// It reports whether this operand is **finished**, which is not the same as
// whether it did anything. A `+n` on a name that is not a reference falls
// through to the ordinary declaration, which is what leaves `h=1; typeset +n
// h` the plain `h=1` both shells list — and so does a reference with nothing
// to point at, because there is no value to leave behind and the name is then
// an ordinary valueless declaration: `typeset -n g; typeset +n g` lists as
// `declare -- g` there and as a bare `g` in ksh93, which is what
// declareEmpty already writes.
func (r *Runner) namerefAttributeRemoved(name string) bool {
	if !r.isNameref(name) {
		return false
	}
	if r.refuseReadonly(name, removedAttribute) {
		// A **frozen reference** may not be taken apart: measured the same
		// day, `w=2; declare -rn k=w; declare +n k` is `declare: k: readonly
		// variable` at 1 in bash 5.3.20 and `typeset: k: is read only` in
		// ksh93u+, and `k` is still the frozen reference afterwards. It is
		// the freeze the *reference* carries, which is the half `unset -n`
		// already asks and the opposite of what a write through one asks.
		//
		// removedAttribute is the form this is: a declaration asking a name
		// to give an attribute up, whose sentence is the declaration's and
		// which gives up nothing of the enclosing line.
		return true
	}
	target, aimed := r.namerefTarget(name)
	r.unsetNameref(name)
	if !aimed {
		// Nothing to leave behind, so the operand falls through to the
		// ordinary declaration it now is. The attribute is the claim here;
		// **what a listing then says about a name holding nothing is not**,
		// and is not yet right: `typeset -n g; typeset +n g; typeset -p g`
		// is `declare -- g` in bash 5.3.20 and a bare `g` in ksh93u+ and is
		// `g: not found` here, because the cell the reference took over is
		// still hidden and neither this fall-through nor declareEmpty brings
		// it back. The name reads unset in all three — `${g-UNSET}` is
		// UNSET everywhere — so what differs is the listing alone.
		return false
	}
	// The target's *name*, which is what the reference was holding — not
	// what the target holds, which is the row that says the reference was
	// not followed: `$foo` reads `v` afterwards and not `bar`.
	r.setVar(name, target)
	return true
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

// warnAboutASelfReferenceOnTheBuiltin is the first half of what a
// *declaration* of `local -n r=r` says in the dialect that takes one, which
// says it twice.
//
// Measured 2026-09-15 on bash 5.3.20, `f() { local -n r=r; }; f`:
//
//	f.sh: line 1: local: warning: r: circular name reference
//	f.sh: line 1: warning: r: circular name reference
//
// This is the first, carrying the builtin's name; warnAboutACycle is the
// second, spoken as the shell. The same pair comes out of `declare -n` and
// `typeset -n` with their own word in front. Two sentences and not one
// because that is what the shell writes; a single warning left the line count
// short wherever this shape is scored.
//
// The two are written at the call site rather than joined in one helper
// because a refusal can land **between** them: measured, `f(){ local r=(a b);
// local -n r=r; }` in bash writes this half, then `r: reference variable
// cannot be an array`, and never the second half. See declareNameref.
//
// Nothing at all in the dialect with no wording for it, which is the one that
// refuses this declaration outright and never reaches here.
func (r *Runner) warnAboutASelfReferenceOnTheBuiltin(builtin, name string) {
	w := r.diag().NamerefCircularWarning
	if w == "" {
		return
	}
	r.diagf("%s: %s\n", builtin, Wording(w, "warning: %[1]s: circular name reference", name))
}

// warnAboutNamerefDepth is what a *write* through a self reference says,
// which is a different sentence from the read's.
//
// Measured on bash 5.3.20 over the same function: reading `$r` writes
// `warning: r: circular name reference` and `r=SET` writes `warning: r:
// maximum nameref depth (8) exceeded`, both as the shell. The 8 is bash's own
// bound and travels with the wording rather than with namerefDepth here,
// because it is a number this shell reports rather than one it enforces — the
// resolution below is a single step to the outer cell and never a walk.
func (r *Runner) warnAboutNamerefDepth(name string) {
	if w := r.diag().NamerefDepthWarning; w != "" {
		r.DiagnoseAsTheShellf("%s\n", Wording(w, "warning: %[1]s: maximum nameref depth exceeded", name))
	}
}

// A self reference resolves **outward**, to the cell the name had before any
// function took it over.
//
// Measured 2026-09-15 on bash 5.3.20, and the discriminator is a caller that
// has a local of the same name in between:
//
//	r=L0
//	h() { local r=L1; g; echo "h=[$r]"; }
//	g() { local r=L2; f; echo "g=[$r]"; }
//	f() { local -n r=r; echo "f=[$r]"; r=SET; }
//	h; echo "top=[$r]"
//
// writes `f=[L0]`, `g=[L2]`, `h=[L1]`, `top=[SET]`. So it is not "the
// caller's" cell — two callers' locals are stepped straight over — it is the
// global one, and both the read and the write land there.
//
// In this engine there is one table and a stack of saved outer values, so the
// global cell is the copy held by the **oldest** scope that shadowed the name
// — the value the last return will put back — which is the same walk
// setGlobalVar takes. What is deliberately different is the answer when no
// scope shadowed it at all: there the reference *is* the global cell and the
// loop closes on itself, which bash reports as a circular read of an empty
// value. `f() { declare -gn r=r; echo "[$r]"; r=SET; }` over an outer `r`
// writes `[]` and leaves the outer value alone, where the same line without
// `-g` writes `[OUTER]` and sets it.

// selfNameref reports whether a name is a reference aimed at its own name,
// which is the one shape that resolves outward rather than to another name.
func (r *Runner) selfNameref(name string) bool {
	target, ok := r.nameref[name]
	return ok && target == name
}

// selfNamerefValue is the read: what the global cell of `name` holds, and
// whether it holds anything. The second result is false where nothing
// shadowed the name, which is the closed loop above and not an unset cell —
// the two answer alike here, an empty value, and no caller has to tell them
// apart.
func (r *Runner) selfNamerefValue(name string) (string, bool) {
	for _, sc := range r.scopes {
		v, saved := sc.saved[name]
		if !saved {
			continue
		}
		if sc.removedBefore[name] || !sc.existed[name] {
			return "", false
		}
		return v, true
	}
	return "", false
}

// selfNamerefStore is the write, and it reports whether the value landed
// anywhere: a reference that is itself the global cell has nothing outside it
// to write to, and bash says `circular name reference` there rather than the
// depth it says when the write does land.
func (r *Runner) selfNamerefStore(name, value string) bool {
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		sc.saved[name] = value
		sc.existed[name] = true
		if sc.removedBefore != nil {
			sc.removedBefore[name] = false
		}
		return true
	}
	return false
}

// selfNamerefAssignment is setVarAs's one line for the shape above.
//
// A write that lands nowhere is a **failed assignment** and not merely a
// quiet one: measured on bash 5.3.20, `f() { declare -gn r=r; r=SET; echo
// NOPE; }; f; echo AFTER` writes the warning, reports 1, never writes `NOPE`,
// and runs `AFTER` — which is the shape a readonly reassignment already
// takes here, the command list given up and the caller carrying on. See
// refuseReadonly, whose two lines these are.
func (r *Runner) selfNamerefAssignment(name, value string, form assignForm) {
	if r.selfNamerefStore(name, value) {
		r.warnAboutNamerefDepth(name)
		return
	}
	r.warnAboutACycle(name)
	r.status, r.assignFailed = 1, true
	if !form.declaresRatherThanAssigns() {
		r.abandonTheCommand()
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
