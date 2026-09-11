// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

// A layer that passes bytes through and can say what it is over — the shape
// the editor's line discipline and the block store's capture both have.
type seeThrough struct{ w io.Writer }

func (s *seeThrough) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s *seeThrough) Unwrap() io.Writer           { return s.w }

// A layer that passes bytes through and says nothing. Not wrong, opaque: the
// chain stops being readable here. It is in this file so that the difference
// between the two is asserted rather than assumed.
type opaqueLayer struct{ w io.Writer }

func (o *opaqueLayer) Write(p []byte) (int, error) { return o.w.Write(p) }

// TestAGuardUnderAnotherLayerIsNotTakenTwice is #2069, at the mutex.
//
// A guard goes on the shell's stream, something else wraps the result and
// leaves the wrapper there, and then a second construct asks for a guard. If
// the second one goes on, the two share a lock — the lock belongs to the
// stream, deliberately — and sync.Mutex is not reentrant, so the first write
// through the chain takes it twice on one goroutine and parks for ever.
//
// Measured as a write that comes back rather than as a shape, because the
// shape is the mechanism and the write is the fault. The write is on a
// goroutine of its own so that this fails in two seconds rather than hanging
// the package: a deadlock has nothing to interrupt it.
func TestAGuardUnderAnotherLayerIsNotTakenTwice(t *testing.T) {
	var mu sync.Mutex
	var buf bytes.Buffer

	inner := lockWriter(&mu, &buf)
	if _, ok := inner.(*lockedWriter); !ok {
		t.Fatalf("the first guard is %T, want one to have gone on", inner)
	}
	outer := lockWriter(&mu, &seeThrough{w: inner})
	if _, ok := outer.(*lockedWriter); ok {
		t.Errorf("a second guard went on over the first, which is one mutex twice")
	}

	done := make(chan error, 1)
	go func() {
		_, err := outer.Write([]byte("through"))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the write never came back: the chain takes one lock twice on one goroutine")
	}
	if got := buf.String(); got != "through" {
		t.Errorf("the stream holds %q, want %q", got, "through")
	}
}

// And the two halves of the bargain that answer makes.
//
// A guard is still put on where there is none under the layer, because the
// question is whether *this* lock is already taken and not whether anything is
// — and it is still put on where a layer cannot say what it is over, because
// an opaque layer leaves the chain unreadable and the guard that is there to
// be found cannot be.
func TestWhatTheChainIsAskedAndWhatItCannotSee(t *testing.T) {
	var mu, other sync.Mutex
	var buf bytes.Buffer

	t.Run("a different lock under the layer is not this one", func(t *testing.T) {
		w := lockWriter(&mu, &seeThrough{w: lockWriter(&other, &buf)})
		if _, ok := w.(*lockedWriter); !ok {
			t.Errorf("got %T, want a guard: the lock under it is somebody else's", w)
		}
	})
	t.Run("an opaque layer hides the guard beneath it", func(t *testing.T) {
		w := lockWriter(&mu, &opaqueLayer{w: lockWriter(&mu, &buf)})
		if _, ok := w.(*lockedWriter); !ok {
			t.Errorf("got %T, want a guard: nothing here can see past the layer", w)
		}
	})
	t.Run("a guard that is already outermost is still handed back", func(t *testing.T) {
		one := lockWriter(&mu, &buf)
		if again := lockWriter(&mu, one); again != one {
			t.Errorf("got %p, want the guard that is already there (%p)", again, one)
		}
	})
}
