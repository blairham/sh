// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// groupKillShell names the dialect a re-executed copy of this test binary runs
// as, and groupKillScript the script it runs: set, it means "you are the
// shell".
const (
	groupKillShell  = "SH_TEST_GROUP_KILL_SHELL"
	groupKillScript = "SH_TEST_GROUP_KILL_SCRIPT"
)

// **A subshell that signals the process group it shares with the shell dies
// of it**, and the shell runs its own handler before the next command
// (#7165).
//
// Measured 2026-11-01, each shell in a process group of its own, against bash
// 5.3.20, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2 and dash:
//
//	trap 'echo T' USR1; (kill -USR1 0; echo in); echo out=$?
//	        T, out=158 (ksh93 out=286); bash 5.3 writes `User defined
//	        signal 1: 30  ( kill -USR1 0; echo in )` first, bare, and dash
//	        `User defined signal 1: 30`; zsh and ksh93 write nothing
//	…with `kill -USR1 -$$`                       the same
//	(kill -USR1 0; echo in); echo out=$?         nothing: killed by USR1
//	trap 'echo T' USR1; { kill -USR1 0; echo in; }; echo out=$?
//	        T, in, out=0 — a group is the shell itself
//	trap 'echo T' USR1; (trap 'echo S' USR1; kill -USR1 0; echo in); echo out=$?
//	        S, in, T, out=0; ksh93 crashes on its own trap, and is left out
//
// This shell wrote `in` for every subshell row, and `T` after `out=0` in
// three dialects. Each signal number here is the platform's: 30 for USR1 on
// macOS, 10 on Linux, so the notice is checked for its shape and not its
// digits.
func TestASubshellThatSignalsItsOwnGroupDiesOfIt(t *testing.T) {
	if name := os.Getenv(groupKillShell); name != "" {
		refuseACoreDump()
		if syscall.Getpgrp() != os.Getpid() {
			// `kill 0` from the test's own group would reach `go test`.
			os.Exit(125)
		}
		sh := echoShells()[name]
		os.Exit(driver.MainArgs(sh, []string{sh.Name, "-c", os.Getenv(groupKillScript)}))
	}
	usr1 := int(syscall.SIGUSR1)
	died := "out=" + strconv.Itoa(128+usr1) + "\n"
	const trapped = `trap 'echo T' USR1; `
	type want struct {
		out string
		// notice is what stderr holds: "" for nothing, "bare" for the words
		// and the command with no location, "words" for the words alone.
		notice string
	}
	for _, c := range []struct {
		name, script, command string
		want                  map[string]want
	}{
		{
			"to group 0", trapped + `(kill -USR1 0; echo in); echo out=$?`, "( kill -USR1 0; echo in )",
			map[string]want{
				"bash": {"T\n" + died, "bare"},
				"zsh":  {"T\n" + died, ""},
				"dash": {"T\n" + died, "words"},
				"ksh":  {"T\nout=" + strconv.Itoa(256+usr1) + "\n", ""},
			},
		},
		{
			"to the group by its number", trapped + `(kill -USR1 -$$; echo in); echo out=$?`, "( kill -USR1 -$$; echo in )",
			map[string]want{
				"bash": {"T\n" + died, "bare"},
				"zsh":  {"T\n" + died, ""},
				"dash": {"T\n" + died, "words"},
				"ksh":  {"T\nout=" + strconv.Itoa(256+usr1) + "\n", ""},
			},
		},
		{
			"a group is the shell", trapped + `{ kill -USR1 0; echo in; }; echo out=$?`, "",
			map[string]want{
				"bash": {"T\nin\nout=0\n", ""},
				"zsh":  {"T\nin\nout=0\n", ""},
				"dash": {"T\nin\nout=0\n", ""},
				"ksh":  {"T\nin\nout=0\n", ""},
			},
		},
		{
			"a subshell with a handler of its own",
			trapped + `(trap 'echo S' USR1; kill -USR1 0; echo in); echo out=$?`, "",
			map[string]want{
				"bash": {"S\nin\nT\nout=0\n", ""},
				"zsh":  {"S\nin\nT\nout=0\n", ""},
				"dash": {"S\nin\nT\nout=0\n", ""},
			},
		},
	} {
		for dialect, w := range c.want {
			t.Run(dialect+"/"+c.name, func(t *testing.T) {
				ws, out, errs := runInAGroup(t, dialect, c.script)
				if ws.Signaled() || ws.ExitStatus() != 0 {
					t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
				}
				if out != w.out {
					t.Errorf("wrote %q, want %q (stderr %q)", out, w.out, errs)
				}
				checkGroupKillNotice(t, errs, w.notice, c.command)
			})
		}
	}
	// And untrapped, the whole group dies of it, the shell included, with
	// nothing written: no `in`, no `out=`, and no notice for the subshell
	// from a shell that is dead.
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		t.Run(dialect+"/untrapped", func(t *testing.T) {
			ws, out, errs := runInAGroup(t, dialect, `(kill -USR1 0; echo in); echo out=$?`)
			if !ws.Signaled() || ws.Signal() != syscall.SIGUSR1 {
				t.Errorf("ended %v, want killed by USR1", ws)
			}
			if out != "" || errs != "" {
				t.Errorf("wrote %q and %q, want nothing", out, errs)
			}
		})
	}
}

// A command killed by a signal the shell traps is reported with neither the
// location nor the process id, in bash 5.3 — the notice the subshell above
// gets. Measured 2026-11-01: `trap 'echo T' USR1; /bin/sh -c 'kill -USR1 $$'`
// is bare there, and the same line with USR2 trapped instead is the located
// notice with the process id. The pair holds the command fixed and moves only
// whether the shell traps the signal that killed it.
func TestACommandKilledByASignalTheShellTrapsIsReportedBare(t *testing.T) {
	const command = `/bin/sh -c 'kill -USR1 $$'`
	for _, c := range []struct {
		name, trap string
		bare       bool
	}{
		{"trapped", `trap 'echo T' USR1; `, true},
		{"another signal trapped", `trap 'echo T' USR2; `, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, errs := runInAGroup(t, "bash", c.trap+command)
			if c.bare {
				checkGroupKillNotice(t, errs, "bare", command)
				return
			}
			if !strings.HasPrefix(errs, "bash: ") || !strings.HasSuffix(errs, command+"\n") {
				t.Errorf("wrote %q, want the located notice for %s", errs, command)
			}
		})
	}
}

// checkGroupKillNotice checks stderr for the shape of notice want names.
func checkGroupKillNotice(t *testing.T, errs, want, command string) {
	t.Helper()
	switch want {
	case "":
		if errs != "" {
			t.Errorf("wrote %q to stderr, want nothing", errs)
		}
	case "bare":
		// The words padded to twenty-seven columns, then the command, and no
		// location or process id in front.
		words, rest, ok := strings.Cut(errs, "  ")
		if !ok || strings.HasPrefix(errs, "bash") || !strings.HasPrefix(words, "User defined signal 1") ||
			strings.TrimLeft(rest, " ") != command+"\n" || len(errs)-len(command)-1 < 27 {
			t.Errorf("wrote %q, want the bare notice for %s", errs, command)
		}
	case "words":
		if !strings.HasPrefix(errs, "User defined signal 1") || strings.Count(errs, "\n") != 1 ||
			strings.Contains(errs, "(") {
			t.Errorf("wrote %q, want the words for the signal alone", errs)
		}
	}
}

// runInAGroup runs this test binary as the dialect's shell running script, as
// the leader of a process group of its own, and reports how it ended and what
// it wrote to each stream.
func runInAGroup(t *testing.T, dialect, script string) (ws syscall.WaitStatus, out, errs string) {
	t.Helper()
	const deadline = 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestASubshellThatSignalsItsOwnGroupDiesOfIt$")
	cmd.Env = testenv.WithoutRaceExitPause(append(os.Environ(), groupKillShell+"="+dialect, groupKillScript+"="+script))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The whole group, so nothing the shell started outlives the test.
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	_ = cmd.Wait()
	if ctx.Err() != nil {
		t.Fatalf("%s: still running after %v", script, deadline)
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status")
	}
	return ws, o.String(), e.String()
}
