// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// A reading substitution's body is held until the parent lets it go, and its
// `xtrace` lines therefore arrive in a fixed order (#5088).
//
// `: <(print A); sleep 0` wrote the body's line *after* `@ sleep 0`: the body
// ran in a goroutine nothing had yielded to.
//
// **A head start was not enough and is not what this is.** Releasing the body
// early is a lower bound — it has traced *by* a point — and nothing in it stops
// it tracing *earlier* than the reference does; that version flaked about one
// run in a few. Each body is now **held** before its first command and released
// at one of two points: the fork of the next body, or the command's own trace
// line. See interp/procsubtracestart.go.
//
// **Only settled shapes are asserted here.** The reference's order is itself a
// race — over five runs of each of twenty-five shapes it disagreed with itself
// on several — so "matches the reference" is not a well-formed goal for those.
// The rows below are shapes that settle under five runs of each of the three
// shells.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
var procSubFd = regexp.MustCompile(`/dev/fd/[0-9]+`)

// procSubTrace is the `@ `-prefixed lines of a run's standard error. This
// harness has no PATH, so `sleep 0` is traced and then not found; the
// diagnostic that follows is the fixture's and not an answer.
func procSubTrace(t *testing.T, dir, src string) string {
	t.Helper()
	_, _, errs := runZshSplit(t, dir, "PS4='@ '\nsetopt xtrace\n"+src)
	var kept []string
	for _, line := range strings.Split(errs, "\n") {
		if strings.HasPrefix(line, "@ ") {
			kept = append(kept, procSubFd.ReplaceAllString(line, "FD"))
		}
	}
	return strings.Join(kept, "\n") + "\n"
}

func TestAReadingSubstitutionsBodyTracesInOrder(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// One body: nothing is forked after it, so the command's own line
		// releases it and its line lands behind that one.
		{"one body", ": <(print A)\nsleep 0\n", "@ : FD\n@ print A\n@ sleep 0\n"},
		// Two: the first is released by the second's fork, so it traces in
		// front of the command's line and the second behind it. This is the
		// row the issue named as the discriminator.
		{
			"two bodies", ": <(print A) <(print B)\nsleep 0\n",
			"@ print A\n@ : FD FD\n@ print B\n@ sleep 0\n",
		},
		{
			"three bodies", ": <(print A) <(print B) <(print C)\nsleep 0\n",
			"@ print A\n@ print B\n@ : FD FD FD\n@ print C\n@ sleep 0\n",
		},
		{
			"four bodies", ": <(print A) <(print B) <(print C) <(print D)\nsleep 0\n",
			"@ print A\n@ print B\n@ print C\n@ : FD FD FD FD\n@ print D\n@ sleep 0\n",
		},
		// The file spelling runs its body where it stands, so a reading body
		// forked before it is let go first.
		{
			"a reading body before a file one", ": <(print A) =(print B)\nsleep 0\n",
			"@ print A\n@ print B\n@ : FD TMP\n@ sleep 0\n",
		},
		// And a command substitution, for the same reason and with the same
		// release.
		{
			"a reading body before a command substitution",
			`: <(print A) "$(print B)"` + "\nsleep 0\n",
			"@ print A\n@ print B\n@ : FD B\n@ sleep 0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := procSubTrace(t, dir, tc.src)
			// The file spelling's path is the shell's own.
			if i := strings.Index(got, "@ : FD /"); i >= 0 {
				if j := strings.IndexByte(got[i:], '\n'); j >= 0 {
					got = got[:i] + "@ : FD TMP" + got[i+j:]
				}
			}
			if got != tc.want {
				t.Errorf("trace\n%swant\n%s", got, tc.want)
			}
		})
	}
}

// The spellings that do not fork a pipe are unmoved, which is what says this is
// about a forked body and not about the trace.
func TestTheUnforkedSubstitutionsKeepTheirOrder(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a command substitution", `: "$(print A)"` + "\nsleep 0\n", "@ print A\n@ : A\n@ sleep 0\n"},
		{"a file substitution alone", ": =(print A)\nsleep 0\n", "@ print A\n@ : TMP\n@ sleep 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := procSubTrace(t, dir, tc.src)
			if i := strings.Index(got, "@ : /"); i >= 0 {
				if j := strings.IndexByte(got[i:], '\n'); j >= 0 {
					got = got[:i] + "@ : TMP" + got[i+j:]
				}
			}
			if got != tc.want {
				t.Errorf("trace\n%swant\n%s", got, tc.want)
			}
		})
	}
}

// **The hold is bounded by the body's first line, never by its work**, and a
// script that is not tracing takes no hold at all. Both halves are here because
// the failure they guard is a hang, which a row asserting output cannot
// distinguish from a slow machine.
func TestHoldingABodyCannotHoldTheCommandUp(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		// Traced: the body is held, released at the command's line, and the
		// wait ends when it traces `sleep 5` — not when it finishes.
		{"a slow body while tracing", "PS4='@ '\nsetopt xtrace\n: <(sleep 5)\nprint -r -- past\n"},
		// Untraced: nothing is created and nothing is waited for.
		{"a body that never ends, untraced", ": <(while :; do sleep 1; done)\nprint -r -- past\n"},
		// The command's own line is never reached, so the backstop is what
		// lets the body go.
		{"a command that fails before its trace line", "PS4='@ '\nsetopt xtrace\n: <(print A) >/nope/x\nprint -r -- past\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan string, 1)
			go func() {
				out, _ := runZsh(t, dir, tc.src)
				done <- out
			}()
			select {
			case out := <-done:
				if !strings.Contains(out, "past") {
					t.Errorf("out %q, want the command to have run on", out)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("the command did not run on: a held body was never let go")
			}
		})
	}
}

// **The hold cannot be proven by a single run**, and this is the row that
// proves it anyway.
//
// Removing the hold — letting a body run the moment it is forked and keeping
// only the waits — passes every row above, every time, on an idle machine: the
// order it produces is the *same* order, because the parent still waits for the
// body at each release point. What the hold removes is a **race**, and a race
// that is currently being won is invisible to one run.
//
// It is not invisible to two hundred. Measured 2026-09-29: with the hold taken
// out, the one-body row fails inside a `-count=200` run of this package; with it
// in, two hundred runs pass. So the repetition is the instrument, and it is
// cheap — the whole loop is well under a second, because each iteration is one
// short script.
//
// Without this, the mutant that deletes the hold survives, and what ships is a
// bias that looks like an ordering until somebody's machine is busy.
func TestTheOrderHoldsOverManyRuns(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"one body", ": <(print A)\nsleep 0\n", "@ : FD\n@ print A\n@ sleep 0\n"},
		{
			"two bodies", ": <(print A) <(print B)\nsleep 0\n",
			"@ print A\n@ : FD FD\n@ print B\n@ sleep 0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 200; i++ {
				if got := procSubTrace(t, dir, tc.src); got != tc.want {
					t.Fatalf("run %d of 200 gave\n%swant\n%s", i+1, got, tc.want)
				}
			}
		})
	}
}

// A body the command itself reads, which is what says the release point is
// **before** the redirections rather than after them.
//
// `.` opens the path its operand names and reads it, so a body still held when
// the command runs would leave the shell reading a pipe nobody is writing. That
// is the deadlock the earlier reading of this issue said made holding
// impossible; it is real, and it is why the release is taken at the command's
// own trace line and not at removeProcSubs.
//
// The mutant that moves the release back to removeProcSubs hangs here, which is
// the only row in the package that can tell it from the real thing.
func TestACommandThatReadsItsOwnSubstitution(t *testing.T) {
	dir := t.TempDir()
	done := make(chan string, 1)
	go func() {
		out, _ := runZsh(t, dir, "PS4='@ '\nsetopt xtrace\n. <(print -r -- \":\")\nprint -r -- past\n")
		done <- out
	}()
	select {
	case out := <-done:
		if !strings.Contains(out, "past") {
			t.Errorf("out %q, want the command to have run on", out)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the shell never finished reading its own substitution: the body was still held")
	}
}
