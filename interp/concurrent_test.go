// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"sync"
	"testing"
	"time"
)

// Nobody guarding means a plain goroutine, which is the library's answer: a
// panic on one of these ends the process, because that is what interp owes a
// caller whose invariant it has just found broken.
//
// Asserted by running rather than by reading the field, because "nil means go"
// is the half a later refactor could quietly turn into "nil means nothing runs
// at all" — and a background job that never starts is a shell that never
// returns.
func TestWorkRunsWhenNobodyIsGuarding(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	ran := make(chan struct{})
	r.spawn(func() { close(ran) }, func() {})
	select {
	case <-ran:
	case <-time.After(10 * time.Second):
		t.Fatal("the work never ran — a shell with no guard runs one anyway")
	}
}

// A guard is handed the work and is called on the new goroutine.
//
// The second half is the part worth pinning. A hook that could decide *where*
// the work ran could run it on the caller's goroutine, and the shell waits for
// a background job to report its process before it goes on — so a hook meaning
// well would deadlock the shell it was installed to protect. The goroutine is
// started here and the hook is called on it, which takes that away.
func TestAGuardIsCalledOnTheNewGoroutineWithTheWork(t *testing.T) {
	var mu sync.Mutex
	var calls int
	var sameGoroutine bool
	caller := make(chan struct{})

	r := newTestRunner(t, &Runner{})
	r.GuardConcurrent = func(work func(), _ io.Writer) {
		mu.Lock()
		calls++
		mu.Unlock()
		// The caller is blocked below until this returns. If this were
		// running on the caller's goroutine, that block would never end and
		// the test would hang rather than fail — so the wait has a deadline
		// of its own and reports which it was.
		select {
		case <-caller:
		case <-time.After(10 * time.Second):
			mu.Lock()
			sameGoroutine = true
			mu.Unlock()
		}
		work()
	}

	done := make(chan struct{})
	r.spawn(func() { close(done) }, func() {})
	close(caller)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the work never ran")
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("the guard was called %d times, want once per goroutine", calls)
	}
	if sameGoroutine {
		t.Error("the guard ran on the goroutine that spawned the work, not on the new one")
	}
}

// Nothing waiting on one of these goroutines is released until the guard has
// had its say.
//
// The order is the part that makes a report readable. Whatever was waiting —
// a `wait`, the element downstream, the shell reading a coprocess — carries
// straight on the moment it is let go, so a handover that ran first would let
// the next line of the script interleave with the report, and would let a
// caller read what the shell wrote and find the report missing from it. It is
// also why the handover is an argument rather than a `defer` inside the work:
// a defer runs while the panic is still unwinding, which is before any recover.
func TestNothingIsReleasedUntilTheGuardHasReported(t *testing.T) {
	var mu sync.Mutex
	var order []string
	note := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	r := newTestRunner(t, &Runner{})
	r.GuardConcurrent = func(work func(), _ io.Writer) {
		defer func() {
			_ = recover()
			note("reported")
		}()
		work()
	}

	released := make(chan struct{})
	r.spawn(
		func() {
			note("worked")
			panic("an invariant broke")
		},
		func() {
			note("handed over")
			close(released)
		},
	)
	select {
	case <-released:
	case <-time.After(10 * time.Second):
		t.Fatal("nothing was handed over — a panic left the shell waiting for it")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"worked", "reported", "handed over"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}
