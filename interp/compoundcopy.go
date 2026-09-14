// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"sort"
)

// Copying a compound variable is not the spelling it looks like.
//
// `d=$c` renders the tree as *text* and leaves a scalar — measured on ksh93u+
// 2012-08-01, 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch HOME:
//
//	c=(a=1 b=2); d=$c; typeset -p d     d=$'(\n\ta=1\n\tb=2\n)'
//	c=(a=1 b=2); d=$c; ${d.a}           empty
//
// The copy is a bare **name** standing where the value goes, and the two
// spellings that read it that way are a `-C` declaration and an assignment
// whose target is already a compound:
//
//	written                                typeset -p d
//	c=(a=1 b=2); typeset -C d=c            typeset -C d=(a=1;b=2)
//	c=(a=1 b=2); compound d=c              typeset -C d=(a=1;b=2)
//	c=(a=1 b=2); d=(a=9); d=c              typeset -C d=(a=1;b=2)
//	c=(a=1 b=2); d=c                       d=c        the target was not one
//	c=(a=1); typeset -C d=c; d.a=9         c keeps 1, d answers 9
//
// **The right-hand name decides the kind**, which is what makes this a copy of
// a variable rather than of a compound:
//
//	x=5; typeset -C d=x                    d=5             a plain scalar
//	a=(x y); typeset -C d=a                typeset -a d=(x y)
//	typeset -A h=([k]=v); typeset -C d=h   typeset -A d=([k]=v)
//	typeset -C d=nosuch                    nothing at all; `[[ -v d ]]` false
//	typeset -C d="not a name"              typeset -C d=()
//	c=(a=(p=1)); typeset -C d=c.a          typeset -C d=(p=1)
//
// The two spellings differ in one place and it is measured rather than
// reasoned: the `-C` declaration reads *any* kind out of the name, where the
// bare assignment reads only a compound and otherwise stores the text it was
// given. `x=5; d=(a=9); d=x` is `d=x` there, and `a=(p q); d=(a=9); d=a` is
// `d=a` — so a name that is not a compound is a value on that path and a
// source on the other.
//
// **Attributes travel with a member and not with the root.** `typeset -i x=7;
// typeset -C d=x` is `d=7` with no integer word, and `typeset -u x=ab;
// typeset -C d=x` is `d=AB` — the folded value, not the letter. A member's
// letters do travel, because they are part of the compound's value rather
// than of the name holding it: `c=(typeset -i n=5); typeset -C d=c; typeset
// -p d.n` is `typeset -i d.n=5`. That is the same split `typeset -m` records
// in interp/declaremove.go, reached from the other side.

// heldValue is the whole of what one name holds: its kind, its value and —
// for a member — the attributes that shape it.
//
// A snapshot rather than a pair of names, because a copy has to survive the
// target being cleared and the two may be the same name: `c=(a=1); typeset -C
// c=c` is `typeset -C c=()` in the shell, which is the target emptied and
// then copied from what is left of it.
type heldValue struct {
	// suffix is the member path below the root, `.a` and `.b.y` and so on,
	// and empty for the root itself.
	suffix   string
	compound bool
	arr      Array
	isArr    bool
	assoc    AssocArray
	isAssoc  bool
	scalar   string
	isScalar bool
	attrs    nameAttributes
	// members is filled for the root of a compound alone, and holds every
	// name stored under it at any depth.
	members []heldValue
}

// copySourceName reports whether a value may be read as the name of a variable
// to copy from.
//
// A name as the dialect spells one, the dotted form included — `typeset -C
// d=c.a` copies the member. Anything else is not a source: `typeset -C
// d="not a name"` and `typeset -C d=` both leave an empty compound standing,
// which is what the declaration would have made without a value at all.
func (r *Runner) copySourceName(value string) bool {
	return isPlainName(value) || r.dottedName(value)
}

// snapshotVariable reads everything stored under a name, and reports whether
// the name held anything at all.
//
// An unset source is the one answer that is not a value: `typeset -C
// d=nosuch` leaves `d` unset rather than empty, which is the same statement
// Runner.moveParameter makes about a move with nothing to move.
func (r *Runner) snapshotVariable(name string) (heldValue, bool) {
	h, ok := r.snapshotOneName(name)
	if !ok {
		return heldValue{}, false
	}
	if !h.compound {
		return h, true
	}
	prefix := compoundMemberPrefix(name)
	for _, full := range r.compoundDescendants(name) {
		m, ok := r.snapshotOneName(full)
		if !ok {
			// An interior node with nothing of its own — `c.b` where only
			// `c.b.y` is set — which is a compound in the listing and holds
			// no value here. The descendant below it carries the path, so
			// nothing is lost by leaving it out.
			continue
		}
		m.suffix = full[len(prefix)-len(memberSep):]
		h.members = append(h.members, m)
	}
	return h, true
}

// snapshotOneName reads one name's own value, without the members under it.
func (r *Runner) snapshotOneName(name string) (heldValue, bool) {
	h := heldValue{attrs: r.captureAttributes(name)}
	switch {
	case r.isCompoundVariable(name):
		h.compound = true
	case r.AssocArrays[name] != nil && !r.removed[name]:
		h.assoc, h.isAssoc = maps.Clone(r.AssocArrays[name]), true
	case r.Arrays[name] != nil && !r.removed[name]:
		h.arr, h.isArr = maps.Clone(r.Arrays[name]), true
	default:
		// getVar rather than Vars, so that a name read out of the inherited
		// environment is a source too — the same reading appendedOverAScalar
		// takes of "what is this name holding".
		v, held := r.getVar(name)
		if !held {
			return heldValue{}, false
		}
		h.scalar, h.isScalar = v, true
	}
	return h, true
}

// restoreVariable writes a snapshot under a name, members and all.
//
// The root's attributes are deliberately not applied and the members' are:
// see the measured split at the head of this file.
func (r *Runner) restoreVariable(name string, h heldValue) {
	r.restoreOneName(name, h, false)
	for _, m := range h.members {
		r.restoreOneName(name+m.suffix, m, true)
	}
}

// restoreOneName writes one snapshotted value under a name.
func (r *Runner) restoreOneName(name string, h heldValue, withAttributes bool) {
	if withAttributes {
		// Before the value, because an attribute is what a store folds the
		// value through: a member written first and given its letters
		// afterwards would keep whatever text it arrived with.
		r.restoreAttributes(name, h.attrs)
	}
	switch {
	case h.compound:
		r.markCompoundVariable(name)
	case h.isAssoc:
		r.markAssoc(name)
		// The removal record the target's own `unset` left, taken off for the
		// declaration as well as for the elements: an empty table copied onto
		// an unset name is still a table, and a name left marked removed
		// listed as nothing at all.
		r.nameIsBack(name)
		keys := make([]string, 0, len(h.assoc))
		for k := range h.assoc {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			// Through the store rather than into the table, which is not
			// tidiness: the target was just unset, and a name written back
			// under it has to leave the removal record behind it. A direct
			// table write left `typeset -p d` listing `typeset -A d=()` over
			// a table whose elements read back perfectly well.
			r.storeAssocElement(name, k, h.assoc[k])
		}
	case h.isArr:
		r.storeArray(name, h.arr)
	case h.isScalar:
		r.setVar(name, h.scalar)
	}
}

// copyVariableInto replaces everything one name holds with everything another
// one holds, and reports whether there was a source to copy.
//
// The snapshot is taken before the target is cleared, which is what lets the
// two be the same name.
func (r *Runner) copyVariableInto(to, from string) bool {
	h, ok := r.snapshotVariable(from)
	if !ok {
		return false
	}
	r.unsetName(to)
	r.restoreVariable(to, h)
	return true
}

// compoundDeclarationValue is what a `-C` declaration does with the value on
// its operand: reads it as the name of a variable and copies that variable in
// whole, kind included.
//
// The declaration has already marked the name a compound — see
// markDeclaredCompound — so what is left here is the value, and the three
// answers it can have are the measured ones: a name that is set is copied, a
// name that is not unsets the target, and text that is not a name at all
// leaves the empty compound the letter declared.
func (r *Runner) compoundDeclarationValue(name, value string) {
	// The target is emptied first whatever the source turns out to be:
	// `typeset -C d=(z=2); typeset -C d=c` is `typeset -C d=(a=1)` there, with
	// no `z` left, and `typeset -C c=c` is `typeset -C c=()` — which is this
	// clearing, seen through a source that is the target.
	r.unsetCompoundMembers(name)
	if !r.copySourceName(value) {
		return
	}
	if !r.copyVariableInto(name, value) {
		// A source nobody set takes the target with it rather than leaving an
		// empty compound: measured, `d=hello; typeset -C d=nosuch` leaves
		// `[[ -v d ]]` false and `${d}` empty.
		r.unsetName(name)
	}
}

// compoundAssignedFromAName is `d=c` and `d+=c` where the target is already a
// compound and the value names another one.
//
// Only a compound source, which is the measured difference from the `-C`
// spelling: `x=5; d=(a=9); d=x` is `d=x` there and `a=(p q); d=(a=9); d=a` is
// `d=a`, so a name of any other kind is a value on this path rather than a
// source. The second result is whether the assignment was taken.
func (r *Runner) compoundAssignedFromAName(name, value string, appends bool) bool {
	if !r.isCompoundVariable(name) || !r.copySourceName(value) {
		return false
	}
	if !r.isCompoundVariable(value) {
		return false
	}
	if !appends {
		r.unsetCompoundMembers(name)
		r.copyVariableInto(name, value)
		return true
	}
	// `+=` merges, with the source's members standing over the target's own:
	// `c=(a=1); typeset -C d=(z=2); d+=c` is `typeset -C d=(a=1;z=2)` and
	// `c=(a=1 z=3)` over the same target is `typeset -C d=(a=1;z=3)`.
	h, ok := r.snapshotVariable(value)
	if !ok {
		return true
	}
	for _, m := range h.members {
		r.restoreOneName(name+m.suffix, m, true)
	}
	return true
}
