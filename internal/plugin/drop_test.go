// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package rather than plugin_test, and for one reason: the bounded buffer
// is not observable from outside the host. A drop is invisible by design — it
// costs a record and complains to nobody — so the only place the property that
// makes it honest can be graded is from inside, where the channel is.
//
// internal/event's own tests are in-package for the sibling reason: the clock
// a stream stamps records with is deliberately not the caller's.
package plugin

import (
	"context"
	"sync"
	"testing"

	"github.com/blairham/sh/interp"
)

// probe is a record with something in it, numbered by the caller's loop rather
// than by anything under test.
func probe(i int) interp.Event {
	return interp.Event{
		Kind:   interp.EventAccess,
		Action: interp.Action{Kind: interp.ActionStat, Path: "/srv/x"},
		Line:   i,
	}
}

// The property that makes a lossy stream honest rather than a lying one.
//
// A record is numbered before the send is attempted, so a record that is
// dropped has still spent its number and the consumer sees a gap exactly as
// wide as what it lost. That is the event schema's own promise about seq — "a
// consumer that has records 1..40 and then 42 knows it lost one" — and it is
// the whole reason a bounded buffer that drops is an acceptable answer to "an
// observer must not be able to stop the shell".
//
// Numbering on the way out instead would produce a stream that is contiguous
// and incomplete, which is worse than a stream with holes in it: the consumer
// would have no way to know, and the difference between lossy and lying is
// exactly this line.
//
// No feed is started, which is what makes the drop happen at a place the test
// chose rather than at a place the scheduler chose.
func TestADroppedRecordStillSpendsItsSequenceNumber(t *testing.T) {
	t.Parallel()
	o := newObserver(nil)
	ctx := context.Background()
	const lost = 5
	for i := range observerBuffer + lost {
		o.Emit(ctx, probe(i))
	}
	if got := len(o.recs); got != observerBuffer {
		t.Fatalf("the buffer holds %d records, want %d", got, observerBuffer)
	}
	for want := int64(1); want <= observerBuffer; want++ {
		if got := (<-o.recs).Seq; got != want {
			t.Fatalf("record %d has seq %d: what is buffered is not what was numbered", want, got)
		}
	}
	// The next one to get through says how many went missing, which is the
	// entire contract with the consumer.
	o.Emit(ctx, probe(0))
	got := <-o.recs
	if want := int64(observerBuffer + lost + 1); got.Seq != want {
		t.Errorf("seq after the drops = %d, want %d — the gap has to be as wide as the loss", got.Seq, want)
	}
}

// Emit does not block, which is the whole of the observer role's promise to
// the shell. Nothing drains here, so every send past the buffer's capacity
// takes the default branch; a send without one would park the interpreter's
// own goroutine on a plugin.
func TestEmitDoesNotWaitForRoom(t *testing.T) {
	t.Parallel()
	o := newObserver(nil)
	ctx := context.Background()
	// Twenty times the buffer, on the goroutine the test is on: if any of them
	// waits, this never returns and the timeout says so.
	for i := range observerBuffer * 20 {
		o.Emit(ctx, probe(i))
	}
}

// Numbers reach the consumer in the order they were handed out.
//
// seq is documented as a total order of emission, so a stream where 7 arrives
// before 6 would be a stream whose own gap detection cannot be trusted — a
// consumer holding 1..5 and then 7 would call 6 lost while it was still on its
// way. Taking the number and making the send one operation is what holds it,
// and holding a lock across the send is free precisely because the send cannot
// wait.
func TestTheNumbersArriveInTheOrderTheyWereGivenOut(t *testing.T) {
	t.Parallel()
	o := newObserver(nil)
	ctx := context.Background()
	const emitters, each = 8, 50
	var wg sync.WaitGroup
	wg.Add(emitters)
	for range emitters {
		go func() {
			defer wg.Done()
			for i := range each {
				o.Emit(ctx, probe(i))
			}
		}()
	}
	wg.Wait()
	if got := len(o.recs); got != emitters*each {
		t.Fatalf("buffered %d records, want %d: something was dropped where there was room", got, emitters*each)
	}
	last := int64(0)
	for range emitters * each {
		got := (<-o.recs).Seq
		if got <= last {
			t.Fatalf("seq went %d then %d: the order on the wire is not the order they were numbered", last, got)
		}
		last = got
	}
}
