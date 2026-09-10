// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A job the substitution's body backgrounded keeps the pipe open after the
// body has returned.
//
// # The behavior
//
// A real shell forks for `<(cmd)` and the pipe's descriptor belongs to the
// process it forked; a job that process backgrounds is another fork holding a
// copy, so the pipe outlives the body by exactly as long as the job does.
// Measured 2026-09-10 with
//
//	cat <( { for i in 1 2 3; do printf B; sleep 0.2; done } & )
//
// which prints `BBB` in zsh 5.9.2, bash 5.3, bash 3.2 and ksh93 alike. Every
// shell in the panel that has the construct agrees, so this is the core's
// answer and no dialect is asked; the one shell without it has no opinion to
// disagree with. Here it printed a single `B` — #1767 — because the body's
// return closed a descriptor the job was still writing through.
//
// # Why the release is a file and not a sleep
//
// The trap in a case like this is that a job which finishes *before* the body
// returns cannot tell the two behaviors apart: the write lands either way. So
// the job must still be waiting when the body returns, and what releases it
// has to happen strictly afterwards. The body's last act is to write the file
// this test waits for; only then does the test write the file the job is
// waiting for. Two files rather than a duration, so a loaded machine makes the
// case slower and never makes it wrong.
func TestABackgroundJobKeepsASubstitutionsPipeOpen(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		// enable turns on the grammar a case needs by name, so the case
		// states the construct it depends on rather than a shell that
		// happens to have it.
		enable func(*syntax.Dialect)
	}{
		// The shape the issue is about, and the one the prompt theme that
		// found it uses: a worker backgrounded inside the substitution, the
		// body then finishing at once.
		{
			name: "backgrounded by the body",
			src:  "cat <(" + lateJobScript + "\nprintf early\n" + bodyDone + ")",
			want: "earlylate",
		},
		// A function is a frame rather than a boundary, but the job is
		// started through one and the runner that starts it is not the one
		// the substitution made.
		{
			name: "backgrounded inside a function",
			src:  "f() {\n" + lateJobScript + "\n}\ncat <(f\nprintf early\n" + bodyDone + ")",
			want: "earlylate",
		},
		// A subshell *is* a boundary: the job belongs to a clone of a clone,
		// and what it inherited has to reach it.
		{
			name: "backgrounded inside a subshell",
			src:  "cat <( (\n" + lateJobScript + "\n)\nprintf early\n" + bodyDone + ")",
			want: "earlylate",
		},
		// The inner substitution's body makes a clone whose own end is a
		// different pipe. The outer body's job must still hold the outer
		// one — an inner substitution is not a job letting go of anything.
		{
			name: "outer body's job, with an inner substitution beside it",
			src:  "cat <(cat <(printf i)\n" + lateJobScript + "\nprintf early\n" + bodyDone + ")",
			want: "iearlylate",
		},
		// And the other way round: the job belongs to the *inner* body, so
		// what it holds open is the inner pipe, whose end-of-file the outer
		// body is reading until.
		{
			name: "inner body's job",
			src:  "cat <(cat <(" + lateJobScript + "\nprintf early\n" + bodyDone + "))",
			want: "earlylate",
		},
		// A disowned job is let go of by the table and by nothing else. It
		// is still a process a real shell forked, so it still holds the
		// pipe: measured, `cat <( { printf B; sleep 0.3; printf C; } &! )`
		// prints `BC` in zsh 5.9.2 and in ksh93, the two shells that spell
		// it.
		{
			name:   "a disowned job",
			src:    "cat <(" + lateDisownedJobScript + "\nprintf early\n" + bodyDone + ")",
			want:   "earlylate",
			enable: func(d *syntax.Dialect) { d.BackgroundAndDisown = true },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			done := filepath.Join(dir, "body-returned")
			release := filepath.Join(dir, "go-ahead")
			src := strings.NewReplacer("%[1]s", done, "%[2]s", release).Replace(tc.src)

			stop := releaseAfter(t, done, release)
			defer stop()

			// The grammar has to reach the *runner* as well as the parse:
			// a substitution re-parses its own body, with the dialect the
			// runner was given rather than the one the outer source was
			// read with, so a construct enabled only for the outer parse is
			// a syntax error the moment it is inside `<( … )`.
			var setup func(*Runner)
			if tc.enable != nil {
				d := syntax.Core()
				tc.enable(&d)
				setup = func(r *Runner) { r.Dialect = &d }
			}

			out, st := runBoundedScript(t, src, tc.enable, setup)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q — the job's write after the body returned "+
					"went into a pipe the body had closed", out, tc.want)
			}
		})
	}
}

// The same for `>(cmd)`, where the end the shell holds is the reading one.
//
// It is the same descriptor question in the other direction: the job reads
// through the end the body was given, so closing that end on the body's return
// takes the job's input away. Reached only where the dialect hands a `&` job
// the shell's own standard input, which is the axis this names rather than a
// shell — see Semantics.BackgroundJobInput. Measured with
//
//	echo hi > >( { sleep 0.3; cat; } & ); sleep 0.8
//
// which prints `hi` in zsh 5.9.2 and nothing in bash 5.3 or bash 3.2, exactly
// as the axis says it should.
//
// The result goes to a file rather than to the shell's output because the
// script ends before the job does: the job belongs to a clone, so the outer
// shell's `wait` has nothing to wait for and the buffer would be read while
// the job was still writing to it.
func TestABackgroundJobKeepsTheReadingEndOfASubstitutionOpen(t *testing.T) {
	dir := t.TempDir()
	done := filepath.Join(dir, "body-returned")
	release := filepath.Join(dir, "go-ahead")
	got := filepath.Join(dir, "got")

	src := strings.NewReplacer("%[1]s", done, "%[2]s", release, "%[3]s", got).Replace(
		"printf 'hi\\n' > >({ while [ ! -e %[2]s ]; do sleep 0.02; done; read -r v\n" +
			"printf 'got:%s' \"$v\" > %[3]s; } &\n" + bodyDone + ")")

	stop := releaseAfter(t, done, release)
	defer stop()

	if _, st := runBoundedScript(t, src, nil, func(r *Runner) {
		r.Semantics.BackgroundJobInput = BackgroundJobInputIsTheShells
	}); st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if b := waitForFile(t, got); b != "got:hi" {
		t.Errorf("the job read %q, want %q — the body's return closed the end it was reading",
			b, "got:hi")
	}
}

// Many substitutions with a job apiece leave no descriptor behind, and none of
// them waits forever.
//
// This is the other half of the fix and the reason it is not simply "do not
// close". An end nobody ever closes turns an early end-of-file into a reader
// that blocks for good, and a descriptor per substitution is a shell that runs
// out of them; both are worse than what was wrong. So the count has to reach
// zero, and reaching it has to close.
//
// The bound is what says there is no hang — a round that never ends stops the
// test with a message rather than running until the package times out — and
// the descriptor count is what says there is no leak. Each round's job is
// still running when its body returns, so every round exercises the hold
// rather than the case that would have worked anyway.
//
// What each half detects was checked by breaking the fix in two directions.
// A count that never reaches zero is caught by the bound, as a named failure
// at 30 seconds rather than a package timeout. A descriptor left open is
// caught here — 56 against a baseline of 6 over 25 rounds — but only when it
// is a *raw* one: a leaked `*os.File` is closed by the finalizer the runtime
// puts on it, so an equivalent leak spelled `os.Open` passes this. That is
// worth knowing rather than worth working around; the descriptors this
// package holds a substitution's ends in are `*os.File`, so the count here is
// evidence about the syscall-level end of it and the bound is what covers the
// rest.
func TestSubstitutionsWithBackgroundJobsLeaveNoDescriptorsBehind(t *testing.T) {
	const rounds = 25

	before, err := openDescriptors()
	if err != nil {
		t.Skipf("this platform does not list its open descriptors: %v", err)
	}

	deadline := time.Now().Add(2 * time.Minute)
	for i := range rounds {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d rounds finished inside the bound", i, rounds)
		}
		// A job that outlives the body in both directions, on every round.
		// `sleep` rather than a file because nothing here has to be ordered
		// against the body's return — what is being counted is descriptors,
		// and a round that is merely slow still counts them.
		if out, st := runBoundedScript(t,
			"cat <({ sleep 0.05; printf x; } &\nprintf y)", nil, nil); st != 0 || out != "yx" {
			t.Fatalf("round %d: out = %q status %d, want %q", i, out, st, "yx")
		}
		if _, st := runBoundedScript(t,
			`printf 'z\n' > >({ sleep 0.05; cat >/dev/null; } &)`, nil, nil); st != 0 {
			t.Fatalf("round %d: status %d on the writing direction", i, st)
		}
	}

	// The jobs of the last rounds may still be holding their ends, which is
	// the arrangement working rather than a leak. What a leak looks like is a
	// count that never comes back down.
	after := waitForDescriptors(before + descriptorSlack)
	if after > before+descriptorSlack {
		t.Errorf("%d descriptors open after %d substitutions, %d before — "+
			"an end nothing let go of", after, rounds, before)
	}
}

// descriptorSlack is what the rest of the test binary is allowed to be doing
// while this counts. The number under test is a leak of one or two per round,
// so anything that would hide it is far above this.
const descriptorSlack = 8

// lateJobScript is a background job that will not write until the test says
// so, which is what puts its write after the body's return. `%[2]s` is the
// file it waits for.
const lateJobScript = `{ while [ ! -e %[2]s ]; do sleep 0.02; done; printf late; } &`

// lateDisownedJobScript is the same job, let go of by the job table.
const lateDisownedJobScript = `{ while [ ! -e %[2]s ]; do sleep 0.02; done; printf late; } &!`

// bodyDone is the substitution body's last act: the file that says the body
// has reached its end, so the test can release the job strictly after the
// close that used to happen here. `%[1]s` is that file.
const bodyDone = `printf d > %[1]s`

// releaseAfter lets the background job go once the body has said it has
// finished, on a goroutine of the test's own, and answers with the stop for
// the test to defer.
//
// The waiting is here rather than in the script because the script is blocked:
// the command that named the substitution is reading the pipe, which is the
// whole point of the case.
func releaseAfter(t *testing.T, done, release string) func() {
	t.Helper()
	stop := make(chan struct{})
	go func() {
		deadline := time.Now().Add(30 * time.Second)
		for {
			if b, err := os.ReadFile(done); err == nil && len(b) > 0 {
				break
			}
			select {
			case <-stop:
				return
			default:
			}
			if time.Now().After(deadline) {
				// Let the job go anyway. The case is already lost, and a
				// job left waiting for a file that never arrives is a
				// blocked shell and a test that reports a timeout rather
				// than a difference.
				break
			}
			time.Sleep(time.Millisecond)
		}
		_ = os.WriteFile(release, []byte("go"), 0o600)
	}()
	return func() { close(stop) }
}

// openDescriptors counts what this process has open.
//
// /dev/fd is the process's own descriptor table on both platforms this builds
// for — a real directory on one and a symbolic link into /proc on the other —
// so the count is portable even though what backs it is not. The read itself
// takes a descriptor, which is why this is only ever compared against another
// reading of the same thing.
func openDescriptors() (int, error) {
	// Named rather than stat'ed. os.ReadDir asks after every entry it finds,
	// and one of the entries is the descriptor doing the asking — which the
	// directory has already stopped knowing about by the time the question
	// reaches it, so a whole-directory read of this one fails on a platform
	// where reading it works perfectly.
	dir, err := os.Open("/dev/fd")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// waitForDescriptors gives the jobs still finishing a moment to let their ends
// go, so the count is of what was left rather than of what is in flight.
// Generous, because the answer it is used for is "back down" or "never".
func waitForDescriptors(want int) int {
	deadline := time.Now().Add(30 * time.Second)
	for {
		n, err := openDescriptors()
		if err != nil || n <= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}
