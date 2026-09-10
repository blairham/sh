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
	"testing"
	"time"
)

// `zsystem`, measured against zsh 5.9.2 (2026-09-10) with `zsh -f`.
//
// **A lock is a claim about two processes and cannot be tested in one.** A
// `zsystem flock` that opened the file, answered 0 and locked nothing passes
// every single-process case anybody would think to write: the descriptor is
// there, `-f` names it, the unlock succeeds. So the cases below run a second
// program — see modulehelper_test.go — that asks the kernel for the same lock
// without going through this shell, and read what it was told. That is the
// mutation this file exists to kill, and it is the exact shape #1737 was filed
// for.

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
	dir := helperScripts(t)
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
		want := fmt.Sprintf("took=0\nwhile-held=%d\nreleased=0\nafter=0\n", helperExcluded)
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
	dir := helperScripts(t)
	path := filepath.Join(dir, "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	systemDeadline(t, "zsystem flock -r", func() {
		out, st := runZsh(t, dir, `zsystem flock -r -f h `+path+`
print -r -- "took=$?"
lockprobe `+path+`
print -r -- "writer=$?"`)
		want := fmt.Sprintf("took=0\nwriter=%d\n", helperExcluded)
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
//	-t n     0, when n is long enough for the holder to let go
//
// The third is the one that proves a timed wait is a wait. Without it the case
// passes on a shell whose `-t` asks once and gives up, because asking once and
// waiting in vain both answer 2 — measured as a surviving mutant, which is how
// the line got here.
func TestFlockUnderContentionWaitsAndSaysSoThreeWays(t *testing.T) {
	dir := helperScripts(t)
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
zsystem flock -t 20 `+path+`
print -r -- "outlasted=$?"`)
		want := "holder=0 [held\n]\nasked=1\nwaited=2\noutlasted=0\n"
		if out != want || st != 0 {
			t.Errorf("contention = %q (status %d), want %q", out, st, want)
		}
		// The `-t 0` refusal speaks and the timed one does not, which is what
		// keeps a busy lock off a person's terminal in a prompt.
		if got := strings.Count(errs, "failed to lock file"); got != 1 {
			t.Errorf("stderr = %q, want exactly one `failed to lock file`", errs)
		}
	})
	// The `-t 20` cannot have come back before the holder let go. Two thirds
	// of the hold rather than all of it, because the shell starts the holder
	// and the clock starts here.
	if waited := time.Since(started); waited < holdFor*2/3 {
		t.Errorf("the whole run took %v, want at least %v — the timed flock did not wait", waited, holdFor*2/3)
	}
}

// **A `zsystem flock` with no `-t` blocks until the holder lets go**, which is
// its own case because it is its own system call: a timed wait is this shell
// asking over and over and a plain one is the kernel holding on to the
// request. The elapsed time is the assertion — the status is 0 either way, and
// a shell that answered 0 without a lock would answer it instantly.
func TestFlockWithNoTimeoutBlocksUntilTheHolderLetsGo(t *testing.T) {
	dir := helperScripts(t)
	path := filepath.Join(dir, "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const holdFor = 1200 * time.Millisecond
	started := time.Now()
	systemDeadline(t, "zsystem flock with no timeout", func() {
		out, st := runZsh(t, dir, `sysopen -r -u fd <(lockhold `+path+` `+
			strconv.Itoa(int(holdFor/time.Millisecond))+`)
sysread -i $fd -t 5 v
print -r -- "holder=$? [$v]"
zsystem flock `+path+`
print -r -- "blocked=$?"`)
		want := "holder=0 [held\n]\nblocked=0\n"
		if out != want || st != 0 {
			t.Errorf("blocking flock = %q (status %d), want %q", out, st, want)
		}
	})
	if waited := time.Since(started); waited < holdFor*2/3 {
		t.Errorf("the whole run took %v, want at least %v — the blocking flock did not block", waited, holdFor*2/3)
	}
}

// **`zsystem supports` has to be true about what this shell actually built**,
// which is the same rule the feature table follows and the place it is
// easiest to break: it is a status with nothing printed, so a `supports` that
// answered 0 to everything looks like a working one from every angle except
// the one that matters — a script asking before it commits to a lock.
//
// Measured: `flock` and `supports` are the whole subcommand vocabulary, and
// **everything else is 1 including the module's own builtins' names**, so
// `zsystem supports sysread` is 1 in a shell where `sysread` works perfectly.
// The name is the subcommand's, not the feature's.
//
// The two operand-count refusals are **255**, which is the only status like it
// in the module — every other refusal here is 1 — so a script writing
// `zsystem supports` with the name it meant to pass left empty gets an answer
// no ordinary failure produces.
func TestZsystemSupportsAnswersForWhatWasBuilt(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `zsystem supports flock
print -r -- "flock=$?"
zsystem supports supports
print -r -- "supports=$?"
zsystem supports subshell
print -r -- "subshell=$?"
zsystem supports sysread
print -r -- "sysread=$?"
zsystem supports zsystem
print -r -- "zsystem=$?"
zsystem supports
print -r -- "none=$?"
zsystem supports flock extra
print -r -- "two=$?"`)
	want := "flock=0\nsupports=0\nsubshell=1\nsysread=1\nzsystem=1\nnone=255\ntwo=255\n"
	if out != want || st != 0 {
		t.Errorf("zsystem supports = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:zsystem:11: supports: not enough arguments",
		"zsh:zsystem:13: supports: too many arguments",
	)
	// The answers that are only a status said **nothing**, which is what lets
	// `zsystem supports flock || fallback` be written inside a prompt.
	if got, want := len(splitLines(errs)), 2; got != want {
		t.Errorf("stderr = %q, want exactly %d complaints", errs, want)
	}
}

// **What `zsystem` itself refuses, and the subcommand's name is in most of
// it.** Measured one line at a time.
//
// Two of these carry no subcommand name and that is the measurement rather
// than an inconsistency: the sentence about a file names the file and what it
// was to be opened for, which is already more than `flock:` would add.
func TestWhatZsystemRefuses(t *testing.T) {
	dir := t.TempDir()
	out, st, errs := runZshSplit(t, dir, `zsystem
print -r -- "none=$?"
zsystem bogus
print -r -- "subcommand=$?"
zsystem flock
print -r -- "nofile=$?"
zsystem flock /no/such/file
print -r -- "missing=$?"
zsystem flock -r /no/such/file
print -r -- "reading=$?"
zsystem flock -x /no/such/file
print -r -- "letter=$?"
zsystem flock -i bogus /no/such/file
print -r -- "interval=$?"
zsystem flock -u 99
print -r -- "notlocked=$?"`)
	want := "none=1\nsubcommand=1\nnofile=1\nmissing=1\nreading=1\nletter=1\ninterval=1\nnotlocked=1\n"
	if out != want || st != 0 {
		t.Errorf("refusals = %q (status %d), want %q", out, st, want)
	}
	wantWholeLines(t, errs,
		"zsh:zsystem:1: not enough arguments",
		"zsh:zsystem:3: unknown subcommand: bogus",
		"zsh:zsystem:5: flock: not enough arguments",
		"zsh:zsystem:7: failed to open /no/such/file for writing: no such file or directory",
		"zsh:zsystem:9: failed to open /no/such/file for reading: no such file or directory",
		"zsh:zsystem:11: flock: unknown option: x",
		"zsh:zsystem:13: flock: invalid interval value: 'bogus'",
		"zsh:zsystem:15: flock: file descriptor 99 not in use for locking",
	)
}
