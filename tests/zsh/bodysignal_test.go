// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestASignalLeftAtItsDefaultEndsABackgroundBody: a background body that
// leaves a signal at a default that ends a process dies of it — at once,
// whatever program it is waiting for — with the job's status 128 plus the
// number, no EXIT trap, and its program left running. Where a fork would have
// exec'd the program instead, the program is what the signal reaches.
// Measured 2026-10-02 on zsh 5.9.2 under `-f -c` (#5355).
//
// USR1's number is the platform's: 30 on macOS, 10 on Linux.
//
// The program writes a marker after a delay, so whether it outlived the
// body is something the script can see rather than a process to look for.
//
// A row where the program runs on waits for the marker rather than for a
// fixed time (#6230). The program is an orphan by then, started by a fork and
// an exec of its own, and on a loaded runner it wrote its marker after a
// fixed 0.9s had already been spent: measured by running the rows under
// `taskpolicy -b` beside busy loops, every row that lost `survived` found the
// marker there later, from a fraction of a second to seconds on. A row where
// the program is ended keeps the fixed wait, because there a late program
// can only make the row pass, never fail.
func TestASignalLeftAtItsDefaultEndsABackgroundBody(t *testing.T) {
	const prog = `/bin/sh -c '/bin/sleep 0.6; : >marker'`
	const after = `; for i in {1..400}; do [[ -e marker ]] && break; /bin/sleep 0.05; done` +
		`; [[ -e marker ]] && print survived; true`
	const ended = `; /bin/sleep 0.9; [[ -e marker ]] && print survived; true`
	for _, tc := range []struct{ name, src, want string }{
		{
			"the body dies and its program runs on",
			`{ ` + prog + `; print after } & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + after,
			"143\nsurvived\n",
		},
		{
			"a killed body runs no EXIT trap",
			`{ trap 'print X' EXIT; ` + prog + `; print after } & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + after,
			"143\nsurvived\n",
		},
		{
			"the number is the signal's",
			`{ ` + prog + `; print after } & /bin/sleep 0.2; kill -USR1 $!; wait $!; print $?` + after,
			strconv.Itoa(128+int(syscall.SIGUSR1)) + "\nsurvived\n",
		},
		{
			"a function is a body too",
			`f() { ` + prog + ` }; f & /bin/sleep 0.2; kill -TERM %1; wait $!; print $?` + after,
			"143\nsurvived\n",
		},
		{
			"a body with no program dies of KILL",
			`{ trap 'print T' TERM; while :; do :; done } & /bin/sleep 0.2; kill -KILL $!; wait $!; print $?`,
			"137\n",
		},
		// At once, and not when the program is done: the wait is given up,
		// whether the program was the job's first or a later one.
		{
			"the first program's wait is given up",
			`typeset -F SECONDS; { ` + prog + `; print after } & /bin/sleep 0.2; kill -TERM $!; s=$SECONDS; wait $!; print $? $(( SECONDS - s < 0.3 ))`,
			"143 1\n",
		},
		{
			"a later program's wait is given up",
			`typeset -F SECONDS; { /usr/bin/true; ` + prog + `; print after } & /bin/sleep 0.2; kill -TERM $!; s=$SECONDS; wait $!; print $? $(( SECONDS - s < 0.3 ))`,
			"143 1\n",
		},
		{
			"an EXIT trap keeps the last command a shell's",
			`{ trap 'print X' EXIT; ` + prog + ` } & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + after,
			"143\nsurvived\n",
		},
		{
			"the end of an and-list is a last command",
			`true && ` + prog + ` & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + ended,
			"143\n",
		},
		// The exec'd last command: the fork has become the program, so the
		// signal is the program's and nothing writes the marker.
		{
			"the last command is the program",
			`{ true; ` + prog + ` } & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + ended,
			"143\n",
		},
		{
			"a pipeline of programs is its processes",
			prog + ` | /bin/cat & /bin/sleep 0.2; kill -TERM %1; wait; print done` + ended,
			"done\n",
		},
		// And the two that end nothing: a background job ignores INT, and a
		// trap set anywhere keeps the fork a shell.
		{
			"INT is ignored by a background job",
			`{ /bin/sleep 0.3; print after } & /bin/sleep 0.1; kill -INT $!; wait`,
			"after\n",
		},
		{
			"a trap keeps the last command from being exec'd",
			`{ trap 'print U' USR1; ` + prog + ` } & /bin/sleep 0.2; kill -TERM $!; wait $!; print $?` + after,
			"143\nsurvived\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if took := time.Since(start); took > 4*time.Second {
				t.Errorf("took %v: the kill was not prompt", took)
			}
		})
	}
}

// TestAKilledBodysProgramOutlivesTheShell: the program a killed body was
// running is not ended when the shell itself finishes either. Measured
// 2026-10-02 on zsh 5.9.2: the marker the program writes after the shell has
// exited is there (#5355).
func TestAKilledBodysProgramOutlivesTheShell(t *testing.T) {
	dir := t.TempDir()
	runZsh(t, dir, `{ /bin/sh -c '/bin/sleep 0.5; : >marker'; print after } & /bin/sleep 0.2; kill -TERM $!`)
	// Waited for rather than slept for, as the rows above are (#6230): an
	// orphan on a loaded runner writes its marker late, not never.
	var err error
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err = os.Stat(filepath.Join(dir, "marker")); err == nil {
			return
		}
	}
	t.Errorf("the program did not outlive the shell: %v", err)
}
