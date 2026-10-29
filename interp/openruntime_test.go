// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// The open constructs are pushed in place and truncated on the way out, so
// the three things that make that safe have to hold. A clause replaces its
// construct's own slot. A call's view starts past its caller's slots and
// hands them back as they were. A copy of the runner writes an array of its
// own, so neither side's push or replacement reaches the other's view.
func TestTheOpenConstructsArePushedInPlaceSafely(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	view := func(r *Runner) string { return strings.Join(r.openRun, " ") }
	outer := r.openRuntime("while")
	inner := r.openRuntime("if")
	r.replaceOpenRuntime("then")
	if got := view(r); got != "while then" {
		t.Fatalf("the clause did not replace its construct: %q", got)
	}

	call := r.openRuntimeFresh(nil)
	if got := view(r); got != "" {
		t.Errorf("a call's body starts inside %q", got)
	}
	inCall := r.openRuntime("if")
	r.replaceOpenRuntime("else")
	if got := view(r); got != "else" {
		t.Errorf("inside the call the field is %q", got)
	}
	inCall.undo()
	call.undo()
	if got := view(r); got != "while then" {
		t.Errorf("a call's pushes reached its caller's slots: %q", got)
	}

	c := r.clone()
	c.replaceOpenRuntime("elif-then")
	inCopy := c.openRuntime("for")
	if got := view(r); got != "while then" {
		t.Errorf("a copy's replacement and push reached the shell it was copied from: %q", got)
	}
	inCopy.undo()
	r.replaceOpenRuntime("else")
	if got := view(c); got != "while elif-then" {
		t.Errorf("the shell's replacement reached its copy: %q", got)
	}

	inner.undo()
	outer.undo()
	if len(r.openRun) != 0 {
		t.Errorf("the stack did not empty: %q", view(r))
	}
	// Once the array has room, a construct costs nothing: that was a fresh
	// slice and a closure for every one of them (#6423).
	if n := testing.AllocsPerRun(100, func() {
		u := r.openRuntime("if")
		r.replaceOpenRuntime("then")
		f := r.openRuntimeFresh(nil)
		r.openRuntime("for").undo()
		f.undo()
		u.undo()
	}); n != 0 {
		t.Errorf("a construct allocated %v times; want 0", n)
	}
}
