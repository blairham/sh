// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// The kept placeholder, asserted against the kernel: which processes were this
// test's children *before* a body asked, and which one it was handed. See
// procanchorspare.go for why the start is moved and not removed.

// spareArgv is a placeholder for these tests: `cat` blocks on its standard
// input and exits at end of file, which is the whole contract a placeholder
// has. The interp_test file of the same name argues why it is not a shell.
func spareArgv(t *testing.T) []string {
	t.Helper()
	path, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("no placeholder program on this machine: %v", err)
	}
	return []string{path}
}

// keptSpare waits until a placeholder has been started ahead of the next body,
// and answers with its group. Peeked at and put back, so the next take still
// finds it.
func keptSpare(t *testing.T, s *anchorSpares) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		next := s.next
		s.mu.Unlock()
		if next != nil {
			h := <-next
			next <- h
			if h == nil {
				t.Fatal("the placeholder kept ahead failed to start")
			}
			return h.pgid
		}
		if time.Now().After(deadline) {
			t.Fatal("no placeholder was started ahead of the next body")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestTheSecondBodyIsHandedAPlaceholderAlreadyRunning is #6420's lever: the
// first body to ask pays for a start, and the next one is answered with a
// process that existed before it asked — so nothing is forked between the
// question and the answer.
func TestTheSecondBodyIsHandedAPlaceholderAlreadyRunning(t *testing.T) {
	argv := spareArgv(t)
	s := &anchorSpares{}
	t.Cleanup(s.stop)

	first, err := s.take(argv)
	if err != nil {
		t.Fatalf("first take: %v", err)
	}
	t.Cleanup(first.release)
	kept := keptSpare(t, s)
	// Running before the body asks, by the kernel's account and not only by
	// this file's bookkeeping.
	if err := syscall.Kill(kept, 0); err != nil {
		t.Fatalf("the kept placeholder %d is not running: %v", kept, err)
	}

	second, err := s.take(argv)
	if err != nil {
		t.Fatalf("second take: %v", err)
	}
	t.Cleanup(second.release)
	if second.pgid == first.pgid {
		t.Fatalf("both bodies were given group %d; a group is one body's", first.pgid)
	}
	if second.pgid != kept {
		t.Errorf("the second body was given %d, a placeholder started when it asked; want %d, the one kept ahead", second.pgid, kept)
	}
	// And it is still what a placeholder must be: the leader of a group of
	// its own, alive until its pipe is closed.
	if got, err := syscall.Getpgid(second.pgid); err != nil || got != second.pgid {
		t.Errorf("Getpgid(%d) = %d, %v; want it to lead its own group", second.pgid, got, err)
	}
}

// TestAPlaceholderForAnotherCommandIsNotHandedOut: an embedder that changed
// ProcessAnchor asked for a different program, and the kept one is ended
// rather than given to a body that would be asking for the new one.
func TestAPlaceholderForAnotherCommandIsNotHandedOut(t *testing.T) {
	argv := spareArgv(t)
	s := &anchorSpares{}
	t.Cleanup(s.stop)
	first, err := s.take(argv)
	if err != nil {
		t.Fatalf("first take: %v", err)
	}
	t.Cleanup(first.release)
	kept := keptSpare(t, s)

	other := append([]string{"/bin/sh", "-c", `exec "$0"`}, argv...)
	second, err := s.take(other)
	if err != nil {
		t.Fatalf("second take: %v", err)
	}
	t.Cleanup(second.release)
	if second.pgid == kept {
		t.Errorf("a body asking for %q was given %d, the spare kept for %q", other, kept, argv)
	}
}

// TestStoppingEndsTheKeptPlaceholder is the lifetime: a shell's CleanUp ends
// the placeholder it was keeping, rather than leaving it for the end of the
// program — which, for an embedder, may be never.
func TestStoppingEndsTheKeptPlaceholder(t *testing.T) {
	argv := spareArgv(t)
	s := &anchorSpares{}
	first, err := s.take(argv)
	if err != nil {
		t.Fatalf("first take: %v", err)
	}
	first.release()
	kept := keptSpare(t, s)
	if err := syscall.Kill(-kept, 0); err != nil {
		t.Fatalf("the kept placeholder's group %d is not there before the stop: %v", kept, err)
	}
	s.stop()
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(-kept, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("process group %d is still there after the shell stopped keeping it", kept)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// And nothing is started after the stop, which a body still running
	// after its shell ended would otherwise do.
	late, err := s.take(argv)
	if err != nil {
		t.Fatalf("late take: %v", err)
	}
	late.release()
	time.Sleep(3 * spareDelay)
	s.mu.Lock()
	next := s.next
	s.mu.Unlock()
	if next != nil {
		t.Error("a placeholder was started for a shell that had already ended")
	}
}
