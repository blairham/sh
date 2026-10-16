// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A substitution's number is not counted from descriptors this process holds
// only for a moment — a program being started, or the shell's own plumbing on
// its way into the private band — because a forking shell holds none of them
// in the table an element numbers from (#6454).
//
// The descriptor here is held inside the window, on the lowest free number,
// for longer than the whole run takes, so the answer is not a race: without
// the park waiting the window out, the first substitution steps over it on
// every run.
func TestAParkWaitsOutADescriptorHeldOnlyForAMoment(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	want := lowestFreeFds(t, 3)
	held := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		InTransientFdWindowForTest(func() {
			f, err := os.Open(os.DevNull)
			if err != nil {
				t.Error(err)
				close(held)
				return
			}
			close(held)
			time.Sleep(300 * time.Millisecond)
			_ = f.Close()
		})
	}()
	<-held
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
		AllocateDescriptorsFromTen, nil, "echo <(true) <(true) <(true)", nil))
	<-done
	if !sameFds(got, want) {
		t.Errorf("got %v, want %v — the numbers free outside the window; a descriptor "+
			"held only while something was starting was counted in the answer", got, want)
	}
}

// The directory a shell holds is not in a substitution's numbering either. A
// real shell's working directory is the kernel's and takes no descriptor, so
// the hold this one opens at its first external command is moved out of the
// low numbers: before that, `/bin/echo x >/dev/null; echo <(true) <(true)
// <(true)` answered `3 5 6` in the ksh dialect where ksh93u+ answers `3 4 5`
// (#6454). Deterministic — the hold is opened by the first command and stays
// — so one run is the whole test.
func TestTheHeldDirectoryIsNotInASubstitutionsNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	prog, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no true on PATH: %v", err)
	}
	want := lowestFreeFds(t, 3)
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
		AllocateDescriptorsFromTen, nil, prog+"; echo <(true) <(true) <(true)", nil))
	if !sameFds(got, want) {
		t.Errorf("got %v, want %v — after an external command the shell's hold on its "+
			"directory was counted in the answer", got, want)
	}
}

// And the race the issue was filed from, end to end: a sibling element
// starting a program while the other numbers its substitutions.
//
// Measured on darwin before the fix, the first answer was the free one about
// half the time, so twenty runs all agreeing is not luck. On Linux os/exec
// also keeps a pidfd for the life of the child, which is a descriptor of a
// different kind — held, not momentary — and is claimed as nobody's once the
// start returns (#6476; see claimStartLeftovers).
// Both of the shell's output streams are files so that os/exec hands them
// straight to the child: given anything else it makes a pipe of its own to
// copy through, and that pipe is held for the child's life rather than for a
// moment.
func TestASiblingStartingAProgramIsNotInASubstitutionsNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	prog, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no true on PATH: %v", err)
	}
	d := syntax.Core()
	d.ProcessSubstitution = true
	f, err := syntax.Parse("{ echo <(true) <(true) <(true) >&2; } | "+prog, d)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out, err := os.Create(dir + "/out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errs, err := os.Create(dir + "/err")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.Close()
	const runs = 20
	want := lowestFreeFds(t, 3)
	for i := range runs {
		if err := errs.Truncate(0); err != nil {
			t.Fatal(err)
		}
		if _, err := errs.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		sem := PosixSemantics()
		sem.SubstitutionEndPlacement = SubstitutionEndsAtTheLowestFreeNumber
		sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
			Stdout: out, Stderr: errs,
		})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		r.CleanUp()
		said, err := os.ReadFile(errs.Name())
		if err != nil {
			t.Fatal(err)
		}
		got := substNumbers(t, string(said))
		if !sameFds(got, want) {
			t.Fatalf("run %d of %d: got %v, want %v — the sibling's start was counted in "+
				"the answer", i+1, runs, got, want)
		}
	}
}
