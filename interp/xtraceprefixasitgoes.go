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
	line := &openTraceLine{owner: r, unflushed: true}
	outer := r.openTrace
	r.openTrace = line
	defer func() { r.openTrace = outer }()
	stopped := false
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
		target := traceAssignTarget(a, nil)
		// What a substitution in the value inherits. See prefixLineForAChild.
		line.pending = so() + target
		frozen := r.readonly[a.Name]
		var word string
		if frozen {
			value := r.prefixExpansion(a)
			if !r.prefixWalkFailed(walk) {
				// Held for the route that refuses it, so a substitution in
				// it runs once.
				r.recordPrefixTraceValue(a, value)
				word = r.traceAssign(a, value, r.prefixGlobTraced(a), *d)
			}
		} else {
			one := c.Assigns[i : i+1]
			r.expandPrefixTraceValues(one)
			if words := r.prefixTraceWords(one, *d); len(words) == 1 {
				word = words[0]
			}
		}
		if r.prefixWalkFailed(walk) {
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
	text := prefix + strings.Join(done, " ")
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
	outer := r.traceTo
	r.traceTo = held.to
	r.tracef("%s", held.text)
	r.traceTo = outer
}

// prefixLineForAChild is the copy of an unfinished trace line a substitution
// starts with. The child writes it in front of its first trace line, where
// the dialect's own buffered stream would, and drops it if it writes none.
func (r *Runner) prefixLineForAChild() *openTraceLine {
	line := r.openTrace
	if line == nil || line.owner != r || line.pending == "" {
		return nil
	}
	return &openTraceLine{pending: line.pending, inherited: true}
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
