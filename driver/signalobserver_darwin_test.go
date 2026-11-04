// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// The kqueue that hears PROF and PIPE for a shell is a descriptor a script
// must not be able to name, as the runtime's own are not (#799): `exec 5>f`
// placed onto it would take the observer away, or worse. So it sits above
// every number a script can name, a placement onto it is refused, and it is
// closed again when the run that opened it takes its watch down.
func TestTheSignalObserverHoldsNoDescriptorAScriptCanName(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = r, io.Discard, io.Discard
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh", "-c", "read x"}) }()

	deadline := time.Now().Add(deathDeadline)
	fd := 0
	for fd == 0 {
		if time.Now().After(deadline) {
			_ = w.Close()
			t.Fatal("no observer was started for a run that owns its process")
		}
		time.Sleep(5 * time.Millisecond)
		fd = driver.ObserverDescriptorForTest()
	}
	if fd <= nameableCeiling {
		t.Errorf("the observer holds descriptor %d, which a script can name", fd)
	}
	if !driver.HeldByTheRuntimeForTest(fd) {
		t.Errorf("a script may place a file onto the observer's descriptor %d", fd)
	}

	_ = w.Close()
	select {
	case <-done:
	case <-time.After(deathDeadline):
		t.Fatal("the run never ended")
	}
	for driver.ObserverDescriptorForTest() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the observer still holds descriptor %d after its run took the watch down", fd)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if driver.HeldByTheRuntimeForTest(fd) {
		t.Errorf("descriptor %d is still refused to scripts after the observer let it go", fd)
	}
}
