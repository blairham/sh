// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

// A pid, and the padding a short one is written with.
var pidRun = regexp.MustCompile(` +[0-9]{3,} `)

// TestAJobASignalEndedIsNamedBySignal pins the `jobs` row for a job a signal
// ended, and the notice a named `wait` writes over one. Measured 2026-10-02
// on bash 5.3.20 (#5389).
//
// The words are the host's — macOS writes the number after them and glibc
// does not — and USR1's number is the host's too, so the rows say both.
func TestAJobASignalEndedIsNamedBySignal(t *testing.T) {
	t.Parallel()
	term, hup, usr1 := "Terminated: 15", "Hangup: 1", "User defined signal 1: 30"
	if runtime.GOOS != "darwin" {
		term, hup, usr1 = "Terminated", "Hangup", "User defined signal 1"
	}
	// The words stand in a column 27 wide, both in the listing and in the
	// notice.
	term, hup, usr1 = fmt.Sprintf("%-26s", term), fmt.Sprintf("%-26s", hup), fmt.Sprintf("%-26s", usr1)
	usr1Status := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"
	for _, tc := range []struct{ src, want string }{
		{"/bin/sleep 1 & /bin/sleep 0.2; kill %1; /bin/sleep 0.2; jobs", "[1]+  " + term + " /bin/sleep 1\n"},
		{"/bin/sleep 1 & /bin/sleep 0.2; kill -HUP %1; /bin/sleep 0.2; jobs", "[1]+  " + hup + " /bin/sleep 1\n"},
		{"{ /bin/sleep 1; } & /bin/sleep 0.2; kill $!; /bin/sleep 0.2; jobs", "[1]+  " + term + " { /bin/sleep 1; }\n"},
		{
			"/bin/sh -c 'kill -USR1 $$' & wait $!; echo st=$?",
			"bash: line 1: PID " + usr1 + " /bin/sh -c 'kill -USR1 $$'\n" + usr1Status,
		},
		// TERM is the one a named wait leaves unsaid.
		{"/bin/sh -c 'kill -TERM $$' & wait $!; echo st=$?", "st=143\n"},
		// And a job that exited with the same status a signal would leave
		// was not killed, and is not reported.
		{"/bin/sh -c 'exit 158' & wait $!; echo st=$?", "st=158\n"},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if got := pidRun.ReplaceAllString(out, " PID "); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// A `wait` over a job killed by a signal the shell traps writes nothing, on
// every route into it, where the same job killed by a signal the shell does
// not trap is the located notice. Measured 2026-11-01 on bash 5.3.20, each
// shell in a process group of its own (#7164). The pair holds the job fixed
// and moves only whether the shell traps the signal it died of; the last two
// rows move the trap across the job's death, and it is the table at the
// `wait` that decides.
func TestAWaitOverAJobKilledByATrappedSignalSaysNothing(t *testing.T) {
	t.Parallel()
	usr1 := "User defined signal 1: 30"
	if runtime.GOOS != "darwin" {
		usr1 = "User defined signal 1"
	}
	const job = `/bin/sh -c 'kill -USR1 $$' &`
	notice := "bash: line 1: PID " + fmt.Sprintf("%-26s", usr1) + " /bin/sh -c 'kill -USR1 $$'\n"
	died := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"
	for _, tc := range []struct{ name, src, want string }{
		{"wait $!", "trap 'echo T' USR1; " + job + " wait $!; echo st=$?", died},
		{"another signal trapped", "trap 'echo T' USR2; " + job + " wait $!; echo st=$?", notice + died},
		{"wait %1", "trap 'echo T' USR1; " + job + " wait %1; echo st=$?", died},
		{"bare wait", "trap 'echo T' USR1; " + job + " wait; echo st=$?", "st=0\n"},
		{
			"trapped after it died",
			job + " /bin/sleep 0.3; trap 'echo T' USR1; wait $!; echo st=$?", died,
		},
		{
			"untrapped after it died",
			"trap 'echo T' USR1; " + job + " /bin/sleep 0.3; trap - USR1; wait $!; echo st=$?", notice + died,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), tc.src)
			if got := pidRun.ReplaceAllString(out, " PID "); got != tc.want {
				t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
