// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// blockingFifo makes a named pipe with nobody at either end, so that opening
// it for reading waits.
//
// The cleanup is what keeps the job from outliving the test: a goroutine
// blocked in open(2) is not going anywhere on its own, and a test binary that
// left one behind per case would end with as many. Opening the other end
// releases it, and closing that end then gives the reader its end of input.
func blockingFifo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("named pipes are not available here: %v", err)
	}
	t.Cleanup(func() {
		w, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_ = w.Close()
	})
	return path
}

// deadline runs body and fails the test if it has not finished in time.
//
// A hang is not a wrong answer, and asserting on the output cannot catch one:
// the test simply never returns and the package reports a panic ten minutes
// later naming whichever case the binary was in. So the wait is bounded and
// the failure says what did not happen — the shape #1002 was solved with.
func deadline(t *testing.T, what string, body func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		body()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s did not finish: the shell is still waiting", what)
	}
}

// `&` comes back even when the job's first act is to wait for something that
// is not coming.
//
// Starting a job waits for its pid to settle, so that `$!` is answerable on
// the next line — and the two things that settled it were a process having
// started and the job having ended. A job that blocks *before* either reaches
// neither, so `&` waited for an answer that was never coming and the shell
// never reached the next command at all: measured, this printed nothing here
// and printed the marker at once in all six shells in the panel, which fork
// before they open anything (#1003).
//
// Every shape of job, because the blocking is in the redirection and a
// redirection is opened before the shell knows whether the command is a
// builtin, a function or a program. The subshell, the brace group, the
// function and the bare builtin all failed; the external command failed too,
// through a redirection it never got as far as using.
func TestAnAmpersandReturnsWhenTheJobBlocksBeforeStartingAProcess(t *testing.T) {
	for _, w := range []struct{ name, src string }{
		{"a subshell", `(read x < %[1]s; :) & printf NOW-42`},
		{"a bare builtin", `read x < %[1]s & printf NOW-42`},
		{"a brace group", `{ read x < %[1]s; } & printf NOW-42`},
		{"a function", `f() { read x < %[1]s; }; f & printf NOW-42`},
		{"an external command", `/bin/cat < %[1]s & printf NOW-42`},
	} {
		t.Run(w.name, func(t *testing.T) {
			fifo := blockingFifo(t)
			var got string
			deadline(t, "starting a job whose first act blocks", func() {
				got, _ = runLeavingJobsRunning(t, strings.ReplaceAll(w.src, "%[1]s", fifo), nil)
			})
			if got != "NOW-42" {
				t.Errorf("output = %q, want NOW-42 — the shell did not reach the next command", got)
			}
		})
	}
}

// And the pid a job really has is still the pid it reports.
//
// The cheap fix for the case above is to settle the pid as soon as the job
// starts, and it costs a real answer: `$!` here is the process the job runs,
// and a job settled early would report the zero of a job with none. So the
// settling is only where the open can really wait — a named pipe with no peer
// — and everything else a redirection can name must not reach it.
//
// `/dev/null` is the row that earns its place. A draft settled on a character
// device as well as a fifo, on the reasoning that a terminal's open can wait,
// and `sleep 1 > /dev/null &` — about as ordinary as a background job gets —
// began reporting no pid at all. Nothing in the suite said so until this case
// was written.
func TestABackgroundJobsPidSurvivesARedirectionThatCannotBlock(t *testing.T) {
	for _, w := range []struct{ name, src string }{
		{"a redirection to a file", `/bin/sleep 1 > out.txt & printf "%s" "$!"`},
		{"a subshell around one", `(/bin/sleep 1 > out.txt) & printf "%s" "$!"`},
		{"no redirection at all", `/bin/sleep 1 & printf "%s" "$!"`},
		{"a character device", `/bin/sleep 1 > /dev/null & printf "%s" "$!"`},
		{"reading a character device", `/bin/sleep 1 < /dev/zero & printf "%s" "$!"`},
	} {
		t.Run(w.name, func(t *testing.T) {
			got, _ := runLeavingJobsRunning(t, w.src, nil)
			pid, err := strconv.Atoi(strings.TrimSpace(got))
			if err != nil || pid <= 0 {
				t.Fatalf(`$! = %q, want the job's pid`, got)
			}
		})
	}
}

// A job stopped where it stands has no process, and says so.
//
// The other half of the trade: a job settled at a blocking open is settled
// *without* a pid, because nothing has started and while the job stands there
// nothing will. Zero is what this shell reports for a job it has no process
// for, which is stated on Job.PID rather than papered over — and it is what
// keeps the pid's one write on the near side of the channel that publishes it,
// so a process starting later cannot move it behind the shell's back.
func TestABackgroundJobBlockedBeforeAnyProcessReportsNoPid(t *testing.T) {
	fifo := blockingFifo(t)
	var got string
	deadline(t, "starting a job whose first act blocks", func() {
		got, _ = runLeavingJobsRunning(t, `/bin/cat < `+fifo+` & printf "[%s]" "$!"`, nil)
	})
	if got != "[0]" {
		t.Errorf(`$! = %s, want [0] — a job with no process of its own reports zero`, got)
	}
}
