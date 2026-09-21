// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"sync"
	"testing"
)

// The wake itself: what it costs a publisher to be chatty, and what happens
// to one that outlives its session.

func TestAWakeCostsOneByteHoweverOftenItIsPublished(t *testing.T) {
	t.Parallel()
	// A publisher that fires a thousand times while a command runs must not
	// fill a pipe nobody is reading, and the editor has nothing to do with
	// the thousand that it would not do with the one: the prompt is
	// re-rendered from scratch either way.
	signal := newPromptSignal()
	if signal == nil {
		t.Skip("no pipe on this system")
	}
	t.Cleanup(signal.close)

	var wg sync.WaitGroup
	for range 1000 {
		wg.Add(1)
		go func() { defer wg.Done(); signal.publish() }()
	}
	wg.Wait()

	if !signal.pending {
		t.Fatal("a thousand publishes left nothing to wake on")
	}
	signal.drain()
	if signal.pending {
		t.Error("draining left the wake pending")
	}
	// And the second drain does not block, which is the property that keeps a
	// descriptor reported readable for reasons of its own from hanging the
	// editor on a read of an empty pipe.
	signal.drain()
}

func TestAPublisherThatOutlivesItsSessionIsHarmless(t *testing.T) {
	t.Parallel()
	signal := newPromptSignal()
	if signal == nil {
		t.Skip("no pipe on this system")
	}
	signal.close()
	// A scanner that finishes after the shell has gone is an ordinary outcome
	// and not an error. Neither of these may panic or write to a closed
	// descriptor.
	signal.publish()
	signal.drain()
	if fd := signal.fd(); fd >= 0 {
		t.Errorf("a closed wake still offers descriptor %d", fd)
	}
}

func TestASessionWiresAPublishingThemeAndTakesItBack(t *testing.T) {
	t.Parallel()
	theme := &Theme{}
	s := Shell{Theme: theme}
	signal, stop := s.publishing()
	if signal == nil {
		t.Skip("no pipe on this system")
	}

	theme.Publish()
	if !signal.pending {
		t.Error("a theme's publish did not reach the session's wake")
	}

	stop()
	// Nothing is listening now, so publishing is a no-op rather than a write
	// to a descriptor the session closed.
	theme.Publish()
}

func TestASessionWithNoPublishingThemeOpensNothing(t *testing.T) {
	t.Parallel()
	// The cost of the seam to a session that never has an asynchronous
	// segment is the thing to keep at zero: no pipe, no descriptor in the
	// wait, and no extra call in front of a keystroke.
	for _, theme := range []PromptTheme{
		nil,
		PromptThemeFunc(func(PromptInfo) (ThemedPrompt, bool) { return ThemedPrompt{}, false }),
	} {
		signal, stop := Shell{Theme: theme}.publishing()
		if signal != nil {
			t.Errorf("a theme that does not publish opened a wake")
			signal.close()
		}
		stop()
	}
}
