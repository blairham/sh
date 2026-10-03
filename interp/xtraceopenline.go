// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// An assignment list's trace line, written a word at a time.
//
// Every shell that puts a list on one line writes the same line. They differ
// over *when* its words go out, and that only shows when something else is
// written in the middle of the list. zsh writes each word as the assignment
// is made, so a warning or a refusal from the store lands inside the line,
// after the words already written. Measured 2026-10-02 on zsh 5.9.2 under
// `-f`, with `sed -n l`:
//
//	readonly r; set -x; a=1 r=2
//	    +zsh:1> a=1 r=2 zsh:1: read-only variable: r$
//	    $
//	f(){ g=2 h=3 }; o(){ local g=1 h=1; f }; functions -Wt f; o
//	    +f:0> g=2 f: scalar parameter g set in enclosing scope in function f$
//	    h=3 f: scalar parameter h set in enclosing scope in function f$
//	    $
//	set -x; a=1 b=${x?boom}
//	    +zsh:1> a=1 b=zsh:1: x: boom$
//
// So the prefix and a word's name go out before its value is expanded, the
// value and a space follow once it is, the store comes after that, and
// whatever the store says is written where the line has got to. The line's
// newline comes at the end of the list, and a refused store still gets it.
// An expansion that ends the shell does not.
//
// "Goes out" means something else is written first. The pending text is
// held here and only written when a diagnostic arrives (see
// Runner.flushOpenTraceLine) or when the list ends, so a list with nothing
// to say in the middle writes exactly the line it always did. Process
// substitution and command substitution traces do not flush it. zsh's own
// output there reflects a forked child's copy of the unwritten line, which
// is a fact about how that shell writes and not an ordering of events, and
// this does not model it. See Semantics.TraceAssignmentListIsWrittenAsItGoes.

// openTraceLine is the text of an assignment list's trace line that has not
// been written yet. owner is the runner writing the list, so that a
// subshell's clone, which copies the pointer, does not write its parent's
// line.
type openTraceLine struct {
	owner   *Runner
	pending string
}

// assignListIsWrittenAsItGoes reports whether this list's one trace line is
// written a word at a time.
//
// A compound literal is traced as the member assignments its body performs,
// one line each, and a line of those cannot be held open around them, so a
// list carrying one is written the other way. No dialect that answers yes has
// the construct, and that is why this falls back instead of modeling it.
func (r *Runner) assignListIsWrittenAsItGoes(assigns []*syntax.Assign) bool {
	if len(assigns) == 0 {
		// No list and so no line: an empty command's bare line is written
		// elsewhere, and asking here would write a second, prefixed one.
		// See Semantics.EmptyCommandTrace.
		return false
	}
	for _, a := range assigns {
		if a.Members != nil {
			return false
		}
	}
	return r.ask(r.sem().TraceAssignmentListIsWrittenAsItGoes,
		"an assignment list's trace line written a word at a time")
}

// assignAllAsItGoes performs a traced assignment list, writing its one line
// a word at a time. Runner.assignAll writes it whole for the other answer.
func (r *Runner) assignAllAsItGoes(ctx context.Context, assigns []*syntax.Assign) {
	d := r.diag()
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	line := &openTraceLine{owner: r, pending: r.tracePrefix()}
	outer := r.openTrace
	r.openTrace = line
	defer func() { r.openTrace = outer }()
	for _, a := range assigns {
		target := traceAssignTarget(a, nil)
		line.pending += target
		value, fields, globbed := r.expandScalarAssignValue(a.Value)
		if r.ctl != controlNone {
			// The expansion ended the shell, and the line stays where it got
			// to: `b=${x?boom}` leaves `b=` and no newline.
			return
		}
		e := r.prepareTracedAssign(a, value)
		e.fields, e.fieldsSet = fields, globbed
		if !e.tracesAfterTheStore {
			line.pending += r.traceAssignRest(a, value, e, *d, target)
		}
		r.withPreparedValue(ctx, e)
		if e.tracesAfterTheStore {
			line.pending += r.traceAssignRest(a, value, e, *d, target)
		}
		if r.ctl != controlNone {
			// A refused store ends the list here, and the line gets its
			// newline all the same.
			break
		}
	}
	r.openTrace = outer
	r.tracef("%s\n", line.pending)
}

// traceAssignRest is the part of one assignment's word after its target,
// with the space that follows every word on this line.
func (r *Runner) traceAssignRest(a *syntax.Assign, value string, e *expandedAssign, d Diagnostics, target string) string {
	return strings.TrimPrefix(r.traceAssign(a, value, e, d), target) + " "
}

// flushOpenTraceLine writes what an open assignment trace line holds so far,
// ahead of a diagnostic about to be written into the middle of it.
func (r *Runner) flushOpenTraceLine() {
	line := r.openTrace
	if line == nil || line.owner != r || line.pending == "" {
		return
	}
	text := line.pending
	line.pending = ""
	r.tracef("%s", text)
}
