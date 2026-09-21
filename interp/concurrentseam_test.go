// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The seam a registered builtin needs to run a command *beside* the shell: on
// descriptors it chose, under a handle it keeps, with a name it can find the
// command again by.
//
// Every test here names the seam and never a shell, which is this package's
// rule. What wanted the seam is a dialect facility that runs a command on a
// pseudo-terminal; nothing below opens one, because none of these questions is
// about terminals — they are about whose descriptors the command gets, who
// owns the name, and what stopping one does.

// concurrentSeamRunner is seamRunner with a `beside` builtin registered: it
// starts the shell text it is given on a pipe the test holds the other end of,
// and publishes the name it started it under.
func concurrentSeamRunner(t *testing.T, out, errs *strings.Builder) (*Runner, *os.File) {
	t.Helper()
	r := seamRunner(t, out, errs)
	near, far, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = near.Close() })
	r.Register("beside", func(rr *Runner, ctx context.Context, args []string) int {
		f, perr := syntax.Parse(args[1], syntax.Core())
		if perr != nil {
			t.Fatalf("parse %q: %v", args[1], perr)
		}
		_, started := rr.StartConcurrent(ctx, ConcurrentCommand{
			Name: args[0],
			Out:  far,
			// The far end goes when the command does, so the test's read of
			// the near end finds the end of it.
			CloseEnds: true,
			Run: func(ctx context.Context, sub *Runner) error {
				return sub.RunPart(ctx, f)
			},
		})
		if !started {
			return 1
		}
		return 0
	})
	return r, near
}

// The command runs on the descriptors the caller chose, and the caller reads
// what it wrote from the other end.
//
// The whole of what the seam is for, in one assertion: a builtin handed the
// command's own end of a pipe, and the bytes came out of the near end. A seam
// that started the command on the *shell's* streams would put the text in the
// runner's output instead, which the second check is for.
func TestACommandRunBesideTheShellWritesToTheDescriptorItWasGiven(t *testing.T) {
	var out, errs strings.Builder
	r, near := concurrentSeamRunner(t, &out, &errs)
	runSeam(t, r, "beside one 'echo beside-ok'\n")
	got := readAll(t, near)
	if !strings.Contains(got, "beside-ok") {
		t.Errorf("the near end = %q, want it to carry beside-ok", got)
	}
	if strings.Contains(out.String(), "beside-ok") {
		t.Errorf("the shell's own output = %q, want the text to have gone to the descriptor", out.String())
	}
	if errs.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", errs.String())
	}
}

// A name that is already running is refused rather than replaced.
//
// The caller's own bookkeeping is what the name is for, so losing the command
// already under one would leave a handle nothing could reach — which is worse
// than a refusal the caller can word for itself.
func TestStartingASecondCommandUnderOneNameIsRefused(t *testing.T) {
	var out, errs strings.Builder
	r, _ := concurrentSeamRunner(t, &out, &errs)
	runSeam(t, r, "beside one 'echo first'\nbeside one 'echo second'\necho st=$?\n")
	if !strings.Contains(out.String(), "st=1") {
		t.Errorf("the second start = %q, want st=1", out.String())
	}
}

// The command is findable again by name, and answers whether it is running.
//
// `Running` is asked of the command and not of the table: the two are
// different questions with different answers, since a name outlives the
// command it was given to.
func TestAConcurrentCommandIsFoundAgainByItsName(t *testing.T) {
	var out, errs strings.Builder
	r, near := concurrentSeamRunner(t, &out, &errs)
	runSeam(t, r, "beside one 'echo done'\n")
	readAll(t, near)
	live, ok := r.ConcurrentNamed("one")
	if !ok {
		t.Fatal("ConcurrentNamed(one) = not there, want the command that was started")
	}
	if live.Name() != "one" {
		t.Errorf("Name() = %q, want one", live.Name())
	}
	waitUntil(t, func() bool { return !live.Running() }, "the command to finish")
	if _, still := r.ConcurrentNamed("one"); !still {
		t.Error("ConcurrentNamed(one) after it ended = not there, want the name to outlive the command")
	}
	if !r.ForgetConcurrent("one") {
		t.Error("ForgetConcurrent(one) = false, want the name to have been there")
	}
	if _, gone := r.ConcurrentNamed("one"); gone {
		t.Error("ConcurrentNamed(one) after forgetting = there, want it gone")
	}
}

// Stopping one ends it, and a command that would otherwise run for a long time
// is what makes that visible.
//
// The body is a loop rather than a `sleep`, so the test asserts on the seam's
// own cancellation reaching the shell's command loop rather than on an
// external program being killed — there is no program here to kill.
func TestStoppingAConcurrentCommandEndsIt(t *testing.T) {
	var out, errs strings.Builder
	r, _ := concurrentSeamRunner(t, &out, &errs)
	runSeam(t, r, "beside slow 'while :; do :; done'\n")
	live, ok := r.ConcurrentNamed("slow")
	if !ok {
		t.Fatal("ConcurrentNamed(slow) = not there")
	}
	if !live.Running() {
		t.Fatal("Running() = false before anything stopped it")
	}
	live.Stop()
	waitUntil(t, func() bool { return !live.Running() }, "the stopped command to end")
}

// A subshell sees the commands its parent started, its Stop reaches the one
// running command, and the name it forgets is its own copy of the name.
//
// The three together are the arrangement the table is cloned for: one command,
// two tables. A shared table would take the name out of the parent as well,
// and copied handles would leave the command running.
func TestASubshellOwnsTheNameAndSharesTheCommand(t *testing.T) {
	var out, errs strings.Builder
	r, _ := concurrentSeamRunner(t, &out, &errs)
	var inside *Runner
	r.Register("inside", func(rr *Runner, _ context.Context, _ []string) int {
		inside = rr
		live, ok := rr.ConcurrentNamed("slow")
		if !ok {
			t.Error("the subshell cannot see the command its parent started")
			return 1
		}
		live.Stop()
		rr.ForgetConcurrent("slow")
		return 0
	})
	runSeam(t, r, "beside slow 'while :; do :; done'\n( inside )\n")
	if inside == nil {
		t.Fatal("the subshell never ran")
	}
	live, ok := r.ConcurrentNamed("slow")
	if !ok {
		t.Fatal("the parent lost the name a subshell forgot, want it kept")
	}
	waitUntil(t, func() bool { return !live.Running() },
		"the subshell's stop to reach the parent's command")
	if _, still := inside.ConcurrentNamed("slow"); still {
		t.Error("the subshell still has the name it forgot")
	}
}

// readAll reads the near end to its end, which the far end closing is what
// produces.
func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	var got strings.Builder
	buf := make([]byte, 256)
	for {
		n, err := f.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			return got.String()
		}
	}
}

// waitUntil polls a condition rather than sleeping on it, so a slow machine
// waits longer and a fast one does not wait at all.
func waitUntil(t *testing.T, done func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
