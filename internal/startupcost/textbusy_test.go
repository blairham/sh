// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/startupcost"
)

// An exec the kernel refuses because the file is still open for writing is
// not a measurement, and it is not a shell that will not do the work either
// (#1623).
//
// The package writes an executable and hands its path straight to something
// that execs it — slowWrapper does, and any wrapper a future subject needs
// would too. `os.WriteFile` closes its own descriptor before it returns, so
// the race is not with itself: it is golang/go#22315. Go opens files
// `O_CLOEXEC`, so a child forked anywhere else in the process loses the
// descriptor *at its exec* — but it holds it, open for writing, for the whole
// window between its fork and that exec. This package forks constantly and its
// tests are `t.Parallel`, so that window is open all the time, and an exec of
// the same file inside it is `ETXTBSY`.
//
// It failed `main` this way — run 34407261156 — and the merge queue's runs are
// the ones under the most concurrent load.
//
// Linux enforces the rule; macOS was measured not to, executing a file with a
// writer still open on it without complaint, which is why this is skipped
// there and why only the ubuntu job ever saw the failure.
func writerHeldOpen(t *testing.T) (string, *os.File) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("%s does not refuse to exec a file that is open for writing, so there is nothing to hold off here", runtime.GOOS)
	}
	path := filepath.Join(t.TempDir(), "subject")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf answered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return path, f
}

// TestAnExecRefusedWhileTheFileIsBeingWrittenIsTriedAgain is the fix: the
// window closes by itself the moment the forked child reaches its own exec, so
// the answer is to ask again rather than to give up.
func TestAnExecRefusedWhileTheFileIsBeingWrittenIsTriedAgain(t *testing.T) {
	path, writer := writerHeldOpen(t)

	// Closed from another goroutine partway through, which is the shape of
	// the real thing: the descriptor goes away on its own and nothing the
	// caller does makes it go away sooner.
	var once sync.Once
	closed := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		once.Do(func() { _ = writer.Close() })
		close(closed)
	}()
	defer func() { <-closed }()

	_, out, err := startupcost.RunProgram(startupcost.Subject{Name: "subject", Path: path}, "")
	if err != nil {
		t.Fatalf("the subject was never run: %v", err)
	}
	if out != "answered" {
		t.Errorf("the subject printed %q, want the answer of the program it was given", out)
	}
}

// TestAnExecRefusedForGoodIsStillARefusal is the other half, and it is the one
// that keeps the retry from becoming a way of not noticing.
//
// A writer that never lets go is a real refusal. It has to come back as one,
// carrying the kernel's own error so that the failure names what happened —
// and it has to come back at all, rather than being tried until the test
// times out.
func TestAnExecRefusedForGoodIsStillARefusal(t *testing.T) {
	path, _ := writerHeldOpen(t)

	start := time.Now()
	_, _, err := startupcost.RunProgram(startupcost.Subject{Name: "subject", Path: path}, "")
	took := time.Since(start)

	if err == nil {
		t.Fatal("a file nothing will ever stop writing was reported as measured")
	}
	if !errors.Is(err, syscall.ETXTBSY) {
		t.Errorf("the refusal came back as %v, which does not carry the kernel's reason for it", err)
	}
	// Bounded, and generously: what is being checked is that it gives up at
	// all, not the exact number of tries.
	if took > 10*time.Second {
		t.Errorf("it kept trying for %v, so a real refusal is a hang rather than a failure", took)
	}
}
