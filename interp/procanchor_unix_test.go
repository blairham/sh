// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// A process substitution's body can have a **process group of its own**, led
// by a placeholder the front end names. See interp/procanchor.go for what asks
// for one and why a group is the answer to a question that sounds like it is
// about a process.
//
// Every assertion here is against the kernel — `Getpgid`, and a signal 0 aimed
// at the group — rather than against a number the shell printed. A test that
// only checked the value was non-empty would pass for a shell answering any
// plausible number, which is precisely the answer that must never be given.

// anchorCommand is the placeholder these tests use: a program that blocks on
// its standard input and exits when it reaches end of file, which is the whole
// contract interp asks of one.
//
// `cat` rather than a shell binary, because these are the interpreter's tests
// and a shell built here would tie them to the front end. What the shipped
// binaries actually use is driver's own, which is a copy of themselves — see
// driver/procanchor.go for why it is not this.
func anchorCommand(t *testing.T) []string {
	t.Helper()
	path, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("no placeholder program on this machine: %v", err)
	}
	return []string{path}
}

// recordGroup registers a builtin that answers what the body it runs in was
// told its process group is, so a test can ask the seam directly rather than
// through a dialect's parameter.
func recordGroup(r *Runner, into *int, mu *sync.Mutex) {
	r.Register("group", func(body *Runner, _ context.Context, _ []string) int {
		pgid, ok := body.SubshellProcessGroup()
		mu.Lock()
		if ok {
			*into = pgid
		} else {
			*into = 0
		}
		mu.Unlock()
		return 0
	})
}

// TestASubstitutionBodyLeadsAProcessGroupOfItsOwn is the whole claim, checked
// against the kernel: the number the body is given names a live process group,
// that group is led by the placeholder rather than by this process, and it is
// not the group this process is in — which is the property a script spends the
// value on and the one whose absence killed a session (#2046).
func TestASubstitutionBodyLeadsAProcessGroupOfItsOwn(t *testing.T) {
	var mu sync.Mutex
	var pgid int
	_, st := run(t, `cat <(group)`, func(r *Runner) {
		r.ProcessAnchor = anchorCommand(t)
		recordGroup(r, &pgid, &mu)
	})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if pgid <= 0 {
		t.Fatalf("the body was given %d as its process group, want a real one", pgid)
	}
	if self, err := syscall.Getpgid(0); err != nil {
		t.Fatalf("Getpgid: %v", err)
	} else if pgid == self {
		t.Fatalf("the body was given this process's own group %d; `kill -- -%d` would reach the shell", pgid, pgid)
	}
	// And the group's leader is the placeholder: its id is its own process
	// id, which is what `Setpgid` with no `Pgid` means and what makes the
	// number both a process to signal and a group to signal.
	if got, err := syscall.Getpgid(pgid); err == nil && got != pgid {
		t.Errorf("process %d leads group %d, want it to lead its own", pgid, got)
	}
}

// TestNoAnchorLeavesTheBodyWithoutAGroup is the other half of the seam, and it
// is the behavior every body had before there was one: a front end that named
// no placeholder — a Runner embedded in some other program — gets a shell
// whose bodies answer "no group", not a shell that invents one.
func TestNoAnchorLeavesTheBodyWithoutAGroup(t *testing.T) {
	var mu sync.Mutex
	pgid := -1
	if _, st := run(t, `cat <(group)`, func(r *Runner) {
		recordGroup(r, &pgid, &mu)
	}); st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if pgid != 0 {
		t.Errorf("a body with no placeholder was given group %d, want none", pgid)
	}
}

// TestACommandTheBodyRunsJoinsTheBodysGroup is what makes the number worth
// having. A group holding only the placeholder is a group whose teardown
// reaches nothing, so the daemon a script started this way would outlive the
// `kill -- -$pgid` written to stop it.
//
// Asserted through `ps`, because the question is what the *kernel* put the
// child in and the shell's own bookkeeping cannot answer it.
func TestACommandTheBodyRunsJoinsTheBodysGroup(t *testing.T) {
	var mu sync.Mutex
	var pgid int
	out, st := run(t, `cat <(group; sh -c 'ps -o pgid= -p $$')`, func(r *Runner) {
		r.ProcessAnchor = anchorCommand(t)
		recordGroup(r, &pgid, &mu)
	})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	mu.Lock()
	defer mu.Unlock()
	child, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("the body reported %q as its child's group: %v", out, err)
	}
	if child != pgid {
		t.Errorf("a command the body ran is in group %d, want the body's own %d", child, pgid)
	}
}

// TestThePlaceholderGoesWhenTheBodysFamilyDoes is the lifetime, and it is
// substEnd's rather than the body's: a job the body backgrounded holds the
// group open after the body has returned, the same way it holds the pipe open,
// because a fork gives a real shell both for nothing.
//
// The two halves are one test on purpose. Asserting only that the placeholder
// eventually goes would pass for a shell that ended the group while the job
// that needed it was still running, which is the bug this lifetime exists to
// prevent.
func TestThePlaceholderGoesWhenTheBodysFamilyDoes(t *testing.T) {
	var mu sync.Mutex
	var pgid int
	out, st := run(t, `cat <(group; { sleep 0.4; sh -c 'ps -o pgid= -p $$'; } &)`, func(r *Runner) {
		r.ProcessAnchor = anchorCommand(t)
		recordGroup(r, &pgid, &mu)
	})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	mu.Lock()
	defer mu.Unlock()
	// The job ran *after* the body returned, and it is still in the body's
	// group: a shell that ended the group with the body would have had
	// nowhere to put this child. That is the half a "the placeholder goes
	// eventually" assertion cannot make.
	late, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("the backgrounded job reported %q as its group: %v", out, err)
	}
	if late != pgid {
		t.Errorf("a job the body backgrounded is in group %d, want the body's own %d", late, pgid)
	}
	// run() waits for the substitution's reader, which cannot finish until
	// the job has: the pipe stays open until the last of the family lets go.
	// So by here the group has been released, and what is left is the
	// placeholder noticing — it is a process, and a process exits when it is
	// scheduled to.
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(-pgid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("process group %d is still there after the body's family finished", pgid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
