// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

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
type heldPrefix struct {
	undo []savedVar
	// route is the command the prefix stands in front of, where the walk
	// knows it, so a held scalar carries the export attribute the route's
	// store will give it. See interp/prefixheldexport.go.
	route heldExportRoute
}

// hold records the name's current state and writes the value over it.
//
// The cell is written directly rather than through Runner.setVar: this is a
// value on its way to a store the route below will make for real, and a
// discipline function or a produced parameter's writer fired here would fire
// twice for one assignment.
//
// With the export attribute the route gives the entry, so that a command
// substitution behind it hands a child what the route will (#7003).
func (h *heldPrefix) hold(r *Runner, name, value string) {
	h.undo = append(h.undo, r.saveVar(name))
	r.arraysDel(name)
	r.dropAssocTable(name)
	r.attrClear(name, attrRemoved)
	r.varsPut(name, value)
	if h.route.set {
		if on, moves := r.heldPrefixExport(h.route, name); moves {
			r.exportedPut(name, on)
		}
	}
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
// array or table it places — see holdPlacedLiteral. A subscripted entry is
// held as the element it writes, where the walk has read its subscript — see
// Runner.holdPrefixElement.
func (r *Runner) holdPrefixEntry(h *heldPrefix, a *syntax.Assign, joined string) {
	if prefixIsSubscripted(a) {
		// The element is placed from the entry's own expansion, which an
		// append joins to the element rather than to the name's scalar.
		if part, ok := r.prefixTraceValue(a); ok {
			r.holdPrefixElement(h, a, part)
		}
		return
	}
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
	if arr, appends := r.prefixAppendsAnElement(a); appends {
		// The array with the value after it — see prefixAppendsAnElement.
		h.holdArray(r, name, append(r.readArray(arr), joined))
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
			if r.prefixExpandedInTheChild {
				// The child places the literal and owns the refusal: the
				// placement below reports the subscript, and the walk gives
				// the command up. See Runner.prefixExpansionShell.
				continue
			}
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

// release puts every held cell back, most recent first — unless a failure in
// the command is ending the shell where the dialect leaves a prefix's entries
// as they were assigned. See interp/prefixentriesstay.go.
func (h *heldPrefix) release(r *Runner) {
	if h.route.set && len(h.undo) > 0 && r.prefixEntriesStay(h.route.kind) {
		h.undo = nil
		return
	}
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

// prefixExpansionShell is the shell an external command's assignment prefix
// is expanded in: a copy of this one, where the dialect expands it in the
// child that runs the command, and this shell itself otherwise.
//
// Measured 2026-10-16 under `-c`, `env -i PATH=/usr/bin:/bin` (#6784, #6783):
//
//	                                            zsh 5.9.2, ksh93u+   bash 5.3, dash, ash
//	i=1; y=$((i++)) /usr/bin/true; echo $i      1                    2
//	unset i; y=${i:=5} /usr/bin/true; echo $i   (empty)              5
//	i=1; x=([i++]=a) /usr/bin/true; echo $i     1 (zsh)
//	x=([0]=a) /usr/bin/true; echo after $?      bad subscript …, after 1 (zsh)
//	i=1; y=$((i++)) true; echo $i               2 — a builtin is this shell
//
// The entries behind still see what an entry before them did — `x=([i++]=a)
// y=$i:${x[1]} /usr/bin/printenv y` is `2:a` — because the child expands them
// all in order; it is only the shell that never sees it. So the copy is made
// once for the whole prefix, and only where a value could change something:
// an arithmetic expansion, a parameter expansion that assigns or subscripts,
// or an array literal. Every other prefix expands in this shell to the same
// result, and copying the tables for it would cost every prefixed command.
func (r *Runner) prefixExpansionShell(assigns []*syntax.Assign) *Runner {
	may := false
	for _, a := range assigns {
		if prefixValueMayChangeTheShell(a) {
			may = true
			break
		}
	}
	if !may || !r.ask(r.sem().PrefixExpandsInTheExternalCommandsChild,
		"an external command's assignment prefix expanded in its child") {
		return r
	}
	child := *r
	child.dialectMoves = nil
	child.bodyTexts = nil
	child.pipeCounts = nil
	child.ownTables(r)
	child.prefixExpandedInTheChild = true
	return &child
}

// prefixValueMayChangeTheShell is whether expanding a prefix's value could
// write to the shell: arithmetic, a parameter expansion that assigns or
// subscripts, or an array literal, whose subscripts are arithmetic — or the
// entry's own subscript, which is arithmetic or a key's words, and which the
// child reads too: `i=1; b[i++]=v /usr/bin/true` leaves `i` at 1 in ksh93u+
// and zsh 5.9.2. See interp/prefixsubscriptinturn.go.
func prefixValueMayChangeTheShell(a *syntax.Assign) bool {
	if a.IsArray || len(a.Elems) > 0 || prefixIsSubscripted(a) {
		return true
	}
	if a.Value == nil {
		return false
	}
	for _, sp := range a.Value.Spans {
		switch sp.Kind {
		case syntax.ArithSubst:
			return true
		case syntax.ParamExp:
			if strings.ContainsAny(sp.Value, "=[") {
				return true
			}
		}
	}
	return false
}

// takeThePrefixChildsFailure carries what the child's expansion left on the
// failure record back to this shell, which is where the walk is judged and
// the command given up. Nothing where the prefix expanded here.
func (r *Runner) takeThePrefixChildsFailure(ex *Runner) {
	if ex == r {
		return
	}
	r.expandErr, r.badSubscript, r.badRange = ex.expandErr, ex.badSubscript, ex.badRange
	if ex.givingUpAlready() {
		// A failure that unwound the child. A `return` or `break` raised by
		// code inside a value is the child's alone and is not carried: the
		// child's end is the end of it (#6752).
		r.ctl, r.abandon, r.errexitStopped = ex.ctl, ex.abandon, ex.errexitStopped
	}
	if ex.expandErr || ex.givingUpAlready() {
		r.status = ex.status
	}
}
