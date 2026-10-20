// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// The second process `zsh/system`'s lock and `zsh/zselect`'s wait are tested
// against.
//
// **Both are claims about something outside this process and neither can be
// tested inside one.** A `zsystem flock` that opens the file, answers 0 and
// locks nothing passes every single-process case anybody would write, and a
// `zselect` that returns at once passes every case that only reads its status.
// So the cases in systemlock_test.go and zselect_test.go run a child, and read
// what the child was told rather than what this shell says it did.
//
// One helper for both files rather than one each. The two need the same three
// things — a process that competes for a lock, a process that holds one, and a
// process that writes a byte after a delay — and a second copy of the re-exec
// machinery beside the first is the shape of bug this tree has found repeatedly:
// the copy is written from the same understanding and then only one of them
// gets the fix.
//
// It is this binary re-entered rather than a program compiled by the test,
// because a build step in a test is slow, and rather than a script calling
// `sleep`, because the snippets run with a scratch directory as their whole
// PATH and what is installed on a runner is not this project's to assume.

// helperMode is the environment entry that tells the child which of the three
// jobs it is. Empty in the test process, which is how the entry point below
// knows it is not the helper.
//
// SH_TEST_-prefixed because that prefix is the module's re-execution channel:
// this suite assembles the environment it runs in from an allowlist
// (internal/testenv), so a marker outside the channel is scrubbed on the way
// into the child and the helper silently becomes a skipped test instead.
const helperMode = "SH_TEST_MODULE_HELPER"

// helperExcluded is the status the lock-probing child exits with when it could
// not take the lock. Not 1: a Go test binary exits 1 when a test fails, and a
// child that failed for its own reasons would otherwise read as evidence.
const helperExcluded = 7

// TestModuleHelperProcess is **not a test**. It is the child, re-entered
// through this binary by the wrapper scripts helperScripts writes.
//
// Three modes:
//
//	probe   ask the kernel for a write lock once, and say so in the status
//	hold    take one, say `held` on standard output, keep it for a while
//	delay   write a byte after a delay, for something to wait on
//
// None of them goes near this package's own code, which is what makes them
// evidence: a child using `zsystem` to check `zsystem` would agree with any
// mistake it was making.
func TestModuleHelperProcess(t *testing.T) {
	mode := os.Getenv(helperMode)
	if mode == "" {
		t.Skip("not the helper process")
	}
	milliseconds, err := strconv.Atoi(os.Getenv("SH_TEST_MODULE_HELPER_MS"))
	if err != nil {
		milliseconds = 1000
	}
	if mode == "delay" {
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
		fmt.Print("x")
		os.Exit(0)
	}
	path := os.Getenv("SH_TEST_MODULE_HELPER_PATH")
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: %v\n", err)
		os.Exit(9)
	}
	lock := syscall.Flock_t{Type: syscall.F_WRLCK}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock); err != nil {
		os.Exit(helperExcluded)
	}
	if mode == "probe" {
		os.Exit(0)
	}
	// The shell finds out that the lock is taken by *reading* this, through a
	// process substitution, rather than by polling for a file: a poll needs a
	// `sleep` and this snippet's PATH is a scratch directory with these
	// wrappers on it, and a poll with no sleep in it spins.
	fmt.Println("held")
	_ = os.Stdout.Close()
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	os.Exit(0)
}

// helperScripts writes the three wrappers a snippet calls the child by, and
// returns the scratch directory they are the whole PATH of.
//
// Wrappers rather than the binary itself, because the snippet has to be
// readable: `lockprobe $f` is what a case is about and
// `/very/long/path.test -test.run=… probe` is not.
func helperScripts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("finding this test binary: %v", err)
	}
	for name, mode := range map[string]string{
		"lockprobe": "probe", "lockhold": "hold", "delaywrite": "delay",
	} {
		script := "#!/bin/sh\n" +
			"SH_TEST_MODULE_HELPER_PATH=\"$1\" SH_TEST_MODULE_HELPER_MS=\"$2\" " + helperMode + "=" + mode +
			" exec " + strconv.Quote(self) + " -test.run='^TestModuleHelperProcess$'\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
