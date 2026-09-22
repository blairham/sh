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
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	delete(r.removed, name)
	r.Vars[name] = value
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
	if r.readonly[a.Name] {
		// Refused, so nothing was assigned and there is nothing for the next
		// entry to see: the name keeps what the shell holds.
		return "", false
	}
	return r.prefixEntryName(a.Name), true
}
