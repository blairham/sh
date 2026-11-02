// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
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
	// What the rows' output needs answered and none of them is about.
	sh.Semantics.SignalDeathStatusIsTwoFiftySix = interp.No
	sh.Semantics.ChildInterruptEndsTheScript = interp.No
	os.Exit(driver.MainArgs(sh, []string{"testsh", "-c", src}))
}

// heldRun re-executes this test binary as a shell running src and reports how
// it ended and what it wrote.
func heldRun(t *testing.T, name, src string, d interp.InterruptDeferral) (syscall.WaitStatus, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deathDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+name)
	cmd.Env = testenv.WithoutRaceExitPause(
		append(os.Environ(), heldScript+"="+src, heldDeferral+"="+strconv.Itoa(int(d))))
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
		// Nor in a `<(…)` body, which nothing waits for: the shell is in a
		// builtin, and every reference dies there at once.
		{"not in a <(…) body", `read x < <(/bin/sh -c "kill -INT \$PPID; sleep 0.2"); echo survived`, interp.InterruptWaitsForAnyProgram, ""},
		{"the program died of it too", `/bin/sh -c "kill -INT \$PPID; sleep 0.2; kill -INT \$\$; sleep 1"; echo survived`, interp.InterruptWaitsForAnyProgram, ""},
		// Inside parentheses, for a lone program (#5541): pending past the
		// program, cleared by the next one, and ending the shell when the
		// parentheses end.
		{"parentheses: the end of them ends the shell", `( /bin/sh -c "kill -INT \$PPID; sleep 0.1"; echo in $? ); echo after $?`, interp.InterruptWaitsForALoneProgram, "in 0\n" + diesMark},
		{"parentheses: the next program clears it", `( /bin/sh -c "kill -INT \$PPID; sleep 0.1"; /bin/sleep 0.1; echo in2 ); echo after $?`, interp.InterruptWaitsForALoneProgram, "in2\nafter 0\n"},
		{"parentheses: nested, the inner end", `( ( /bin/sh -c "kill -INT \$PPID; sleep 0.1"; echo inner ); echo outer ); echo after`, interp.InterruptWaitsForALoneProgram, "inner\n" + diesMark},
		{"not held at all", survives, interp.InterruptEndsTheShell, ""},
		{"not held where unanswered", survives, interp.InterruptDeferralUnspecified, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			ws, out := heldRun(t, name, c.src, c.d)
			if want, dies := strings.CutSuffix(c.out, diesMark); dies || c.out == "" {
				if !ws.Signaled() || ws.Signal() != syscall.SIGINT || out != want {
					t.Errorf("%s: %v, wrote %q; want %q and killed by SIGINT", c.src, ws, out, want)
				}
				return
			}
			if ws.Signaled() || ws.ExitStatus() != 0 || out != c.out {
				t.Errorf("%s: %v, wrote %q; want %q at 0", c.src, ws, out, c.out)
			}
		})
	}
}

// diesMark ends a row's wanted output where the shell writes that much and
// then dies of SIGINT.
const diesMark = "\x00dies"

// **Where the dialect does not hold an interrupt for the program, it still
// waits for the program before dying of it** (#7059). Measured 2026-10-30
// under `-c`, SIGINT sent to the shell's pid alone, from another process and
// from the program itself: zsh 5.9.2, dash and BusyBox ash die of it in every
// row below, but only once the program — or the `( … )`, pipeline,
// substitution or function call it stands in — is done, with the
// parentheses' `echo in` written first. A builtin holds nothing, so a shell in
// a loop of its own dies where the interrupt lands. This shell died at once in
// every row and left the program running.
//
// The program writes its marker after the interrupt and only then exits, so
// the marker being there once the shell has died is what says the shell
// waited for it. It writes nothing else, so nothing it leaves running can hold
// the output this test reads.
func TestAnInterruptEndsTheShellOnlyOnceTheProgramIsDone(t *testing.T) {
	runHeldAsAShell()
	const name = "TestAnInterruptEndsTheShellOnlyOnceTheProgramIsDone"
	dir := t.TempDir()
	for i, c := range []struct {
		name   string
		src    string
		out    string
		waited bool
	}{
		{"a program", `P; echo after`, "", true},
		{"parentheses run to their end", `( P; echo in ); echo after`, "in\n", true},
		{"a substitution", `x=$(P; echo in); echo "after $x"`, "", true},
		{"a pipeline", `P | cat >/dev/null; echo after`, "", true},
		{"a function", `f() { P; echo inf; }; f; echo after`, "", true},
		{"a <(…) body holds nothing", `read x < <(/bin/sh -c "exec 2>/dev/null; sleep 0.2; kill -INT $$; sleep 3; : >M"); echo after`, "", false},
		{"a builtin holds nothing", `/bin/sh -c "exec >/dev/null 2>&1; sleep 0.2; kill -INT $$; sleep 3; : >M" & while :; do :; done`, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			marker := filepath.Join(dir, strconv.Itoa(i))
			program := `/bin/sh -c "exec >/dev/null 2>&1; kill -INT $$; sleep 0.3; : >` + marker + `"`
			src := strings.ReplaceAll(strings.Replace(c.src, "P", program, 1), ">M", ">"+marker)
			ws, out := heldRun(t, name, src, interp.InterruptEndsTheShell)
			_, err := os.Stat(marker)
			waited := err == nil
			if !ws.Signaled() || ws.Signal() != syscall.SIGINT || out != c.out || waited != c.waited {
				t.Errorf("%s: %v, wrote %q, waited for the program %v; want %q, killed by SIGINT, waited %v",
					c.src, ws, out, waited, c.out, c.waited)
			}
		})
	}
}
