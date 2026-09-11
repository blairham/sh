// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"context"
	"io"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// layers counts the wrappers over one of the Runner's streams, following each
// one down to whatever it is over.
//
// The count is the thing #2069 is about. A session that adds a pair of layers
// per descriptor handler is not merely untidy: two of the layers are guards
// over one lock, and the first write through them takes it twice.
func layers(w io.Writer) int {
	n := 0
	for w != nil {
		u, ok := w.(interp.WriterUnder)
		if !ok {
			return n
		}
		n++
		w = u.Unwrap()
	}
	return n
}

// TestHandlersThatStartBackgroundJobsDoNotStackGuardsOnTheStream is #2069 by
// the route it was found on.
//
// A descriptor handler runs with a layer over the Runner's streams — the line
// discipline, handed back at the handler's first byte. A background job
// started inside one leaves the shell's streams guarded afterwards, on purpose:
// the job outlives the statement that started it and can still write. Those
// two facts met badly. The guard went on *over* the handler's layer, so the
// handler could no longer find its own wrapper to take off, and the next
// handler put a second layer over the guard and the next job a second guard
// over that. Both guards hold one lock, because the lock belongs to the
// stream, and the write after that never came back.
//
// This machine's own prompt theme does exactly this: it arms a descriptor with
// `zle -F` and its handler starts jobs. The session reached its first prompt,
// deadlocked writing a diagnostic, and read nothing typed at it afterwards.
//
// Asserted as the depth after a second round against the depth after the
// first, which is the growth rather than any particular number: what has to be
// true is that a session can run handlers all day.
func TestHandlersThatStartBackgroundJobsDoNotStackGuardsOnTheStream(t *testing.T) {
	read := armedForever(t)
	p := &descriptorProbe{}
	var runner *interp.Runner
	s := newSessionWith(t, func(cfg *Shell) {
		runner = cfg.Runner
		cfg.WatchedDescriptors = p.watched
		cfg.DescriptorReady = p.ready
	})
	job, err := syntax.Parse(": &", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	p.answer = func(in Line) (Line, bool) {
		_, _ = runner.Run(context.Background(), job)
		p.disarm()
		return in, false
	}

	s.prompt++
	waitFor(t, s.screen, "[1]", "the first prompt")

	round := func() int {
		t.Helper()
		was := len(p.called())
		p.arm(int(read.Fd()))
		// A keystroke, because the editor only waits on descriptors while it
		// is waiting for one: an armed descriptor is offered from inside that
		// wait and not from outside it.
		if _, werr := s.control.WriteString("x"); werr != nil {
			t.Fatal(werr)
		}
		waitUntil(t, "a descriptor handler", func() bool { return len(p.called()) > was })
		settled(t, p)
		return layers(runner.Stdout)
	}
	first := round()
	second := round()
	// Fatal rather than an error: without the fix the stream after this point
	// deadlocks the first thing that writes to it, and a test that carried on
	// would hang rather than fail.
	if second != first {
		t.Fatalf("the stream is %d layers deep after a second handler and %d after the first;"+
			" each round is stacking a guard over the last one's", second, first)
	}

	// And the session is still there, which is the fault as a person meets it.
	if _, werr := s.control.WriteString("\x15echo still-here\n"); werr != nil {
		t.Fatal(werr)
	}
	waitFor(t, s.ran, "still-here", "a command typed after two handlers that started background jobs")
	s.end()
}
