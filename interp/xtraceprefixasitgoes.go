// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A command's assignment prefix, traced a word at a time.
//
// This is the dialect that writes an assignment list's line a word at a time
// (Semantics.TraceAssignmentListIsWrittenAsItGoes), and it writes a prefix
// the same way. Its trace stream is buffered, and that is what can be seen.
// A word's name goes into the buffer before its value is expanded, so a
// substitution in the value, which runs in a child with a copy of the
// buffer, writes the line so far in front of its own. A value that will not
// expand, or a store the shell refuses, leaves the line where it got to, and
// that text is written only when the shell exits. The complaint therefore
// comes first, and the line has no newline. Measured 2026-10-03 on zsh 5.9.2
// under `-f -c` (#5546):
//
//	set -x; a=$(echo s >&2) true
//	        `+zsh:1> a=+zsh:1> echo s`, `s`, `+zsh:1> a='' +zsh:1> true`
//	set -x; a=1 b=${x?boom} c=$(echo s >&2) true
//	        `zsh:1: x: boom`, then `+zsh:1> a=1 b=`, and the shell is gone
//	set -x; a=1 b=$((1/0)) :; echo st=$?
//	        `zsh:1: division by zero`, then `+zsh:1> a=1 b=`
//	readonly r; set -x; a=1 r=$(echo s >&2) c=3 true
//	        `+zsh:1> a=1 r=+zsh:1> echo s`, `s`, the refusal, then
//	        `+zsh:1> a=1 r='' ` — the frozen name's value is expanded and
//	        written, and nothing after it is
//	readonly r; set -x; a=1 r=2 /bin/echo hi; echo st=$?
//	        the refusal and `st=1`: the shell carries on, and the line it
//	        left behind is never written
//
// The last is a program, which this shell would start in a child, and
// `command true` is the same. The line left behind is in that child's
// buffer, and the child does not write it. The rule here is the one that
// covers both: the line so far is written only if the shell stops.
//
// ksh93 writes a prefix entry by entry and bash writes each entry on a line
// of its own (see Runner.walkThePrefixBeforeTheRedirections). dash and
// BusyBox ash write no line for a prefix that failed (#5509).

// heldPrefixTrace is a prefix's trace line that stopped short. It is
// written if the shell stops and dropped if it carries on.
type heldPrefixTrace struct {
	text string
	to   io.Writer
}

// tracesPrefixAsItGoes reports whether this dialect writes a command's
// prefix a word at a time.
func (r *Runner) tracesPrefixAsItGoes() bool {
	return r.sem().TraceAssignmentListIsWrittenAsItGoes == Yes
}

// tracePrefixAsItGoes expands a command's prefix for its trace line, a word
// at a time, and writes the line, or holds the part of it that was reached.
func (r *Runner) tracePrefixAsItGoes(c *syntax.SimpleCmd, argv []string, walk prefixWalk) {
	d := r.diag()
	prefix := r.tracePrefix()
	var done []string
	so := func() string {
		if len(done) == 0 {
			return prefix
		}
		return prefix + strings.Join(done, " ") + " "
	}
	// The part of the parent's line a substitution's child was handed, which
	// this line starts with: the first line the child opens takes it, as an
	// assignment list's does. See openTraceLine.lead (#6711).
	line := &openTraceLine{owner: r, unflushed: true, prefixOf: c, lead: r.takeInheritedTraceLine()}
	outer := r.openTrace
	r.openTrace = line
	defer func() { r.openTrace = outer }()
	stopped := false
	// Held across the entries rather than per entry, so each value sees the
	// ones in front of it, with the export attribute the route gives each.
	// See expandPrefixTraceValuesHolding.
	held := heldPrefix{route: r.heldExportRouteOf(argv)}
	defer held.release(r)
	// And an element write's subscript is read where its entry stands, so the
	// element is held for the entries behind it. See
	// interp/prefixsubscriptinturn.go.
	subscripts := r.prefixSubscriptsReadHere(c.Assigns, argv)
	for i, a := range c.Assigns {
		if a.Operand {
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok && a.IsArray {
			continue
		}
		if r.subscriptedPrefixDropped(a) {
			continue
		}
		target := traceAssignTarget(a, nil, nil, r.wordAsWrittenLayout())
		// What a substitution in the value inherits, and what a command
		// run in the middle of the prefix writes ahead of its own line —
		// less whatever of it has gone out already. See
		// prefixLineForAChild and Runner.writeOpenTraceLineAhead.
		line.pending = line.lead + strings.TrimPrefix(so()+target, line.flushed)
		line.front, line.entry = so(), ""
		frozen := r.attrHas(a.Name, attrReadonly)
		var word string
		if frozen {
			value := r.prefixExpansion(a)
			if !r.prefixWalkFailed(walk) {
				// Held for the route that refuses it, so a substitution in
				// it runs once.
				r.recordPrefixTraceValue(a, value)
				word = r.traceAssign(a, value, r.prefixGlobTraced(a), d)
			}
		} else {
			one := c.Assigns[i : i+1]
			r.expandPrefixTraceValuesHolding(one, &held, "", subscripts)
			if words := r.prefixTraceWords(one, d); len(words) == 1 {
				word = words[0]
			}
		}
		if r.prefixWalkFailed(walk) {
			// Where the value was in and the subscript failed, the line got
			// as far as the whole entry. See Runner.prefixEntryValueIsIn.
			if line.entry != "" {
				target = line.entry + " "
			}
			done = append(done, target)
			stopped = true
			break
		}
		if word != "" {
			done = append(done, word)
		}
		if frozen {
			stopped = true
			break
		}
	}
	line.pending = ""
	if !stopped {
		r.tracePrefixAndCommand(c, argv)
		return
	}
	text := line.lead + strings.TrimPrefix(prefix+strings.Join(done, " "), line.flushed)
	if !r.prefixWalkFailed(walk) {
		// A refused store writes its word and the space after it.
		text += " "
	}
	r.heldPrefix = heldPrefixTrace{text: text, to: r.traceTo}
}

// writeHeldPrefixTrace writes a prefix's line that stopped short, where the
// shell has stopped, and drops it where the shell goes on.
func (r *Runner) writeHeldPrefixTrace() {
	held := r.heldPrefix
	r.heldPrefix = heldPrefixTrace{}
	if held.text == "" || r.ctl != controlExit {
		return
	}
	if r.inSubshell && r.abandon == abandonParamError {
		// A child the `${x?word}` stopped goes without writing it: measured
		// 2026-10-30 on zsh 5.9.2, `set -x; x=$( a=1 b=${q?b} true )` and
		// `( a=1 b=${q?b} true )` write the complaint and no `a=1 b=`,
		// where the same prefix in the shell itself is written at its end,
		// and a child a division stopped writes it as well (#6711).
		return
	}
	outer := r.traceTo
	r.traceTo = held.to
	r.tracef("%s", held.text)
	r.traceTo = outer
}

// prefixEntryValueIsIn puts a subscripted entry's whole word on its prefix's
// open line once the value is in, ahead of the reading of its subscript. A
// substitution in the subscript copies the line with the value on it, and a
// subscript that fails leaves the line there. Measured 2026-10-30 on zsh 5.9.2
// under `-f -c`, with `f() { :; }; b=(x y z)` (#6711):
//
//	set -x; b[$(echo s >&2; echo 1)]=v f
//	                       `+zsh:1> b[$(echo s >&2; echo 1)]=v +zsh:1> echo s`
//	set -x; b[1/0]=v f     the complaint, then `+zsh:1> b[1/0]=v `
//	set -x; b[${q?e}]=v f  the complaint, then `+zsh:1> b[${q?e}]=v `
//
// where the line held only `b[…]=`, as it does while the value expands.
func (r *Runner) prefixEntryValueIsIn(a *syntax.Assign) {
	line := r.openTrace
	if line == nil || line.owner != r || line.prefixOf == nil || line.front == "" {
		return
	}
	words := r.prefixTraceWords([]*syntax.Assign{a}, r.diag())
	if len(words) != 1 {
		return
	}
	line.entry = words[0]
	line.pending = line.lead + strings.TrimPrefix(line.front+line.entry+" ", line.flushed)
}

// prefixLineForAChild is the copy of an unfinished trace line a substitution
// starts with. The child writes it in front of its first trace line or its
// first diagnostic, where the dialect's own buffered stream would, and drops
// it if it writes neither.
func (r *Runner) prefixLineForAChild() *openTraceLine {
	line := r.openTrace
	if line == nil || line.owner != r || line.pending == "" {
		return nil
	}
	// Less the text this shell was handed, where that goes out first. See
	// Runner.writeHandedTextBeforeACopy.
	r.writeHandedTextBeforeACopy(line)
	if line.pending == "" {
		return nil
	}
	return &openTraceLine{pending: line.pending, inherited: true, unflushed: line.unflushed}
}

// takeInheritedTraceLine is the copied line a child writes ahead of its
// first trace line, once.
func (r *Runner) takeInheritedTraceLine() string {
	line := r.openTrace
	if line == nil || !line.inherited || line.owner != r || line.pending == "" {
		return ""
	}
	text := line.pending
	line.pending = ""
	return text
}
