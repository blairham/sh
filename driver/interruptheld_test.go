// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// heldScript and heldDeferral carry a script and an answer for
// Semantics.InterruptWaitsForTheProgram into a re-executed test binary.
const (
	heldScript   = "SH_TEST_INTERRUPT_HELD_SCRIPT"
	heldDeferral = "SH_TEST_INTERRUPT_HELD_DEFERRAL"
)

// runHeldAsAShell is the shell half: it runs the script with the axis
// answered as asked, and never returns.
func runHeldAsAShell() {
	src := os.Getenv(heldScript)
	if src == "" {
		return
	}
	writeNoCore()
	sh := shell()
	n, _ := strconv.Atoi(os.Getenv(heldDeferral))
	sh.Semantics.InterruptWaitsForTheProgram = interp.InterruptDeferral(n)
	os.Exit(driver.MainArgs(sh, []string{"testsh", "-c", src}))
}

// heldRun re-executes this test binary as a shell running src and reports how
// it ended and what it wrote.
func heldRun(t *testing.T, name, src string, d interp.InterruptDeferral) (syscall.WaitStatus, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deathDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), heldScript+"="+src, heldDeferral+"="+strconv.Itoa(int(d)))
	cmd.WaitDelay = time.Second
	out, _ := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s: still running after %v", src, deathDeadline)
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("no wait status for the shell half")
	}
	return ws, string(out)
}

// **An interrupt that reaches the shell while a foreground program runs is
// held for the program, where the dialect holds one** (#5416). Measured
// 2026-10-02 under `-c`, the program sending SIGINT to the shell itself: bash
// 5.3.20, bash 3.2.57 and ksh93u+ carry on unless the program died of it too,
// and zsh 5.9.2, dash and BusyBox ash die where it lands. A builtin holds
// nothing. See interp.Semantics.InterruptWaitsForTheProgram.
func TestAnInterruptWaitsForTheForegroundProgram(t *testing.T) {
	runHeldAsAShell()
	const name = "TestAnInterruptWaitsForTheForegroundProgram"
	const survives = `/bin/sh -c "kill -INT \$PPID; sleep 0.2; exit 4"; echo survived $?`
	for _, c := range []struct {
		name string
		src  string
		d    interp.InterruptDeferral
		out  string // empty: the shell dies of SIGINT
	}{
		{"held, and dropped", survives, interp.InterruptWaitsForAnyProgram, "survived 4\n"},
		{"held for a lone program too", survives, interp.InterruptWaitsForALoneProgram, "survived 4\n"},
		{"in a function", `f() { /bin/sh -c "kill -INT \$PPID; sleep 0.1"; echo inf; }; f`, interp.InterruptWaitsForAnyProgram, "inf\n"},
		{"in a pipeline, by any program", `/bin/sh -c "kill -INT \$PPID; sleep 0.1" | cat; echo survived`, interp.InterruptWaitsForAnyProgram, "survived\n"},
		{"not in a pipeline, for a lone program", `/bin/sh -c "kill -INT \$PPID; sleep 0.1" | cat; echo survived`, interp.InterruptWaitsForALoneProgram, ""},
		{"not in a substitution", `x=$(/bin/sh -c "kill -INT \$PPID; sleep 0.1"); echo survived`, interp.InterruptWaitsForAnyProgram, ""},
		{"the program died of it too", `/bin/sh -c "kill -INT \$PPID; sleep 0.2; kill -INT \$\$; sleep 1"; echo survived`, interp.InterruptWaitsForAnyProgram, ""},
		{"not held at all", survives, interp.InterruptEndsTheShell, ""},
		{"not held where unanswered", survives, interp.InterruptDeferralUnspecified, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			ws, out := heldRun(t, name, c.src, c.d)
			if c.out == "" {
				if !ws.Signaled() || ws.Signal() != syscall.SIGINT {
					t.Errorf("%s: %v, wrote %q; want killed by SIGINT", c.src, ws, out)
				}
				return
			}
			if ws.Signaled() || ws.ExitStatus() != 0 || out != c.out {
				t.Errorf("%s: %v, wrote %q; want %q at 0", c.src, ws, out, c.out)
			}
		})
	}
}
