// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"sync"
	"testing"
)

// A callback that leaves a **postdisplay** has it drawn, which is the whole of
// how an inline suggestion arrives asynchronously.
//
// TestACallbackThatRewritesTheLineIsDrawn is the same round trip for the
// buffer and the cursor, and it passed throughout: `serveDescriptor` took
// `out.Buffer` and `out.Cursor` from the answer and dropped `out.Postdisplay`
// on the floor. So a shell whose callback set only a postdisplay — which is
// exactly what a suggestion is — was told its answer had been drawn and
// nothing had been.
//
// It is the descriptor path that makes this the interesting one. A widget
// bound to a key comes back through runShellWidget, which has always carried
// the field; a suggestion fetched in the background comes back through here,
// and that is the path a real configuration takes (#4413).
func TestACallbackThatLeavesAPostdisplayHasItDrawn(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })

	// Once, for the reason TestACallbackThatRewritesTheLineIsDrawn gives: the
	// pipe is never drained, so the callback goes on being offered the line.
	var once sync.Once
	p := &descriptorProbe{drain: read, answer: func(in Line) (Line, bool) {
		out, changed := in, false
		once.Do(func() {
			// The line is left exactly as it was and only the postdisplay is
			// added, which is what a suggestion does and what makes this a
			// test of the field rather than of the round trip.
			out, changed = Line{Buffer: in.Buffer, Cursor: in.Cursor, Postdisplay: "-dropped"}, true
		})
		return out, changed
	}}
	s := probeSession(t, p)
	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")
	p.arm(int(read.Fd()))
	if _, werr := s.control.WriteString("echo kept"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "echo kept", "the typed line")
	if _, werr := write.WriteString("wake\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.screen, "-dropped", "the postdisplay the callback left")
	p.disarm()

	// And it is drawn *after* the line without being part of it: accepting
	// runs what was typed, not what was suggested.
	if _, werr := s.control.WriteString("\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "kept", "the command's output")
	if got := s.ran.String(); strings.Contains(got, "dropped") {
		t.Errorf("the postdisplay was run as part of the line: %q", got)
	}
	s.end()
}
