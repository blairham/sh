// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// `zsystem`, measured against zsh 5.9.2 (2026-09-10) with `zsh -f`.
//
// **A lock is a claim about two processes and cannot be tested in one.** A
// `zsystem flock` that opened the file, answered 0 and locked nothing passes
// every single-process case anybody would think to write: the descriptor is
// there, `-f` names it, the unlock succeeds. So the cases below run a second
// program — a child that asks the kernel for the same lock, without going
// through this shell — and read what it was told. That is the mutation this
// file exists to kill, and it is the exact shape #1737 was filed for.

// lockProbeMode is how the child process is told which half of the job it is.
const lockProbeMode = "SH_FLOCK_PROBE"

// lockProbeExcluded is the status the probing child exits with when it could
// not take the lock. Not 1: a Go test binary exits 1 when a test fails, and a
// child that failed for its own reasons would otherwise read as evidence.
const lockProbeExcluded = 7

// TestFlockProbeHelperProcess is **not a test**. It is the second process the
// cases below need, re-entered through this binary so that the file has no
// build step and no dependency on what is installed on the runner.
//
// Two modes. `probe` asks the kernel for a write lock once and says so in its
// status; `hold` takes one, tells the shell it has it by creating a file
// beside the lock, and keeps it for as long as it was told to. Neither goes
// anywhere near this package's own code, which is what makes them evidence:
// a child using `zsystem` to check `zsystem` would agree with any mistake it
// was making.
func TestFlockProbeHelperProcess(t *testing.T) {
	mode := os.Getenv(lockProbeMode)
	if mode == "" {
		t.Skip("not the helper process")
	}
	path := os.Getenv("SH_FLOCK_PATH")
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: %v\n", err)
		os.Exit(9)
	}
	lock := syscall.Flock_t{Type: syscall.F_WRLCK}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock); err != nil {
		os.Exit(lockProbeExcluded)
	}
	if mode == "probe" {
		os.Exit(0)
	}
	held, err := strconv.Atoi(os.Getenv("SH_FLOCK_HOLD"))
	if err != nil {
		held = 2000
	}
	// The shell finds out that the lock is taken by *reading* this, through
	// a process substitution, rather than by polling for a file: a poll needs
	// a `sleep` and this snippet's PATH is a scratch directory with two
	// wrapper scripts on it, and a poll with no sleep in it spins.
	fmt.Println("held")
	os.Stdout.Close()
	time.Sleep(time.Duration(held) * time.Millisecond)
	os.Exit(0)
}

// lockHelpers writes the two wrapper scripts a snippet calls the helper by,
// and returns the directory they are on the PATH of.
//
// A wrapper rather than the binary itself, because the snippet has to be
// readable: `lockprobe $f` is what the case is about and
// `/very/long/path.test -test.run=… probe` is not.
func lockHelpers(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("finding this test binary: %v", err)
	}
	for _, mode := range []string{"probe", "hold"} {
		script := "#!/bin/sh\n" +
			"SH_FLOCK_PATH=\"$1\" SH_FLOCK_HOLD=\"$2\" " + lockProbeMode + "=" + mode +
			" exec " + strconv.Quote(self) + " -test.run='^TestFlockProbeHelperProcess$'\n"
		name := filepath.Join(dir, "lock"+mode)
		if err := os.WriteFile(name, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// **The lock excludes another process, and stops excluding it when unlocked.**
//
// Both halves in one case, because either alone passes on a shell that has the
// other direction wrong: a `flock` that locked and never released looks
// exactly like a working one until something needs the file back, and a
// `flock` that never locked at all looks like a working *unlock*.
//
// The child is asked twice with nothing between the two but `zsystem flock
// -u`, so the only thing that can have changed the answer is the unlock.
func TestFlockExcludesAnotherProcessUntilItIsUnlocked(t *testing.T) {
	dir := lockHelpers(t)
	path := filepath.Join(dir, "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	systemDeadline(t, "zsystem flock against a second process", func() {
		out, st := runZsh(t, dir, `zsystem flock -f h `+path+`
print -r -- "took=$?"
lockprobe `+path+`
print -r -- "while-held=$?"
zsystem flock -u $h
print -r -- "released=$?"
lockprobe `+path+`
print -r -- "after=$?"`)
		want := fmt.Sprintf("took=0\nwhile-held=%d\nreleased=0\nafter=0\n", lockProbeExcluded)
		if out != want || st != 0 {
			t.Errorf("flock against a child = %q (status %d), want %q", out, st, want)
		}
	})
}

// **A read lock lets another reader in and keeps a writer out.**
//
// The half that says the `-r` letter reaches the kernel rather than being
// accepted and dropped. A shell that took an exclusive lock for `-r` passes
// every case above.
func TestAReadLockSharesAndStillExcludesAWriter(t *testing.T) {
	dir := lockHelpers(t)
	path := filepath.Join(dir, "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	systemDeadline(t, "zsystem flock -r", func() {
		out, st := runZsh(t, dir, `zsystem flock -r -f h `+path+`
print -r -- "took=$?"
lockprobe `+path+`
print -r -- "writer=$?"`)
		want := fmt.Sprintf("took=0\nwriter=%d\n", lockProbeExcluded)
		if out != want || st != 0 {
			t.Errorf("read lock = %q (status %d), want %q", out, st, want)
		}
	})
	systemDeadline(t, "a second reader", func() {
		out, st := runZsh(t, dir, `zsystem flock -r -f a `+path+`
zsystem flock -r -t 0 -f b `+path+`
print -r -- "second-reader=$?"`)
		if want := "second-reader=0\n"; out != want || st != 0 {
			t.Errorf("two read locks = %q (status %d), want %q", out, st, want)
		}
	})
}

// **The three statuses of a lock somebody else is holding, under real
// contention**, which is the only place they can be told apart:
//
//	-t 0     1, and it says so
//	-t n     2 after n seconds, and says **nothing**
//	no -t    0, once the holder lets go
//
// The last of those is the one that proves the wait is a wait. It cannot
// return before the holder's own deadline, so the elapsed time is read here
// rather than only the status: a `flock` that answered 0 immediately would
// have the same status and no lock.
func TestFlockUnderContentionWaitsAndSaysSoThreeWays(t *testing.T) {
	dir := lockHelpers(t)
	path := filepath.Join(dir, "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const holdFor = 1500 * time.Millisecond
	started := time.Now()
	systemDeadline(t, "zsystem flock under contention", func() {
		out, st, errs := runZshSplit(t, dir, `sysopen -r -u fd <(lockhold `+path+` `+
			strconv.Itoa(int(holdFor/time.Millisecond))+`)
sysread -i $fd -t 5 v
print -r -- "holder=$? [$v]"
zsystem flock -t 0 `+path+`
print -r -- "asked=$?"
zsystem flock -t 0.2 `+path+`
print -r -- "waited=$?"
zsystem flock `+path+`
print -r -- "blocked=$?"`)
		want := "holder=0 [held\n]\nasked=1\nwaited=2\nblocked=0\n"
		if out != want || st != 0 {
			t.Errorf("contention = %q (status %d), want %q", out, st, want)
		}
		// The `-t 0` refusal speaks and the timed one does not, which is what
		// keeps a busy lock off a person's terminal in a prompt.
		if got := strings.Count(errs, "failed to lock file"); got != 1 {
			t.Errorf("stderr = %q, want exactly one `failed to lock file`", errs)
		}
	})
	// The blocking form cannot have come back before the holder let go. Two
	// thirds of the hold rather than all of it, because the shell starts the
	// holder and the clock starts here.
	if waited := time.Since(started); waited < holdFor*2/3 {
		t.Errorf("the whole run took %v, want at least %v — the blocking flock did not block", waited, holdFor*2/3)
	}
}
