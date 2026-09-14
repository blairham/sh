// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv_test

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/testenv"
)

// TestAFreshlyWrittenExecutableRuns is the measurement WriteExecutable exists
// for, kept as a test rather than written down as a claim.
//
// The load is manufactured rather than waited for: eight goroutines starting
// external commands in a loop are what keep a forked-but-not-yet-exec'd child
// in the process at almost all times, which is the only thing that can hold a
// write descriptor on a file this goroutine has already closed. That is the
// whole mechanism of #2787 and of #1623, and it is not reachable at all
// without something else in the process forking.
//
// **It is the mutation that makes this a gate.** Written with os.WriteFile in
// place of WriteExecutable, in a golang container on Linux, 34 of 400 rounds
// came back ETXTBSY; with the lock, 0 of 400, and 0 again on repeat runs. So a
// removed lock fails this test rather than merely making it likelier to fail —
// the number of rounds here is chosen so that a single survivor is a
// vanishingly unlikely thing to see and not a coin toss.
//
// Skipped off Linux because no other platform in the panel enforces the rule:
// macOS executes a file that still has a writer open on it without complaint,
// so the test could pass there with the guard deleted and would say nothing.
func TestAFreshlyWrittenExecutableRuns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("%s does not refuse to exec a file that is open for writing, so there is nothing here to catch", runtime.GOOS)
	}
	dir := t.TempDir()

	stop := make(chan struct{})
	var forking sync.WaitGroup
	for range 8 {
		forking.Add(1)
		go func() {
			defer forking.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = exec.Command("/bin/sh", "-c", ":").Run()
			}
		}()
	}
	defer func() {
		close(stop)
		forking.Wait()
	}()

	const rounds = 400
	var busy int
	for i := range rounds {
		path := filepath.Join(dir, fmt.Sprintf("fixture%d", i))
		if err := testenv.WriteExecutable(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		switch err := exec.Command(path).Run(); {
		case err == nil:
		case errors.Is(err, syscall.ETXTBSY):
			busy++
		default:
			t.Fatalf("running %s failed for a reason this test is not about: %v", path, err)
		}
	}
	if busy > 0 {
		t.Errorf("%d of %d freshly written fixtures could not be executed because the kernel still saw a writer on them; the write is not being held off against this process's own forks", busy, rounds)
	}
}
