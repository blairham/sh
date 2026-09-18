// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a valueless local declaration finds in the cell it makes, where the
// script has asked for the *enclosing* binding instead of for nothing.
//
// A fresh binding starting empty is what every shell in the panel with a local
// scope does, and localattributes.go and freshcell.go are the two halves of
// making that true here — the value, the compound tables and every attribute
// are put aside at the shadow so that the cell holds nothing. One shell lets a
// script ask for the other answer, wholesale under a `shopt` name and one
// declaration at a time under a letter, and this file is that request.
//
// It is a **restore** rather than a shadow that never happened, and the
// difference is what makes it small: the scope has already saved everything —
// the scalar, both compound tables, the declared-only mark and every
// attribute — precisely so it can give them back on return, so inheriting is
// handing the same record to the cell that was just made. Nothing here reaches
// past the innermost scope, so the value is the one the *caller* could see and
// not the global: measured, a function called from one that holds `v=MID`
// inherits `MID` over a global `OUT`.
//
// Measured 2026-09-17 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// from a script file, over a caller's `v=OUTER`, `declare -i n=5`,
// `declare -a a=(x y)`, `declare -A m=([k]=w)`, `declare -x e=EXP`,
// `declare -u up=abc` and `declare -l lo=ABC`:
//
//	f(){ local v; local n; local a; local m; declare -p v n a m; }
//
//	                    inheriting off          inheriting on
//	declare -p v        declare -- v            declare -- v="OUTER"
//	declare -p n        declare -- n            declare -i n="5"
//	declare -p a        declare -- a            declare -a a=([0]="x" [1]="y")
//	declare -p m        declare -- m            declare -A m=([k]="w" )
//	declare -p e        declare -- e            declare -x e="EXP"
//	declare -p up       declare -- up           declare -u up="ABC"
//	declare -p lo       declare -- lo           declare -l lo="abc"
//
// So the attributes travel with the value, which is the half a restore of the
// scalar alone would miss, and the export attribute travels far enough to
// reach a child: `env` in that function lists `e=EXP`.
//
// Six measurements bound it, and each is a line below.
//
//   - **A value on the declaration wins.** `local -I v=NEW` is
//     `declare -- v="NEW"` with no attribute inherited either, so this is
//     asked only of a declaration that brought no value — which is why it
//     lives in declareEmpty and nowhere else.
//
//   - **The declaration's own letters are kept and the inherited ones are
//     added to them.** `local -a n` over an `-i n=5` is
//     `declare -ai n=([0]="5")`, and `local -i v` over a plain `v=OUTER` is
//     `declare -i v="OUTER"`. So this stands after the letters are applied
//     and merges rather than replacing.
//
//   - **The inherited compound is what a container letter on the same line
//     then has to convert.** `declare -a a=(x y); f(){ local -A a; }` is
//     `cannot convert indexed to associative array` with the option on and a
//     silent `declare -A a` with it off — same line, two answers, and the
//     difference is whether there was anything in the cell to convert. That
//     is what dropTheOuterCompound stands down for, and it is the other half
//     of the row above: both say the cell is not being built empty.
//
//   - **A name the enclosing scope does not hold is inherited as nothing**,
//     not as the empty string: `local zzz` is `declare -- zzz` at 0 with the
//     option on, which is the same cell a shell that inherits nothing makes.
//     That is what restoreTheOuterBinding's false is for.
//
//   - **The name-reference attribute does not travel.** Over a caller's
//     `declare -n nr=v`, `local nr` is `declare -- nr="v"` — the *text* the
//     reference held is inherited and the reference is not. A local that
//     inherited the reference would aim the callee's own name at whatever
//     the caller was pointing at, which is the leak interp/nameref.go exists
//     to prevent, and it is not what was measured.
//
//   - **Only a fresh cell.** A second declaration of a name its own scope has
//     already made has no enclosing binding in front of it — the value it
//     would take is the local the first declaration wrote — and the measured
//     answer keeps that value: `local v=S; local v` is `declare -- v="S"`,
//     with the option on as without it. The `fresh` the shadow already
//     answers is what says so, and it is the same gate every other question
//     in declareEmpty hangs on.

// LocalInheritsTheOuterValue reports whether a valueless local declaration
// takes the value and attributes of the name at the enclosing scope instead
// of starting empty — bash's `localvar_inherit`, which is off there by
// default and is the only name any shell in the panel has for the question.
//
// The positive direction, because it names a capability a script turns *on*:
// a Runner that was never told makes the fresh binding every shell with a
// local scope makes, which is what the zero value already did.
//
// The per-declaration spelling of the same request is the `I` letter, read
// through declareFlags and joined with this at the one place both are asked.
// They are one request with one answer, measured: `local -I v` in a shell
// with the option off and `local v` in a shell with it on produce the same
// binding, down to the attributes.
func (r *Runner) LocalInheritsTheOuterValue() bool { return r.localInherits }

// SetLocalInheritsTheOuterValue moves it.
func (r *Runner) SetLocalInheritsTheOuterValue(on bool) { r.localInherits = on }

// declarationInherits joins the two spellings of the one request: the letter
// this declaration wrote, and the shell-wide switch.
//
// One helper rather than the expression written out, because every site that
// asks has to ask the same question — a place reading only the switch would
// answer no for `local -I` in a shell that has never set the name, which is
// exactly the shape the letter exists for.
func (r *Runner) declarationInherits(f declareFlags) bool {
	return f.inherit || r.LocalInheritsTheOuterValue()
}

// restoreTheOuterBinding gives the cell this declaration just made whatever
// the innermost scope put aside for the name, reporting whether there was
// anything to give.
//
// False says the enclosing scope held nothing under this name, which is not
// the same as holding an empty value: the caller then makes the ordinary
// fresh binding, so `local zzz` over an unset `zzz` is unset either way.
func (r *Runner) restoreTheOuterBinding(name string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	had := false
	value, held := sc.saved[name], sc.existed[name]
	// A name the enclosing scope held as a **reference** hands over the name
	// it pointed at, as text, and not the reference. Measured 2026-09-17:
	// over a caller's `declare -n nr=v`, `local nr` is `declare -- nr="v"`.
	// A local that inherited the reference would aim the callee's own name
	// at whatever the caller was pointing at, which is the leak
	// interp/nameref.go exists to prevent — and it is not what bash does.
	//
	// Read as the scalar rather than beside it, because a reference's text
	// is not in the scalar table: the name reads as removed there, so a
	// branch of its own would have had to take that record back as well.
	if saved, ok := sc.savedAttrs[name]; ok && saved.isNameref && !held {
		value, held = saved.nameref, true
	}
	// The scalar, and its removed-by-`unset` bit with it: a name the script
	// had taken away is inherited as taken away rather than as present and
	// empty, which is what the scope recorded for the return.
	if held {
		r.Vars[name] = value
		setBool(&r.removed, name, false)
		had = true
	} else if sc.removedBefore != nil {
		setBool(&r.removed, name, sc.removedBefore[name])
	}
	// The two compound tables, which freshcell.go has just emptied. Copied
	// back, because the scope's copy has to stay the scope's: it is what the
	// caller's name gets on return, and a table handed over by reference
	// would be the one this call then writes into.
	if sc.arrayExisted[name] {
		if r.Arrays == nil {
			r.Arrays = map[string]Array{}
		}
		r.Arrays[name] = sc.savedArrays[name].clone()
		had = true
	}
	if sc.assocExisted[name] {
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = sc.savedAssoc[name].clone()
		had = true
	}
	if sc.declaredOnlyBefore != nil {
		setBool(&r.declaredOnlyCompound, name, sc.declaredOnlyBefore[name])
	}
	// And the attributes, added to the ones this declaration's own letters
	// have already written rather than replacing them — which is measured:
	// `local -a n` over an `-i n=5` carries both letters.
	if saved, ok := sc.savedAttrs[name]; ok && r.inheritAttributes(name, saved) {
		had = true
	}
	// The export attribute is *not* restored here, and that is measured
	// rather than forgotten: the dialect that has this option is one whose
	// local already carries the attribute of the name it shadows
	// (Semantics.LocalInheritsTheExportAttribute), so the letter is on the
	// binding before this runs and a child is told the value. A dialect
	// answering that the other way and growing this option would need a line
	// here; none does. See localExportAttribute.
	return had
}

// inheritAttributes adds the attributes a scope put aside back onto a name,
// leaving the ones this declaration's letters already wrote in place.
//
// Additive, which is the whole difference between it and restoreAttributes
// next door: that one is the *return*, where the cell is going away and the
// outer name has to carry exactly what it carried, and this one is a cell
// being built out of two sources. So nothing here takes an attribute off.
//
// The name reference is deliberately not among them — see the file comment.
func (r *Runner) inheritAttributes(name string, a nameAttributes) bool {
	had := false
	for _, at := range []struct {
		table *map[string]bool
		on    bool
	}{
		{&r.integer, a.integer},
		{&r.lowered, a.lower},
		{&r.uppered, a.upper},
		{&r.unique, a.unique},
		{&r.traced, a.traced},
		{&r.hidden, a.hidden},
	} {
		if at.on {
			setBool(at.table, name, true)
			had = true
		}
	}
	if a.baseSet {
		setInt(&r.integerBase, name, a.base, true)
		had = true
	}
	if a.isFloat {
		setInt(&r.floatPrecision, name, a.precision, true)
		setBool(&r.floatExponent, name, a.floatExponent)
		had = true
	}
	if a.hasWidth {
		if r.fieldWidth == nil {
			r.fieldWidth = map[string]fieldWidth{}
		}
		r.fieldWidth[name] = a.width
		had = true
	}
	return had
}
