// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
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
//
// **And every subshell around the sender dies of it too**, because each is a
// member of the same group with the shell's handled signals back at their
// defaults (#7245). Measured 2026-11-01 the same way:
//
//	trap 'echo T' USR1; ( (kill -USR1 0; echo in2); echo in1 ); echo out=$?
//	        T, out=158 in all five (ksh93 out=286); bash 5.3 writes the
//	        notice bare for the outer parentheses, dash the words alone
//	…with a third level, `( ( (…; echo in3); echo in2 ); echo in1 )`
//	        the same
//	trap 'echo T' USR1; ( trap 'echo S' USR1; (kill -USR1 0; echo in2); echo in1 )
//	        S, in1, T, out=0: the subshell that traps it runs its handler and
//	        carries on; ksh93 crashes on its own trap, and is left out
//	trap 'echo T' USR1; : | { (kill -USR1 0; echo in2); echo in1; }
//	        T, out=158 in bash and dash, whose last element is forked; T,
//	        in1, out=0 in zsh and ksh93, whose last element is the shell
//	( (kill -USR1 0; echo in2); echo in1 ); echo out=$?
//	        nothing: the whole group is killed by USR1
//
// Here, before, every enclosing body ran on and wrote `in1` (and `in2`), and
// bash wrote the notice located, from the outer parentheses.
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
		{
			"the subshell around it", trapped + `( (kill -USR1 0; echo in2); echo in1 ); echo out=$?`,
			"( ( kill -USR1 0; echo in2 ); echo in1 )",
			map[string]want{
				"bash": {"T\n" + died, "bare"},
				"zsh":  {"T\n" + died, ""},
				"dash": {"T\n" + died, "words"},
				"ksh":  {"T\nout=" + strconv.Itoa(256+usr1) + "\n", ""},
			},
		},
		{
			"two subshells around it",
			trapped + `( ( (kill -USR1 0; echo in3); echo in2 ); echo in1 ); echo out=$?`,
			"( ( ( kill -USR1 0; echo in3 ); echo in2 ); echo in1 )",
			map[string]want{
				"bash": {"T\n" + died, "bare"},
				"zsh":  {"T\n" + died, ""},
				"dash": {"T\n" + died, "words"},
				"ksh":  {"T\nout=" + strconv.Itoa(256+usr1) + "\n", ""},
			},
		},
		{
			// The notice is the inner parentheses', written by the outer
			// ones, and is not what this row is about: see unchecked.
			"a subshell around it with a handler of its own",
			trapped + `( trap 'echo S' USR1; (kill -USR1 0; echo in2); echo in1 ); echo out=$?`, "",
			map[string]want{
				"bash": {"S\nin1\nT\nout=0\n", unchecked},
				"zsh":  {"S\nin1\nT\nout=0\n", ""},
				"dash": {"S\nin1\nT\nout=0\n", "words"},
			},
		},
		{
			"a pipeline element around it", trapped + `: | { (kill -USR1 0; echo in2); echo in1; }; echo out=$?`, "",
			map[string]want{
				"bash": {"T\n" + died, unchecked},
				"zsh":  {"T\nin1\nout=0\n", ""},
				"dash": {"T\n" + died, unchecked},
				"ksh":  {"T\nin1\nout=0\n", ""},
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
		for name, script := range map[string]string{
			"untrapped":                         `(kill -USR1 0; echo in); echo out=$?`,
			"untrapped, with a subshell around": `( (kill -USR1 0; echo in2); echo in1 ); echo out=$?`,
		} {
			t.Run(dialect+"/"+name, func(t *testing.T) {
				ws, out, errs := runInAGroup(t, dialect, script)
				if !ws.Signaled() || ws.Signal() != syscall.SIGUSR1 {
					t.Errorf("ended %v, want killed by USR1", ws)
				}
				if out != "" || errs != "" {
					t.Errorf("wrote %q and %q, want nothing", out, errs)
				}
			})
		}
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

// **Under the monitor, a `( … )` or a background job is a process group of its
// own**, so a group kill inside it reaches that group and not the shell, whose
// handler does not run (#7246).
//
// Measured 2026-11-01, each shell in a process group of its own and no
// terminal, against bash 5.3.20 and bash 3.2.57, and on a pseudo-terminal
// against zsh 5.9.2 and dash, which agree with bash where they grant `set -m`
// and cannot be reached here because they do not grant it without one:
//
//	set -m; trap 'echo T' USR1; (kill -USR1 0; echo in); echo out=$?
//	        the bare notice, out=158, no T; here, before: T as well
//	…with `(set +m; …)` inside                   the same: the fork decides
//	trap 'echo T' USR1; (set -m; kill -USR1 0; echo in); echo out=$?
//	        T, out=158: forked without the monitor, in the shell's group
//	…with `kill -USR1 -$BASHPID`                 the first row; here,
//	        before: in, out=0
//	…with `kill -USR1 -$$`                        in, T, out=0: the shell's
//	        group, which the body is not in; here, before: T, out=158
//	set -m; trap 'echo T' USR1; ( (kill -USR1 0; echo in2); echo in1 )
//	        the bare notice for the outer parentheses, out=158, no T
//	set -m; trap 'echo T' USR1; (kill -USR1 0; echo in) & wait $!
//	        out=158, nothing else; here, before: T as well
//	set -m; (kill -USR1 0; echo in); echo out=$?   and TERM
//	        the notice, out=158 (143); here, before: nothing, the whole
//	        shell killed
//
// ksh93u+ runs a `( … )` in its own process, so there is no job to be a group
// and it writes T and out=286 as without the monitor; its background job is a
// group of its own, and writes no T.
func TestAJobForkedUnderTheMonitorIsAGroupOfItsOwn(t *testing.T) {
	usr1 := int(syscall.SIGUSR1)
	died := "out=" + strconv.Itoa(128+usr1) + "\n"
	diedKsh := "out=" + strconv.Itoa(256+usr1) + "\n"
	const trapped = `set -m; trap 'echo T' USR1; `
	for _, c := range []struct {
		name, dialect, script, out, notice, command string
	}{
		{
			"bash/to group 0", "bash", trapped + `(kill -USR1 0; echo in); echo out=$?`,
			died, "bare", "( kill -USR1 0; echo in )",
		},
		{
			"bash/the monitor turned off inside", "bash", trapped + `(set +m; kill -USR1 0; echo in); echo out=$?`,
			died, "bare", "( set +m; kill -USR1 0; echo in )",
		},
		{
			"bash/the monitor turned on inside", "bash",
			`trap 'echo T' USR1; (set -m; kill -USR1 0; echo in); echo out=$?`,
			"T\n" + died, "bare", "( set -m; kill -USR1 0; echo in )",
		},
		{
			"bash/to the body's own group", "bash", trapped + `(kill -USR1 -$BASHPID; echo in); echo out=$?`,
			died, "bare", "( kill -USR1 -$BASHPID; echo in )",
		},
		{
			"bash/to the shell's group", "bash", trapped + `(kill -USR1 -$$; echo in); echo out=$?`,
			"in\nT\nout=0\n", "", "",
		},
		{
			"bash/the subshell around it", "bash", trapped + `( (kill -USR1 0; echo in2); echo in1 ); echo out=$?`,
			died, "bare", "( ( kill -USR1 0; echo in2 ); echo in1 )",
		},
		{
			"bash/a background job", "bash", trapped + `(kill -USR1 0; echo in) & wait $!; echo out=$?`,
			died, "", "",
		},
		{
			"bash/untrapped TERM", "bash", `set -m; (kill -TERM 0; echo in); echo out=$?`,
			"out=143\n", unchecked, "",
		},
		{
			"ksh/to group 0", "ksh", trapped + `(kill -USR1 0; echo in); echo out=$?`,
			"T\n" + diedKsh, "", "",
		},
		{
			"ksh/to the shell's group", "ksh", trapped + `(kill -USR1 -$$; echo in); echo out=$?`,
			"T\n" + diedKsh, "", "",
		},
		{
			// The notice is `wait`'s, which is not this row's question.
			"ksh/a background job", "ksh", trapped + `(kill -USR1 0; echo in) & wait $!; echo out=$?`,
			diedKsh, unchecked, "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ws, out, errs := runInAGroup(t, c.dialect, c.script)
			if ws.Signaled() || ws.ExitStatus() != 0 {
				t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q (stderr %q)", out, c.out, errs)
			}
			checkGroupKillNotice(t, errs, c.notice, c.command)
		})
	}
	// Untrapped, the subshell dies of it and the shell, which is not in its
	// group, writes the located notice and carries on.
	t.Run("bash/untrapped", func(t *testing.T) {
		const command = "( kill -USR1 0; echo in )"
		ws, out, errs := runInAGroup(t, "bash", `set -m; `+command+`; echo out=$?`)
		if ws.Signaled() || ws.ExitStatus() != 0 {
			t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
		}
		if out != died {
			t.Errorf("wrote %q, want %q (stderr %q)", out, died, errs)
		}
		if !strings.Contains(errs, ": line 1: ") || !strings.Contains(errs, "User defined signal 1") ||
			!strings.HasSuffix(errs, command+"\n") {
			t.Errorf("wrote %q, want the located notice for %s", errs, command)
		}
	})
}

// **Under the monitor a pipeline is a group of its own in four dialects, and a
// `<(…)` in one** — the two splits #7246 left, each an axis
// (Semantics.MonitoredPipelineIsAGroupOfItsOwn, #7318;
// Semantics.MonitoredProcessSubstitutionIsAGroupOfItsOwn, #7306).
//
// Measured 2026-11-01 on a pseudo-terminal, where zsh and dash grant `set -m`,
// and with no terminal for bash and ksh93, each shell in a process group of
// its own, against bash 5.3.20, zsh 5.9.2, dash and ksh93u+ 2012-08-01:
//
//	set -m; trap 'echo T' USR1; : | (kill -USR1 0; echo in); echo out=$?
//	        bash, zsh, dash: out=158, no T; ksh93: T, out=286; here,
//	        before: T in every dialect
//	…the element first, `( … ) | /bin/cat`        bash: out=0, no T
//	…without `set -m`                              T in every column
//	…untrapped                                     bash: out=158; here,
//	        before: the whole shell killed
//	…`: | { kill -USR1 -$BASHPID; echo in k=$?; }`  bash: no such process,
//	        in k=1: a later element leads no group; here, before: k=0
//	…`{ kill -USR1 -$BASHPID; … } | /bin/cat`      bash: out=0, the
//	        first element leads the group; here, before: in k=0
//	set -m; cat <( p=$BASHPID; kill -0 -- -$p; echo k=$? )
//	        bash: no such process, k=1; zsh, with $sysparams[pid]: k=0;
//	        here, before: k=0 in both
//	set -m; trap 'echo T' USR1; cat <(kill -USR1 0; echo in); echo after
//	        bash, ksh93: T; zsh: no T; here, before: T in all three
//
// The notices are not this test's question: a pipeline element's is a gap of
// its own, and ksh93's for the `cat` its group kill also reached is #7238's.
func TestAMonitoredPipelineOrProcessSubstitutionIsAGroupInOneDialect(t *testing.T) {
	usr1 := int(syscall.SIGUSR1)
	died := "out=" + strconv.Itoa(128+usr1) + "\n"
	const trapped = `set -m; trap 'echo T' USR1; `
	const procSubKill = trapped + `/bin/cat <(kill -USR1 0; echo in); echo after`
	for _, c := range []struct {
		name, dialect, script, out string
		// terminal runs the shell on a pseudo-terminal, which is what zsh and
		// dash grant `set -m` on.
		terminal bool
	}{
		{"bash/the last element", "bash", trapped + `: | (kill -USR1 0; echo in); echo out=$?`, died, false},
		{"bash/the first element", "bash", trapped + `(kill -USR1 0; echo in) | /bin/cat; echo out=$?`, "out=0\n", false},
		{
			"bash/without the monitor", "bash", `trap 'echo T' USR1; : | (kill -USR1 0; echo in); echo out=$?`,
			"T\n" + died, false,
		},
		{"bash/untrapped", "bash", `set -m; : | (kill -USR1 0; echo in); echo out=$?`, died, false},
		{
			"bash/a later element's own number", "bash",
			trapped + `: | { kill -USR1 -$BASHPID; echo in k=$?; }; echo out=$?`, "in k=1\nout=0\n", false,
		},
		{
			"bash/the first element's own number", "bash",
			trapped + `{ kill -USR1 -$BASHPID; echo in k=$?; } | /bin/cat; echo out=$?`, "out=0\n", false,
		},
		{
			"ksh/the last element", "ksh", trapped + `: | (kill -USR1 0; echo in); echo out=$?`,
			"T\nout=" + strconv.Itoa(256+usr1) + "\n", false,
		},
		{"zsh/the last element", "zsh", trapped + `: | (kill -USR1 0; echo in); echo out=$?`, died, true},
		{"dash/the last element", "dash", trapped + `: | (kill -USR1 0; echo in); echo out=$?`, died, true},
		{
			"bash/a process substitution's own number", "bash",
			`set -m; /bin/cat <( p=$BASHPID; kill -0 -- -$p; echo k=$? ); echo outer $?`, "k=1\nouter 0\n", false,
		},
		{"bash/a process substitution's group kill", "bash", procSubKill, "T\nafter\n", false},
		{
			"zsh/a process substitution's own number", "zsh",
			`zmodload zsh/system; set -m; /bin/cat <( p=$sysparams[pid]; kill -0 -$p; echo k=$? ); echo outer $?`,
			"k=0\nouter 0\n", true,
		},
		{"zsh/a process substitution's group kill", "zsh", procSubKill, "after\n", true},
		{"ksh/a process substitution's group kill", "ksh", procSubKill, "T\nafter\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := runInAGroup
			if c.terminal {
				run = runOnATerminal
			}
			ws, out, errs := run(t, c.dialect, c.script)
			if ws.Signaled() || ws.ExitStatus() != 0 {
				t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q (stderr %q)", out, c.out, errs)
			}
		})
	}
}

// **A `$( … )` leads no process group, under the monitor or not**, so a kill
// aimed at its own number as a group is refused as one that is not there
// (#7324). Not a split: no shell forks it as a job.
//
// Measured 2026-11-01, bash 5.3.20 with no terminal and the shell in a process
// group of its own, zsh 5.9.2 on a pseudo-terminal, where it grants `set -m`:
//
//	set -m; echo $( p=$BASHPID; kill -0 -- -$p; echo k=$? ); echo outer $?
//	        bash: no such process, k=1, outer 0; zsh, with
//	        $sysparams[pid]: no such process, k=1; here, before: k=0 in both
//	…without `set -m`                       the same, and here too
//	…with `( … )` for `$( … )`              k=0 in both: a job's group
//	set -m; ( set -m; echo $( p=$BASHPID; kill -0 -- -$p; echo k=$? ); : )
//	        bash: no such process, k=1; here, before: k=0
//	…with `( … )` for `$( … )`              k=0: with job control back on
//	        inside, a `( … )` leads a group and a `$( … )` still none
//	set -m; trap 'echo T' USR1; echo "x$(kill -USR1 0; echo in)y"; echo out=$?
//	        bash: xy, T, out=0 — the shell's group, with the monitor as
//	        without it; here the same, before and after
//
// dash agrees, with the number read back from a child, and ksh93u+ forks no
// `$( … )` at all; neither has a parameter that names the body's own number,
// so neither is reachable here.
func TestACommandSubstitutionLeadsNoGroupUnderTheMonitor(t *testing.T) {
	bashRefusal := regexp.MustCompile(`^bash: line 1: kill: \(-[0-9]+\) - No such process\n$`)
	zshRefusal := regexp.MustCompile(`^zsh:kill:1: kill -[0-9]+ failed: no such process\n$`)
	const (
		bashOwn = `p=$BASHPID; kill -0 -- -$p; echo k=$?`
		zshOwn  = `p=$sysparams[pid]; kill -0 -$p; echo k=$?`
		zshSet  = `zmodload zsh/system; set -m; `
	)
	for _, c := range []struct {
		name, dialect, script, out string
		// refusal is the shape stderr must have, nil for nothing at all.
		refusal  *regexp.Regexp
		terminal bool
	}{
		{
			"bash/its own number", "bash", `set -m; echo $( ` + bashOwn + ` ); echo outer $?`,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/without the monitor", "bash", `echo $( ` + bashOwn + ` ); echo outer $?`,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a subshell's own number", "bash", `set -m; ( ` + bashOwn + ` ); echo outer $?`,
			"k=0\nouter 0\n", nil, false,
		},
		{
			"bash/inside a subshell that turns the monitor on", "bash",
			`set -m; ( set -m; echo $( ` + bashOwn + ` ); : ); echo outer $?`, "k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a subshell inside one that turns the monitor on", "bash",
			`set -m; ( set -m; ( ` + bashOwn + ` ); : ); echo outer $?`, "k=0\nouter 0\n", nil, false,
		},
		{
			"bash/a group kill to 0", "bash",
			`set -m; trap 'echo T' USR1; echo "x$(kill -USR1 0; echo in)y"; echo out=$?`, "xy\nT\nout=0\n", nil, false,
		},
		{
			"zsh/its own number", "zsh", zshSet + `echo $( ` + zshOwn + ` ); echo outer $?`,
			"k=1\nouter 0\n", zshRefusal, true,
		},
		{
			"zsh/a subshell's own number", "zsh", zshSet + `( ` + zshOwn + ` ); echo outer $?`,
			"k=0\nouter 0\n", nil, true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := runInAGroup
			if c.terminal {
				run = runOnATerminal
			}
			ws, out, errs := run(t, c.dialect, c.script)
			if ws.Signaled() || ws.ExitStatus() != 0 {
				t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q (stderr %q)", out, c.out, errs)
			}
			if c.refusal == nil && errs != "" {
				t.Errorf("wrote %q to stderr, want nothing", errs)
			}
			if c.refusal != nil && !c.refusal.MatchString(errs) {
				t.Errorf("wrote %q to stderr, want the refusal %s", errs, c.refusal)
			}
		})
	}
}

// **A body forked inside a forked body leads no process group, unless the
// body that forks it turned the monitor on itself** (#7329). A fork keeps the
// option and runs no job control under it, so the noun is the monitor of the
// shell that forks, and only the shell itself and a body that ran `set -m`
// after its own fork have one.
//
// Measured 2026-11-01, bash 5.3.20 with `$BASHPID` and 3.2.57 with the number
// read back from a child (`p=$(exec /bin/sh -c 'echo $PPID')`), with no
// terminal and the shell in a process group of its own, zsh 5.9.2 with
// `$sysparams[pid]` on a pseudo-terminal, which agree in every row:
//
//	set -m; ( ( p=<pid>; kill -0 -- -$p; echo k=$? ); : ); echo outer $?
//	        no such process, k=1; here, before: k=0
//	set -m; ( set -m; ( … ); : )                 k=0: a job again
//	( set -m; ( … ); : ), no `set -m` outside     k=0
//	set -m; ( set +m; ( … ); : )                 no such process, k=1
//	set -m; ( set +m; set -m; ( … ); : )         k=0
//	set -m; ( ( … ) & wait; : )                  k=1; here, before: k=0
//	set -m; ( { …; } | /bin/cat; : )             k=1; here, before: k=0
//	set -m; ( set -m; ( ( … ); : ); : )          k=1: the middle body did not
//	        turn it on; here, before: k=0
//	set -m; echo $( ( … ) )                      k=1; here, before: k=0
//	set -m; ( … ) & wait, and ( … ) | /bin/cat   k=0: the parentheses are
//	        the fork, not a body inside it
//
// bash keeps `m` in `$-` at every depth, and zsh drops it below the first.
// dash agrees on the first row and turns job control back on for none of the
// others: `set -m; ( set -m; ( … ); : )` is no such process there, with the
// number read back from a child, and `set -m; trap 'echo T' USR1; ( set -m; (
// kill -USR1 0; echo in2 ); echo in1 )` reaches both bodies, where bash and
// zsh write `in1` and out=0. Neither shape is reachable here in dash, which
// has no parameter for the body's own number, and the group kill is #7331's.
func TestABodyForkedInsideAForkedBodyLeadsAGroupOnlyUnderItsOwnMonitor(t *testing.T) {
	bashRefusal := regexp.MustCompile(`^bash: line 1: kill: \(-[0-9]+\) - No such process\n$`)
	zshRefusal := regexp.MustCompile(`^zsh:kill:1: kill -[0-9]+ failed: no such process\n$`)
	const (
		bashOwn = `p=$BASHPID; kill -0 -- -$p; echo k=$?`
		zshOwn  = `p=$sysparams[pid]; kill -0 -$p; echo k=$?`
		zshSet  = `zmodload zsh/system; `
		outer   = `; echo outer $?`
	)
	for _, c := range []struct {
		name, dialect, script, out string
		// refusal is the shape stderr must have, nil for nothing at all.
		refusal  *regexp.Regexp
		terminal bool
	}{
		{
			"bash/a subshell's subshell", "bash", `set -m; ( ( ` + bashOwn + ` ); : )` + outer,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/the monitor turned on again inside", "bash", `set -m; ( set -m; ( ` + bashOwn + ` ); : )` + outer,
			"k=0\nouter 0\n", nil, false,
		},
		{
			"bash/the monitor turned on inside alone", "bash", `( set -m; ( ` + bashOwn + ` ); : )` + outer,
			"k=0\nouter 0\n", nil, false,
		},
		{
			"bash/the monitor turned off inside", "bash", `set -m; ( set +m; ( ` + bashOwn + ` ); : )` + outer,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a background job inside", "bash", `set -m; ( ( ` + bashOwn + ` ) & wait; : )` + outer,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a pipeline inside", "bash", `set -m; ( { ` + bashOwn + `; } | /bin/cat; : )` + outer,
			"k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a body that did not turn it on itself", "bash",
			`set -m; ( set -m; ( ( ` + bashOwn + ` ); : ); : )` + outer, "k=1\nouter 0\n", bashRefusal, false,
		},
		{
			"bash/a background subshell", "bash", `set -m; ( ` + bashOwn + ` ) & wait` + outer,
			"k=0\nouter 0\n", nil, false,
		},
		{
			"bash/a subshell as a pipeline's first element", "bash", `set -m; ( ` + bashOwn + ` ) | /bin/cat` + outer,
			"k=0\nouter 0\n", nil, false,
		},
		{
			"zsh/a subshell's subshell", "zsh", zshSet + `set -m; ( ( ` + zshOwn + ` ); : )` + outer,
			"k=1\nouter 0\n", zshRefusal, true,
		},
		{
			"zsh/the monitor turned on again inside", "zsh",
			zshSet + `set -m; ( set -m; ( ` + zshOwn + ` ); : )` + outer, "k=0\nouter 0\n", nil, true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := runInAGroup
			if c.terminal {
				run = runOnATerminal
			}
			ws, out, errs := run(t, c.dialect, c.script)
			if ws.Signaled() || ws.ExitStatus() != 0 {
				t.Errorf("ended %v, want an exit at 0 (stderr %q)", ws, errs)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q (stderr %q)", out, c.out, errs)
			}
			if c.refusal == nil && errs != "" {
				t.Errorf("wrote %q to stderr, want nothing", errs)
			}
			if c.refusal != nil && !c.refusal.MatchString(errs) {
				t.Errorf("wrote %q to stderr, want the refusal %s", errs, c.refusal)
			}
		})
	}
}

// unchecked is a row whose stderr is not this test's question: a notice for
// a body other than the parentheses the shell ran — the inner parentheses,
// named by the ones around them, or a pipeline element — which is a gap of
// its own (#7310, #7311).
const unchecked = "unchecked"

// checkGroupKillNotice checks stderr for the shape of notice want names.
func checkGroupKillNotice(t *testing.T, errs, want, command string) {
	t.Helper()
	switch want {
	case unchecked:
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
	return runInAGroupWith(t, dialect, script, nil)
}

// runOnATerminal is runInAGroup with the shell on a pseudo-terminal of its
// own, as the leader of a session the terminal belongs to, which is where zsh
// and dash grant `set -m`. Only its input is the terminal, so what it writes
// is read back as runInAGroup reads it.
func runOnATerminal(t *testing.T, dialect, script string) (ws syscall.WaitStatus, out, errs string) {
	t.Helper()
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer control.Close()
	defer terminal.Close()
	return runInAGroupWith(t, dialect, script, func(cmd *exec.Cmd) {
		cmd.Stdin = terminal
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	})
}

// runInAGroupWith is runInAGroup with setup, when given, making the last
// changes to the command before it starts.
func runInAGroupWith(
	t *testing.T, dialect, script string, setup func(*exec.Cmd),
) (ws syscall.WaitStatus, out, errs string) {
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
	if setup != nil {
		setup(cmd)
	}
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
