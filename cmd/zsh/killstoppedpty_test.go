// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A signal aimed at a job that is **stopped** continues it first, so the
// signal lands instead of sitting pending on a process nothing has run again.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — on a pseudo-terminal with `TERM=dumb`, `PS1` and `PS2`
// exported empty and the shell started `-fiV +Z`, one `sleep 300 &` and one
// `kill -STOP %1` per row:
//
//	kill -0 %1      SN, and `[1]  + running     sleep 300`
//	kill -WINCH %1  SN
//	kill -CONT %1   SN
//	kill -STOP %1   TN
//	kill -TERM %1   the process is gone, `[1]  + terminated  sleep 300`
//
// This shell continued nothing (#4526): every one of those left the process
// reading `TN`, and on Linux — where the kernel holds a fatal signal pending
// for a stopped process rather than acting on it — `kill -STOP %1` followed
// by `kill -TERM %1` was a silent no-op, the job still listed as suspended
// and the SIGTERM never delivered.
//
// **The state is read from outside the shell**, by `ps` on the pid, and that
// is the whole reason this file exists rather than an assertion about the job
// table. A stopped process that was never continued and a process that
// received nothing look identical from inside: the table would agree with
// itself either way. `ps -o stat=` is the only witness that is not the
// subject — `T` is stopped, `S` is sleeping, nothing at all is gone.
func TestASignalToAStoppedJobContinuesItFirst(t *testing.T) {
	control, screen := jobNoticeSession(t)
	stopAJob(t, control, screen)

	// The positive control, and it is load-bearing twice over: it says the
	// job really is stopped before the row under test, and it says `ps` on
	// this machine can report that state at all. A row asserting "not
	// stopped" against an instrument that never reports stopped would pass
	// for any shell whatever.
	if got := jobState(t, control, screen); !strings.HasPrefix(got, "T") {
		t.Fatalf("the job is %q rather than stopped; nothing below grades anything", got)
	}

	// Signal 0 delivers nothing anybody can act on, which is what makes it
	// the row that discriminates: a job running afterwards was continued by
	// the shell and by nothing else. A fatal signal cannot ask this on
	// macOS, where the kernel ends a stopped process without waiting to be
	// continued.
	jobNoticeType(t, control, screen, "kill -0 %1")
	if got := jobState(t, control, screen); !strings.HasPrefix(got, "S") {
		t.Errorf("after `kill -0 %%1` the job is %q, want it running: the shell did not continue it", got)
	}
}

// And not for a signal that stops, which is the one shape the rule excludes.
//
// Beside the row above rather than folded into it: together they are the pair
// that says the rule is keyed on the signal and not on "any `kill` wakes a
// job up". Measured on 5.9.2 the same day — `kill -STOP %1` on a job already
// stopped leaves it `TN`.
func TestAStoppingSignalLeavesTheJobStopped(t *testing.T) {
	control, screen := jobNoticeSession(t)
	stopAJob(t, control, screen)
	if got := jobState(t, control, screen); !strings.HasPrefix(got, "T") {
		t.Fatalf("the job is %q rather than stopped; nothing below grades anything", got)
	}
	jobNoticeType(t, control, screen, "kill -STOP %1")
	if got := jobState(t, control, screen); !strings.HasPrefix(got, "T") {
		t.Errorf("after `kill -STOP %%1` the job is %q, want it still stopped", got)
	}
}

// The issue's own bar: `sleep`, `kill -STOP %1`, `kill -TERM %1` reports the
// job terminated and leaves nothing behind.
//
// `ps` again rather than `jobs`, for the reason at the top of this file. The
// notice is asserted on the **raw** bytes the terminal was given, as
// TestAKilledJobsNoticeNamesTheSignal is: a view with the escapes taken out
// could have normalized away the thing under test.
func TestAStoppedJobKilledIsReportedTerminated(t *testing.T) {
	control, screen := jobNoticeSession(t)
	stopAJob(t, control, screen)
	if got := jobState(t, control, screen); !strings.HasPrefix(got, "T") {
		t.Fatalf("the job is %q rather than stopped; nothing below grades anything", got)
	}

	// Everything drawn since the kill, waited out until the notice is in it.
	// `[1]  + ` is the notice and nothing else here. Since #4524 the notice
	// arrives on its own rather than at the prompt after the next command, so
	// no second line is typed to fetch it — jobNoticeAfter says why the
	// reading is a tail and not a seek.
	got := jobNoticeAfter(t, control, screen, "kill -TERM %1", "[1]  + ")
	const want = "[1]  + terminated  sleep 300"
	if !strings.Contains(got, want) {
		t.Errorf("after the kill the terminal was given\n%s\nwant a row %q",
			smoke.Readable(smoke.LastLines(got, 10)), want)
	}
	if state := jobState(t, control, screen); state != "" {
		t.Errorf("the process is %q afterwards, want it gone", state)
	}
}

// stopAJob starts a `sleep` in the background, keeps its pid, and stops it.
//
// `kill -STOP` rather than `^Z`, and it makes no difference which: measured
// 2026-09-25 on zsh 5.9.2, a job suspended by the terminal's own key answers
// `kill -0 %1`, `kill -WINCH %1` and `kill -TERM %1` exactly as one stopped
// by the explicit signal does. The explicit one is used here because it needs
// no foreground command and no raw-mode keystroke.
func stopAJob(t *testing.T, control *os.File, screen *smoke.Screen) {
	t.Helper()
	jobNoticeType(t, control, screen, "sleep 300 &")
	jobNoticeType(t, control, screen, "p=$!")
	// The fence jobState reads its answer out of. See there for why it is a
	// parameter rather than a word.
	jobNoticeType(t, control, screen, "q=QQ")
	t.Cleanup(func() {
		// The job outlives the shell in the rows where it was never killed,
		// so it is ended here rather than waited out. Continued first: a
		// process that is still stopped cannot act on a TERM on Linux, which
		// is the very fact this file is about.
		_, _ = control.WriteString("kill -CONT $p 2>/dev/null; kill -9 $p 2>/dev/null\n")
	})
	jobNoticeType(t, control, screen, "kill -STOP %1")
}

// jobState is what `ps` says about the job's process — `T` stopped, `S`
// sleeping, empty when there is no such process.
//
// Read through the shell because that is the only thing holding the pid, and
// the fence around the value is *spelled* as a parameter so that the typed
// line does not carry it: the terminal echoes what is typed, so a marker the
// input contains literally would be found in that echo and the row would pass
// for a shell that ran nothing. `${q}<` is typed and `QQ<` is what only an
// expansion can produce.
func jobState(t *testing.T, control *os.File, screen *smoke.Screen) string {
	t.Helper()
	// Waited out on the closing fence rather than read off a prompt: a job
	// notice can arrive unprompted between these lines (#4524), and a prompt
	// drawn under one is a prompt this read would otherwise mistake for the
	// one its own line produced. See jobNoticeAfter.
	got := jobNoticeAfter(t, control, screen, `print "${q}<$(ps -o stat= -p $p)>${q}"`, ">QQ")
	i := strings.Index(got, "QQ<")
	if i < 0 {
		t.Fatalf("no state was printed:\n%s", smoke.Readable(smoke.LastLines(got, 10)))
	}
	rest := got[i+len("QQ<"):]
	j := strings.Index(rest, ">QQ")
	if j < 0 {
		t.Fatalf("the state was never closed:\n%s", smoke.Readable(smoke.LastLines(got, 10)))
	}
	return strings.TrimSpace(rest[:j])
}
