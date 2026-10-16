// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A command's assignment prefixes are worked through **left to right**, and
// each one's right-hand side sees the ones in front of it.
//
// Measured 2026-09-22 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, with `f(){ echo "fn:[$A]"; }`:
//
//	unset K A; K=v1 A=${K#v} f
//	unset K A; K=v4 A=${K#v} eval 'echo eval:[$A]'
//	unset K A; K=v5 A=${K#v} /usr/bin/env
//
//	route               bash 5.3.20  dash 0.5.12  zsh 5.9.2  ksh93u+
//	a function          fn:[1]       fn:[1]       fn:[1]     fn:[1]
//	eval                eval:[4]     eval:[4]     eval:[4]   eval:[4]
//	an external         A=5          A=5          A=5        A=5
//
// Four columns, three routes, one answer, so it is a fact here rather than an
// axis. This shell expanded every value against the variables the command
// started with, so a later prefix reading an earlier one got nothing — at
// status 0, with the empty value carried into the child. `CC=gcc
// CFLAGS=-I${CC%cc}include make` is the shape a real script writes.
//
// Two things the same probes say do **not** change, and both are pinned
// beside the fix so a wider reading cannot creep in:
//
//   - The command's own argument words never see the prefix. `K=v3
//     A=${K#v} echo "[$A]"` is `[]` in all four.
//   - Whether the names survive the command is a separate question the panel
//     already splits on, and it is not this one.
//
// The value is made visible by writing the cell and putting it back, rather
// than by a scope of its own, for one reason: the function and the builtin
// routes already store each prefix as they walk the list, and they already
// answered these probes correctly because of it. What was missing was the two
// places that do not store — the ordered walk one dialect makes ahead of its
// redirections, which expanded every value before applying any, and the
// external route, which never stores at all because a child runs the command.
// Both now hold what they expanded for the entries behind them.

// holdPrefixValues makes the values of a command's assignment prefixes
// visible to the prefixes behind them, and hands back what to put back.
//
// One entry at a time, from the caller's own loop, so the ordering is the
// caller's — an entry it refused or skipped holds nothing, which is what
// keeps a refused prefix from being seen by the one behind it.
type heldPrefix struct{ undo []savedVar }

// hold records the name's current state and writes the value over it.
//
// The cell is written directly rather than through Runner.setVar: this is a
// value on its way to a store the route below will make for real, and a
// discipline function or a produced parameter's writer fired here would fire
// twice for one assignment.
func (h *heldPrefix) hold(r *Runner, name, value string) {
	h.undo = append(h.undo, r.saveVar(name))
	r.arraysDel(name)
	r.dropAssocTable(name)
	r.attrClear(name, attrRemoved)
	r.varsPut(name, value)
}

// holdArray is hold for an entry that puts an array under its name.
func (h *heldPrefix) holdArray(r *Runner, name string, values []string) {
	elems := make([]Element, len(values))
	for i, v := range values {
		elems[i] = Scalar(v)
	}
	h.holdPlaced(r, name, ArrayOf(elems...))
}

// holdPlaced is holdArray for an array already built, which is what a literal
// whose elements name their places comes to.
func (h *heldPrefix) holdPlaced(r *Runner, name string, a Array) {
	h.undo = append(h.undo, r.saveVar(name))
	r.varsDel(name)
	r.dropAssocTable(name)
	r.attrClear(name, attrRemoved)
	r.arraysPut(name, a)
}

// holdTable is hold for an entry whose literal is a keyed table.
func (h *heldPrefix) holdTable(r *Runner, name string, table AssocArray) {
	h.undo = append(h.undo, r.saveVar(name))
	r.varsDel(name)
	r.arraysDel(name)
	r.attrClear(name, attrRemoved)
	r.ownAssocs()
	r.assocsPut(name, table)
}

// holdPrefixEntry holds what one entry came to for the entries behind it, the
// one spelling every walk uses so that none of them can hold a different
// thing: the scalar, or the **array** an entry that stores one puts there.
// Measured 2026-10-15 from `-c` under `env -i PATH=/usr/bin:/bin`, `x=(a b)
// y=$x /usr/bin/printenv y` is `a` in ksh93u+ and `a b` in zsh 5.9.2 —
// each shell's own reading of an array's name — where holding the scalar the
// routes carry for a literal showed `y` nothing (#6547).
//
// A literal whose elements name their own places, or nest, is held as the
// array or table it places — see holdPlacedLiteral.
func (r *Runner) holdPrefixEntry(h *heldPrefix, a *syntax.Assign, joined string) {
	name, ok := r.prefixHoldableName(a)
	if !ok {
		return
	}
	if elems, isArray := r.prefixArrayLiteral(a); isArray {
		var values []string
		for _, e := range elems {
			if e.subscripted || e.nested != nil || e.members != nil {
				r.holdPlacedLiteral(h, a, name, elems)
				return
			}
			values = append(values, e.fields...)
		}
		h.holdArray(r, name, values)
		return
	}
	if fields, globbed := r.prefixGlobFields(a); globbed && len(fields) > 1 {
		h.holdArray(r, name, fields)
		return
	}
	h.hold(r, name, joined)
}

// holdPlacedLiteral holds a prefix literal whose elements name their places,
// or nest, as what the statement form places for it: an indexed array with
// each value where its subscript says, or a keyed table in the dialect, or
// over the name, that reads the subscripts as keys. Measured 2026-10-15
// under `env -i PATH=/usr/bin:/bin`, `-c` and a script file alike (#6636):
//
//	                                         ksh93u+   zsh 5.9.2
//	x=([2]=a) y=${x[2]} /usr/bin/printenv y   a         a
//	x=([2]=a b) y=${x[3]} /usr/bin/printenv y  (refused)  b
//	x=([1]=a [3]=c) y="${x[*]}" printenv y    a c       a  c
//	x=([2]=a [5]=b) y="${!x[*]}" printenv y   2 5
//	typeset -A x; x=([k]=v) y=${x[k]} …                 v
//	x=( (1 2) ) y=${x[0][1]} printenv y       2
//	f(){ echo "[$y]"; }; x=([2]=a) y=${x[2]} f   [a]  [a]
//	x=([2]=a) y=${x[2]} eval 'echo "[$y]"'    [a]       [a]
//	i=1; x=([i++]=a) y=$i:${x[1]} printenv y            2:a
//
// The last row is why the subscripts are read once: the arithmetic is the
// placement's, the hold places ahead of the store the route makes for real,
// and the store must not move `i` a second time. The index each one came to
// is recorded on the element — see Runner.literalSubscriptIndex — so the
// store places from the record. A subscript that does not come to a place
// the array has (an arithmetic failure, a position before the first) is left
// to the store, which owns the refusal, and nothing is held.
//
// An append is not held, nor a literal holding a compound body or one whose
// nesting makes it a table: each starts from what the name holds or runs a
// body at placement, and neither is a shape this needs to see.
func (r *Runner) holdPlacedLiteral(h *heldPrefix, a *syntax.Assign, name string, elems []literalElem) {
	if a.Append || nestingRetypesTheLiteral(elems) {
		return
	}
	for _, e := range elems {
		if e.members != nil {
			return
		}
	}
	if r.assocDeclared(name) || r.literalSubscriptIsAKey(name, elems) {
		table := AssocArray{}
		for _, e := range elems {
			if !e.subscripted || e.nested != nil || e.appendValue {
				return
			}
			table[e.sub] = Scalar(e.value)
		}
		h.holdTable(r, name, table)
		return
	}
	for i := range elems {
		e := &elems[i]
		if !e.subscripted || e.subPlaced {
			continue
		}
		idx, err := r.subscriptValue(e.sub)
		if err != nil || idx < r.arrayBase() {
			return
		}
		e.subPlaced, e.subAt = true, idx
	}
	built, ok := r.literalInto(name, NewArray(literalLen(elems)), 0, elems, true)
	if !ok {
		return
	}
	h.holdPlaced(r, name, built)
}

// release puts every held cell back, most recent first.
func (h *heldPrefix) release(r *Runner) {
	r.restoreVars(h.undo)
	h.undo = nil
}

// prefixHoldableName is the name a prefix entry's value should be held under
// while the entries behind it expand, and false where there is nothing to
// hold: a positional parameter, a subscripted word, or an operand of a
// declaration builtin rather than a prefix at all.
//
// A **name reference** moves the cell off the word the script wrote, so the
// hold follows it for the same reason the store does — see
// interp/namerefprefix.go.
func (r *Runner) prefixHoldableName(a *syntax.Assign) (string, bool) {
	if a.Operand || prefixIsSubscripted(a) {
		return "", false
	}
	if _, ok := positionalAssignIndex(a.Name); ok {
		return "", false
	}
	if r.attrHas(a.Name, attrReadonly) {
		// Refused, so nothing was assigned and there is nothing for the next
		// entry to see: the name keeps what the shell holds.
		return "", false
	}
	return r.prefixEntryName(a.Name), true
}
