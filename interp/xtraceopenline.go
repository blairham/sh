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
// to say in the middle writes exactly the line it always did. A substitution
// in a value does not flush it. It runs in a child that has a copy of the
// unwritten line, and the child writes that copy ahead of its own first
// line: `set -x; x=$(echo a) y=$(echo b)` is `+zsh:1> x=+zsh:1> echo a`,
// `+zsh:1> x=a y=+zsh:1> echo b`, then `+zsh:1> x=a y=b ` (#5546). See
// Runner.prefixLineForAChild and Semantics.TraceAssignmentListIsWrittenAsItGoes.

// openTraceLine is the text of an assignment list's trace line that has not
// been written yet. owner is the runner writing the list, so that a
// subshell's clone, which copies the pointer, does not write its parent's
// line.
type openTraceLine struct {
	owner   *Runner
	pending string
	// inherited says this is a child's copy of its parent's line, written
	// ahead of the child's first trace line or its first diagnostic. See
	// Runner.prefixLineForAChild.
	inherited bool
	// unflushed says a diagnostic does not write this line first: a
	// command's prefix line is written after its complaint, if at all. See
	// interp/xtraceprefixasitgoes.go.
	unflushed bool
	// carries is a child's inherited line whose text this one starts with
	// and has not taken, because this line may be dropped rather than
	// written. Writing this one writes that text, so it is emptied there.
	// See Runner.openTraceLineCarryingTheInherited.
	carries *openTraceLine
	// flushed is the part of the line already written, ahead of a trace
	// line a command run by this shell wrote in the middle of the list —
	// code in a glob qualifier. The rest of the line goes on from there.
	// See Runner.writeOpenTraceLineAhead.
	flushed string
	// keptForAChild says the pending text was last written ahead of a
	// child's command and is still this shell's to write: a diagnostic that
	// child writes does not write it again.
	keptForAChild bool
	// prefixOf is the command whose assignment prefix this line is, where
	// it is one; a bare list's line names none.
	prefixOf *syntax.SimpleCmd
}

// writeOpenTraceLineAhead is the text of the open line to write ahead of a
// trace line this shell is about to write for a command, and whether there
// is any: a command run in the middle of an assignment list — code in a glob
// qualifier — writes the list's line as far as it has got ahead of its own,
// and the list goes on from there. A command that runs in a **child** writes
// it too and leaves it pending, as a substitution's copy does, so the next
// command written by this shell writes it again. Measured 2026-10-27 on zsh
// 5.9.2 under -fc, in a directory where `[ab]` matches `a` and `b` (#6752):
//
//	set -x; x=( [ab](e:"print -u2 code":) )
//	    +zsh:1> x=+zsh:1> print -u2 code   code   +zsh:1> print -u2 code
//	    code   ( a b )
//	set -x; x=( [ab](e:"print -u2 code":) ) print in
//	    … ( a b ) +zsh:1> print in   — the prefix line goes on the same way
//	set -x; x=( [ab](e:"/usr/bin/true":) )
//	    +zsh:1> x=+zsh:1> /usr/bin/true   twice, then +zsh:1> x=( a b )
//	set -x; x=( [ab](e:"/usr/bin/true; true":) )
//	    +zsh:1> x=+zsh:1> /usr/bin/true   +zsh:1> x=+zsh:1> true
//	    +zsh:1> /usr/bin/true   +zsh:1> true   ( a b )
//	set -x; x=( [ab](e:"true | true":) )
//	    the same shape: the forked element keeps the text, the last takes it
//	set -x; x=( [ab](e:"nosuchcmd_zz; true":N) )
//	    +zsh:1> x=+zsh:1> nosuchcmd_zz, the complaint, then as the row above
//
// So a child's trace and a child's complaint are written against the line
// without taking it, and a command of this shell takes it.
func (r *Runner) writeOpenTraceLineAhead() (string, bool) {
	line := r.openTrace
	if line == nil || line.owner != r || line.pending == "" {
		return "", false
	}
	text := line.pending
	if r.traceLineForAChild {
		line.keptForAChild = true
		return text, true
	}
	line.pending, line.flushed, line.keptForAChild = "", line.flushed+text, false
	if line.carries != nil {
		line.carries.pending = ""
	}
	return text, true
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
		if elementWriteHasAValueToOrder(a) && r.sem().ElementSubscriptIsExpandedBeforeTheValue == Yes {
			// And an element write whose value waits for its subscript,
			// which this loop expands ahead of everything. Read rather than
			// asked, for the same reason: no dialect writing its line as it
			// goes expands the subscript first. See interp/elementorder.go.
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
	line := &openTraceLine{owner: r, pending: r.openTraceLineText()}
	outer := r.openTrace
	r.openTrace = line
	defer func() { r.openTrace = outer }()
	for _, a := range assigns {
		target := traceAssignTarget(a, nil, nil, r.wordAsWrittenLayout())
		line.pending += target
		failedBefore := r.expandErr
		value, fields, globbed := r.expandScalarAssignValue(a.Value)
		if r.assignValueFailed(failedBefore) {
			// The list stops, nothing is stored, and the line gets its
			// newline unless the failure left it open. Measured 2026-10-03,
			// `b=0; set -x; a=1 b=$((1/0)) c=3; echo "[$b][$c]"` is `+zsh:1>
			// a=1 b=`, the complaint and an empty line, and nothing after it
			// (#5546).
			if r.traceLineLeftOpenByTheFailure() {
				return
			}
			break
		}
		e := r.prepareTracedAssign(a, value)
		e.fields, e.fieldsSet = fields, globbed
		if r.assignValueFailed(failedBefore) {
			// An array literal's elements are expanded here rather than
			// above, and a failure among them stops the list the same way:
			// nothing of the literal is written, where the words as the
			// script spelled them used to end the line. Measured
			// 2026-10-15 on zsh 5.9.2, `set -x; x=( a $((1/0)) b )` is
			// `+zsh:1> x=`, the complaint and an empty line (#6623).
			if r.traceLineLeftOpenByTheFailure() {
				return
			}
			break
		}
		// An element literal's words are written once the store has built
		// them, which is after a substitution among them has run and before
		// the subscript is read. See Runner.openTraceTakesTheLiteral (#6764).
		literalLater := a.IsArray && a.Index != nil && !e.tracesAfterTheStore
		if !e.tracesAfterTheStore && !literalLater {
			line.pending += r.traceAssignRest(a, value, e, d, target)
		}
		if literalLater {
			r.openTraceLiteral = a
		}
		r.withPreparedValue(ctx, e)
		r.openTraceLiteral = nil
		if e.tracesAfterTheStore {
			line.pending += r.traceAssignRest(a, value, e, d, target)
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

// assignValueFailed reports whether expanding an assignment's value failed:
// it ended the shell, or it would not expand and the list stops there.
func (r *Runner) assignValueFailed(failedBefore bool) bool {
	return r.ctl != controlNone || r.expandErr && !failedBefore
}

// traceLineLeftOpenByTheFailure reports whether a list's trace line is left
// where it got to, with no newline, after a value that failed.
//
// Only where the failure is a request to stop: a `${x?word}` the shell
// exits on at once. Every other failure ends the line, with the complaint
// written into it and the newline after it, whether or not the shell then
// ends. Measured 2026-10-15 on zsh 5.9.2 under -f, `set -x` in front:
//
//	x=${q?b}           -c  `+zsh:1> x=zsh:1: q: b`, no empty line
//	x=${q?b}           -i  `+zsh:2> x=zsh: q: b`, then an empty line
//	x=${q?b}        -i -c  the same, and the shell then ends
//	set -u; x=$nope    -c  `+zsh:1> x=zsh:1: nope: …`, then an empty line
//	x=$((1/0))         -c  the same
//
// So whether the shell ends is not what decides it, since `set -u` ends it
// too. What decides it is whether the operand is an exit request, and in an
// interactive shell it is not (see Runner.paramErrorAsAnErrorCondition).
// This used to leave the line open after every failure that set a transfer,
// which gave the rest of the rows the first row's answer (#6623).
func (r *Runner) traceLineLeftOpenByTheFailure() bool {
	return r.ctl == controlExit && r.abandon == abandonParamError && !r.Interactive &&
		r.sem().ParamErrorIsAnExitRequest == Yes
}

// openTraceLineText is what a line opened a word at a time starts with: the
// prefix, behind whatever part of its parent's line a child was handed and
// has not written yet.
//
// The parent's line is in the child's buffer ahead of anything the child
// writes, so a line the child opens goes after it, and whatever writes the
// child's line writes the parent's first. Measured 2026-10-15 on zsh 5.9.2,
// `set -x; x=$( y=$( echo ${q?b} ) )` writes `+zsh:1> x=+zsh:1> y=` and then
// the complaint (#6623).
func (r *Runner) openTraceLineText() string {
	return r.takeInheritedTraceLine() + r.tracePrefix()
}

// openTraceLineCarryingTheInherited opens a line that is held only for what
// a complaint or a child writes of it, and is then dropped for a line written
// whole: a `case` arm's patterns as they are reached. A child's inherited text
// goes in front of it, as it goes in front of a list's line, but it is not
// taken, so the whole line written afterwards still carries it. Measured
// 2026-10-15 on zsh 5.9.2, `set -x; x=$( case a in $(echo a)) ;; esac )`
// writes `+zsh:1> x=+zsh:1> case a (+zsh:1> echo a` and then `+zsh:1>
// x=+zsh:1> case a (a)`, and with `$((1/0))` as the pattern it writes
// `+zsh:1> x=+zsh:1> case a (`, the complaint, and then only `)` (#6623).
func (r *Runner) openTraceLineCarryingTheInherited(text string) *openTraceLine {
	line := &openTraceLine{owner: r, pending: text}
	if in := r.openTrace; in != nil && in.inherited && in.owner == r && in.pending != "" {
		line.pending, line.carries = in.pending+text, in
	}
	return line
}

// traceAssignRest is the part of one assignment's word after its target,
// with the space that follows every word on this line.
func (r *Runner) traceAssignRest(
	a *syntax.Assign,
	value string,
	e *expandedAssign,
	d *Diagnostics,
	target string,
) string {
	return strings.TrimPrefix(r.traceAssign(a, value, e, d), target) + " "
}

// flushOpenTraceLine writes what an open assignment trace line holds so far,
// ahead of a diagnostic about to be written into the middle of it.
//
// A child's copy of its parent's line is also written ahead of a diagnostic,
// as it is ahead of the child's first trace line. Measured 2026-10-15 on zsh
// 5.9.2 under -f, through -c and at a prompt: `set -x; x=$( echo ${q?b} )`
// writes `+zsh:1> x=` and then the complaint, and so do `a=1 x=$( … ) b=2`,
// `x=( $( … ) )`, and a `case` pattern holding the substitution. A command's
// prefix line is the exception in the child as it is in the parent: `x=$(
// echo ${q?b} ) true` writes only the complaint (#6623).
func (r *Runner) flushOpenTraceLine() {
	line := r.openTrace
	if line == nil || line.owner != r || line.pending == "" || line.unflushed || line.keptForAChild {
		return
	}
	text := line.pending
	line.pending = ""
	if line.carries != nil {
		line.carries.pending = ""
	}
	r.tracef("%s", text)
}

// openTraceTakesTheLiteral writes an element literal's words onto the open
// trace line, as they were built. Measured 2026-10-15 on zsh 5.9.2 under
// `-c`, `a=(1 2); set -x; a[$(echo SUB >&2; echo 1)]=(p $(echo VAL >&2))`:
//
//	+zsh:1> a[$(echo SUB >&2; echo 1)]=+zsh:1> echo VAL
//	VAL
//	+zsh:1> a[$(echo SUB >&2; echo 1)]=( p ) +zsh:1> echo SUB
//
// So the line holds the target alone while the literal's own substitution
// runs and the expanded words by the time the subscript's does, where this
// wrote the words as the script spelled them on both (#6764).
func (r *Runner) openTraceTakesTheLiteral(a *syntax.Assign, words []string) {
	if r.openTraceLiteral != a || r.openTrace == nil || r.openTrace.owner != r {
		return
	}
	r.openTraceLiteral = nil
	d := r.diag()
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = r.traceQuote(w, d.TraceQuoting, d.TraceMetacharacters)
	}
	r.openTrace.pending += wrapArrayLiteral(strings.Join(quoted, " "), d.TraceArrayLiteral) + " "
}
