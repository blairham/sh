// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package sigbook_test

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/sigbook"
)

// raise sends sig to this process and reports whether c heard it within the
// wait.
func raise(t *testing.T, c chan os.Signal, sig syscall.Signal, wait time.Duration) bool {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case <-c:
		return true
	case <-time.After(wait):
		return false
	}
}

// handBackAfter leaves sig handled rather than ignored once the test is done,
// so nothing after it inherits the ignore.
func handBackAfter(t *testing.T, sig syscall.Signal) {
	t.Cleanup(func() { signal.Notify(make(chan os.Signal, 1), sig) })
}

// An ignore takes a subscription away, and PutBack gives it back. The first
// half is the control: it is what says the second half is PutBack's doing and
// not a subscription that was never lost.
func TestPutBackGivesBackWhatAnIgnoreTook(t *testing.T) {
	handBackAfter(t, syscall.SIGUSR2)
	c := make(chan os.Signal, 1)
	sigbook.Notify(c, syscall.SIGUSR2)
	t.Cleanup(func() { sigbook.Stop(c) })

	signal.Ignore(syscall.SIGUSR2)
	if raise(t, c, syscall.SIGUSR2, 200*time.Millisecond) {
		t.Fatal("the channel heard SIGUSR2 while the process was ignoring it, so nothing below can show it was given back")
	}
	sigbook.PutBack(syscall.SIGUSR2)
	if !raise(t, c, syscall.SIGUSR2, 5*time.Second) {
		t.Error("PutBack did not subscribe the channel again")
	}
}

// What a channel let go of, it is not handed back: a Stop, and a Forget for a
// signal its owner has decided to ignore on the process.
func TestPutBackLeavesAloneWhatWasLetGo(t *testing.T) {
	handBackAfter(t, syscall.SIGUSR2)
	for _, tc := range []struct {
		name   string
		letGo  func(c chan os.Signal)
		stayed bool
	}{
		{"kept", func(chan os.Signal) {}, false},
		{"stopped", func(c chan os.Signal) { sigbook.Stop(c) }, true},
		{"forgotten", func(c chan os.Signal) { sigbook.Forget(c, syscall.SIGUSR2) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := make(chan os.Signal, 1)
			sigbook.Notify(c, syscall.SIGUSR2)
			t.Cleanup(func() { sigbook.Stop(c) })
			tc.letGo(c)
			signal.Ignore(syscall.SIGUSR2)
			sigbook.PutBack(syscall.SIGUSR2)
			if got := signal.Ignored(syscall.SIGUSR2); got != tc.stayed {
				t.Errorf("SIGUSR2 ignored after PutBack = %v, want %v", got, tc.stayed)
			}
		})
	}
}
