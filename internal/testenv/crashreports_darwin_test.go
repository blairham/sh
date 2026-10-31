// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv

import (
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"
	"testing"
)

// dieOfAnAbort asks a re-executed copy of this binary to die of SIGABRT before
// it does anything else.
const dieOfAnAbort = "SH_TEST_DIE_OF_AN_ABORT"

// The dying half, in init so it goes before TestMain: a child that reached Run
// would make a scratch home it never removes. The runtime's crash traceback
// ends by raising SIGABRT with its default action restored, which is a death by
// a core-dumping signal with nothing here to restore that disposition by hand.
func init() {
	if os.Getenv(dieOfAnAbort) != "" {
		debug.SetTraceback("crash")
		panic("dying of SIGABRT, as the test asked")
	}
}

// TestADeathBelowTheSuiteIsAnsweredHereAndStillEndsTheChild pins both halves
// of keepCrashReportsOut, as reached through Run: a child of the suite that
// dies of a core-dumping signal has its crash answered by this process, which
// is what keeps it from ReportCrash and out of ~/Library/Logs/DiagnosticReports,
// and the child is still killed by that signal, which is what every test that
// asserts a death reads.
//
// The count is how a test can see the report that did not happen. Looking in
// the Library would read a person's real state, and it reads zero anyway once
// a process name has its 25 reports.
func TestADeathBelowTheSuiteIsAnsweredHereAndStillEndsTheChild(t *testing.T) {
	before := crashesAnswered.Load()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), dieOfAnAbort+"=1")
	_ = cmd.Run()
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status for the child")
	}
	if !ws.Signaled() || ws.Signal() != syscall.SIGABRT {
		t.Fatalf("the child ended with status %#x, want killed by SIGABRT — "+
			"it did not die the way this case needs, so the count below would mean nothing", int(ws))
	}
	if got := crashesAnswered.Load() - before; got != 1 {
		t.Errorf("%d crashes answered here for one death below the suite, want 1 — "+
			"the crash went to the host's handler, which is ReportCrash writing a report "+
			"into the person's Library", got)
	}
}
