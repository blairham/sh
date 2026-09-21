// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"reflect"
)

// What a child inherits, where that is not the shell's own input.
//
// A shell hands its standard input to every command it runs, and that is the
// right answer for a shell: `cat` with nothing in front of it reads what the
// shell reads. It is the wrong answer for a front end whose input is a
// *question* — an Agent Client Protocol session can put a `read` to a person
// over its connection, and os/exec reads a non-file stdin on a child's behalf
// whether or not the child ever reads it, so a reader that asks would be
// asked once per external command. See [Runner.ChildStdin], which is where
// that is written down, and docs/design/acp.md (#934).
//
// The rule this file implements is one sentence: **the substitution reaches
// the shell's own input and nothing a script put there.** A redirection, a
// pipeline's pipe, a here-document and a substitution's end are all streams
// the script asked for by name, and a child gets each of them unchanged.

// ensureOwnStdin records what the shell's own standard input is, once, before
// any of a script's redirections can replace it.
//
// Called from RunPart beside the other ensures, so it happens on the first
// chunk of the first run and is a no-op afterwards. A front end that fills
// the field in after the shell has started has changed the shell's input
// mid-session, which is a thing no shell does and this does not model.
func (r *Runner) ensureOwnStdin() {
	if r.ownStdinSet {
		return
	}
	r.ownStdinSet = true
	if in := r.Stdin; in != nil && reflect.TypeOf(in).Comparable() {
		r.ownStdin = in
	}
}

// childStdin is the standard input to hand a process this shell starts.
//
// childIn is still the last word, because a descriptor the script *closed*
// must reach the child closed rather than empty — see childIn, where that
// distinction is the bug #1260 records.
func (r *Runner) childStdin() io.Reader { return childIn(r.inheritedStdin()) }

// inheritedStdin is what a child would inherit, before the closed-descriptor
// marker is applied.
func (r *Runner) inheritedStdin() io.Reader {
	if r.ChildStdin == nil || !r.ownInput(r.Stdin) {
		return r.Stdin
	}
	return r.ChildStdin
}

// ownInput reports whether this stream is the shell's own standard input.
//
// Seen through the concurrency guard, which is the one wrapper this package
// puts over the shell's input and leaves on a runner: a background job in the
// dialect that hands one the shell's descriptor is handed the guarded form of
// it, and a child in that job must be substituted for exactly as a child in
// the shell would be. See lockReader.
//
// The comparison is safe by construction rather than by hope. ownStdin is
// recorded only when its dynamic type is comparable, and `==` on two
// interface values panics only when their dynamic types are *equal* and that
// type is not comparable — so a stream of some other type answers false and
// never panics.
func (r *Runner) ownInput(in io.Reader) bool {
	if l, ok := in.(*lockedReader); ok {
		in = l.r
	}
	return r.ownStdin != nil && in == r.ownStdin
}
