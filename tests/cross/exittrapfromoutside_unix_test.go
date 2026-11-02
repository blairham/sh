// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// outsideDeathShell names the dialect a re-executed copy of this test binary
// runs as, and outsideDeathScript the script it runs: set, it means "you are
// the shell".
const (
	outsideDeathShell  = "SH_TEST_OUTSIDE_DEATH_SHELL"
	outsideDeathScript = "SH_TEST_OUTSIDE_DEATH_SCRIPT"
)

// What each dialect does with its EXIT trap when another process sends it an
// untrapped fatal signal, through the front end that owns the process (#7054).
//
// Measured 2026-10-29, each shell in a process group of its own, `trap 'echo
// "bye $?"' EXIT; (exit 7)` and then an external `sleep`, a `read` on a pipe or
// a busy loop, and `/bin/kill` from another process:
//
//	                 TERM                  HUP
//	bash 5.3, 3.2    bye 7, killed by it   bye 7, killed by it
//	ksh93u+          bye 7, killed by it   bye 7, killed by it
//	zsh 5.9.2        killed, nothing       bye 1, exit 1
//	dash             killed, nothing       killed, nothing
//	BusyBox ash      killed, nothing       killed, nothing
//
// BusyBox ash is the pinned alpine image internal/oracle reaches, the shell
// started under `setsid` with the container under `--init`, so that it is not
// PID 1 and its default dispositions stand. It is ExitTrapRunsOnSignalDeath
// and HangupIsAnOrderlyExit reached from outside: every column answers as it
// does for a signal the script sends itself.
func TestADialectsExitTrapAnswersAFatalSignalFromOutside(t *testing.T) {
	if name := os.Getenv(outsideDeathShell); name != "" {
		refuseACoreDump()
		sh := echoShells()[name]
		os.Exit(driver.MainArgs(sh, []string{sh.Name, "-c", os.Getenv(outsideDeathScript)}))
	}
	type ending struct {
		// sig is the signal the shell must die of, or 0 for an exit at
		// status.
		sig    syscall.Signal
		status int
		out    string
	}
	killed := func(sig syscall.Signal, out string) ending { return ending{sig: sig, out: out} }
	for _, c := range []struct {
		name string
		sig  syscall.Signal
		want map[string]ending
	}{
		{"TERM", syscall.SIGTERM, map[string]ending{
			"bash": killed(syscall.SIGTERM, "bye 7\n"),
			"ksh":  killed(syscall.SIGTERM, "bye 7\n"),
			"zsh":  killed(syscall.SIGTERM, ""),
			"dash": killed(syscall.SIGTERM, ""),
			"ash":  killed(syscall.SIGTERM, ""),
		}},
		{"HUP", syscall.SIGHUP, map[string]ending{
			"bash": killed(syscall.SIGHUP, "bye 7\n"),
			"ksh":  killed(syscall.SIGHUP, "bye 7\n"),
			"zsh":  {status: 1, out: "bye 1\n"},
			"dash": killed(syscall.SIGHUP, ""),
			"ash":  killed(syscall.SIGHUP, ""),
		}},
	} {
		for name, want := range c.want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, out := diesFromOutside(t, name, c.sig, outsideDeathTrapAt7)
				switch {
				case want.sig != 0 && (!ws.Signaled() || ws.Signal() != want.sig):
					t.Errorf("ended %v, want killed by %v", ws, want.sig)
				case want.sig == 0 && (ws.Signaled() || ws.ExitStatus() != want.status):
					t.Errorf("ended %v, want an exit at %d", ws, want.status)
				}
				if out != want.out {
					t.Errorf("wrote %q, want %q", out, want.out)
				}
			})
		}
	}
}

// zsh's answer to a HUP from another process while it waits inside a function
// call, and while it has nothing but `zshexit` to run (#7065, #7066).
//
// Measured 2026-10-30 on zsh 5.9.2 under `-f`, in a session of its own, with
// `/bin/kill -HUP` from another process while the shell waits on `/bin/sleep
// 3`: every row exits 1, and writes what the right-hand column says.
//
//	zshexit() { echo "hook $?"; }; WAIT                              hook 1
//	h() { echo hook; }; zshexit_functions=(h); WAIT                  hook
//	trap 'echo top' EXIT; f() { WAIT; }; f                           nothing
//	trap 'echo top' EXIT; f() { trap 'echo own' EXIT; WAIT; }; f     own
//	trap 'echo top' EXIT; zshexit() { echo hook; }; f() { WAIT; }; f hook
//
// It is the HUP the shell sends itself, reached from outside: the call's own
// trap runs, the top level's that it held for the call never does, and the
// hook runs either way. This shell exited 1 having run nothing for the first,
// second and last, and ran the top level's trap after the call's in the
// fourth.
func TestAHangupFromOutsideRunsTheHookAndOnlyTheCallsOwnTrap(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"`
	for _, c := range []struct{ name, script, out string }{
		{"only an exit hook", `zshexit() { echo "hook $?"; }; ` + wait + `; echo after`, "hook 1\n"},
		{"only an exit hook's list", `h() { echo hook; }; zshexit_functions=(h); ` + wait + `; echo after`, "hook\n"},
		{"in a call holding the trap", `trap 'echo top' EXIT; f() { ` + wait + `; echo in; }; f; echo after`, ""},
		{
			"in a call with a trap of its own",
			`trap 'echo top' EXIT; f() { trap 'echo own' EXIT; ` + wait + `; echo in; }; f; echo after`, "own\n",
		},
		{
			"in a call, with an exit hook",
			`trap 'echo top' EXIT; zshexit() { echo hook; }; f() { ` + wait + `; echo in; }; f; echo after`, "hook\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, "zsh", syscall.SIGHUP, c.script)
			if ws.Signaled() || ws.ExitStatus() != 1 {
				t.Errorf("ended %v, want an exit at 1", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// What each dialect does with a fatal signal from another process while a
// `function name { … }` call waits, with an EXIT trap set (#7067).
//
// Measured 2026-10-31, each shell in a session of its own, TERM from
// `/bin/kill` while the call waits on `/bin/sleep 3` or on a `read` of a fifo
// nobody writes:
//
//	trap 'echo "bye $?"' EXIT; function f { WAIT; echo inf; }; f; echo "after $?"
//	    ksh93u+               after 271, bye 0, exit 0
//	    bash 5.3.20, 3.2.57   bye 0, killed by TERM
//	    zsh 5.9.2             nothing, killed by TERM
//	the same with f() { … }   ksh93u+: bye 271, killed by TERM
//	trap 'echo x' EXIT && trap - EXIT; function f { WAIT; echo inf; }; f; echo "after $?"
//	    ksh93u+               after 271, exit 0
//	function f { trap 'echo "fexit $?"' EXIT; (exit 5); read x <&3; echo inf; }; f; echo "after $?"
//	    ksh93u+               fexit 5, after 271, exit 0
//
// ksh93 ends the call and goes on, which is
// Semantics.SignalDeathEndsAtAKeywordFunctionCall read through each preset
// and the front end that owns the process. This shell's ksh ended the process
// where the signal landed in the first and third, with nothing run: the EXIT
// trap was set aside for the call, so nothing was handed over. The `read` is
// the row where the call's own trap reads what the shell held before a wait
// the signal gave up, rather than what giving it up left.
func TestAFatalSignalFromOutsideEndsAKeywordCallInOneDialect(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"`
	const trap = `trap 'echo "bye $?"' EXIT; `
	for _, c := range []struct {
		dialect, name, script string
		sig                   syscall.Signal
		out                   string
	}{
		{"ksh", "a keyword call", trap + `function f { ` + wait + `; echo inf; }; f; echo "after $?"`, 0, "after 271\nbye 0\n"},
		{"bash", "a keyword call", trap + `function f { ` + wait + `; echo inf; }; f; echo "after $?"`, syscall.SIGTERM, "bye 0\n"},
		{"zsh", "a keyword call", trap + `function f { ` + wait + `; echo inf; }; f; echo "after $?"`, syscall.SIGTERM, ""},
		{"ksh", "a POSIX-form call", trap + `f() { ` + wait + `; echo inf; }; f; echo "after $?"`, syscall.SIGTERM, "bye 271\n"},
		// Set and taken away in one list, so that no EXIT trap ever stood
		// between two statements for the front end to hear about: what it
		// hears is the call.
		{
			"ksh", "a trap that no longer stands",
			`trap 'echo x' EXIT && trap - EXIT; function f { ` + wait + `; echo inf; }; f; echo "after $?"`,
			0, "after 271\n",
		},
		{
			"ksh", "a read in a call with its own trap",
			`/usr/bin/mkfifo "$READY.fifo"; exec 3<>"$READY.fifo"; ` +
				`function f { trap 'echo "fexit $?"' EXIT; /bin/sh -c 'sleep 0.3; printf r >"$1"' sh "$READY" & ` +
				`(exit 5); read x <&3; echo inf; }; f; echo "after $?"`,
			0, "fexit 5\nafter 271\n",
		},
	} {
		t.Run(c.dialect+"/"+c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, c.dialect, syscall.SIGTERM, c.script)
			switch {
			case c.sig != 0 && (!ws.Signaled() || ws.Signal() != c.sig):
				t.Errorf("ended %v, want killed by %v", ws, c.sig)
			case c.sig == 0 && (ws.Signaled() || ws.ExitStatus() != 0):
				t.Errorf("ended %v, want an exit at 0", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// What each dialect does with a fatal signal from another process while a
// `function name { … }` call waits, with a trap on that signal set before the
// call and no EXIT trap anywhere (#7156, #7157).
//
// Measured 2026-11-01, each shell in a session of its own, TERM from
// `/bin/kill` while the call waits on `/bin/sleep 3`:
//
//	trap 'echo term' TERM; function f { WAIT; echo inf; }; f; echo "after $?"
//	    ksh93u+               after 271, exit 0
//	    bash 5.3.20, zsh 5.9.2  term, inf, after 0, exit 0
//	trap 'echo t' TERM; trap - TERM; function f { WAIT; echo inf; }; f; echo "after $?"
//	    ksh93u+               after 271, exit 0
//	    bash 5.3.20, zsh 5.9.2  killed by TERM
//	the same with f() { … }   ksh93u+: killed by TERM
//	trap '' TERM; function f { WAIT; echo inf; }; f; echo "after $?"
//	    ksh93u+, bash, zsh    inf, after 0, exit 0
//
// ksh93 goes on catching a signal it has ever had a trap for, and the call is
// the boundary its death stops at, which is
// Semantics.SignalDeathEndsAtAKeywordFunctionCall read through each preset
// and the front end that owns the process; an ignore is not taken from the
// call with the handlers. This shell's ksh died of TERM in the first, second
// and last: the call's trap table had reset the signal, and nothing had the
// front end hand it over.
func TestASignalTheShellEverTrappedFromOutsideEndsAKeywordCallInOneDialect(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"`
	// The ignore leaves the shell waiting for the program, so it is short.
	const short = `/bin/sh -c 'printf r >"$1"; exec sleep 2' sh "$READY"`
	const call = `function f { ` + wait + `; echo inf; }; f; echo "after $?"`
	for _, c := range []struct {
		dialect, name, script string
		// sig is the signal the shell must die of, or 0 for an exit at 0.
		sig syscall.Signal
		out string
	}{
		{"ksh", "a handler standing", `trap 'echo term' TERM; ` + call, 0, "after 271\n"},
		{"ksh", "a trap taken away", `trap 'echo t' TERM; trap - TERM; ` + call, 0, "after 271\n"},
		{
			"ksh", "the POSIX form",
			`trap 'echo t' TERM; trap - TERM; f() { ` + wait + `; echo inf; }; f; echo "after $?"`,
			syscall.SIGTERM, "",
		},
		{
			"ksh", "an ignore", `trap '' TERM; function f { ` + short + `; echo inf; }; f; echo "after $?"`,
			0, "inf\nafter 0\n",
		},
		{"bash", "a trap taken away", `trap 'echo t' TERM; trap - TERM; ` + call, syscall.SIGTERM, ""},
		{"zsh", "a trap taken away", `trap 'echo t' TERM; trap - TERM; ` + call, syscall.SIGTERM, ""},
	} {
		t.Run(c.dialect+"/"+c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, c.dialect, syscall.SIGTERM, c.script)
			switch {
			case c.sig != 0 && (!ws.Signaled() || ws.Signal() != c.sig):
				t.Errorf("ended %v, want killed by %v", ws, c.sig)
			case c.sig == 0 && (ws.Signaled() || ws.ExitStatus() != 0):
				t.Errorf("ended %v, want an exit at 0", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// What each dialect does with a fatal signal from another process that lands
// while a `( … )` or `$( … )` waits for a program (#7068).
//
// Measured 2026-10-31, each shell in a session of its own, TERM from
// `/bin/kill` in another process while the `sleep` runs:
//
//	trap 'echo "bye $?"' EXIT; (exit 7); ( /bin/sleep 2; echo in ); echo after
//	    ksh93u+               in, bye 0, killed by TERM at 2 s
//	    bash 5.3.20           bye 7, killed by TERM at once
//	    zsh 5.9.2             killed by TERM at once, nothing run
//	trap 'echo "bye $?"' EXIT; (exit 7); x=$(/bin/sleep 2; exit 3); echo after
//	    ksh93u+               bye 3, killed by TERM at 2 s
//	    bash 5.3.20           bye 7, killed by TERM at once
//
// ksh93 runs both in its own process and waits them out, which is
// Semantics.SignalDeathWaitsForAnUnforkedSubshell read through each preset and
// the front end that owns the process. This shell's ksh died at once in both
// with `bye 7`. The program here outlives the signal by longer than the front
// end waits for a death it hands over to be taken, so a shell that waits it
// out has to have taken the death first, or the front end would end the
// process with nothing run.
func TestADeathFromOutsideWaitsForAnUnforkedSubshellInOneDialect(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 2.5' sh "$READY"`
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, c := range []struct {
		dialect, name, script, out string
	}{
		{"ksh", "parentheses", trap + `( ` + wait + `; echo in ); echo after`, "in\nbye 0\n"},
		{"ksh", "a substitution", trap + `x=$(` + wait + `; exit 3); echo after`, "bye 3\n"},
		{"bash", "parentheses", trap + `( ` + wait + `; echo in ); echo after`, "bye 7\n"},
		{"bash", "a substitution", trap + `x=$(` + wait + `; exit 3); echo after`, "bye 7\n"},
		{"zsh", "parentheses", trap + `( ` + wait + `; echo in ); echo after`, ""},
	} {
		t.Run(c.dialect+"/"+c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, c.dialect, syscall.SIGTERM, c.script)
			if !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
				t.Errorf("ended %v, want killed by TERM", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// What each dialect's EXIT trap reads for an INT from another process that
// lands while a `( … )` waits for a program (#7159).
//
// Measured 2026-11-01, each shell in a session of its own, INT from
// `/bin/kill` in another process 0.4 s in, `trap 'echo "bye $?"' EXIT; (exit
// 7)` in front of each line:
//
//	( /bin/sleep 2; echo in ); echo after
//	    ksh93u+               in, bye 0, killed by INT at 2 s
//	    bash 5.3.20           in, after, bye 0, exit 0
//	    zsh 5.9.2             in, killed by INT at 2 s
//	( /bin/sleep 2; exit 3 ); echo after
//	    ksh93u+               bye 3, killed by INT at 2 s
//	( ( /bin/sleep 2; echo inner ); echo outer ); echo after
//	    ksh93u+               inner, bye 258, killed by INT at 2 s
//	    the same with TERM    inner, bye 271, killed by TERM at 2 s
//
// ksh93 reads what the parentheses left, or 256 plus the number where the
// outer ones ended as killed, for INT as for TERM. This shell's ksh read 130
// in all three for INT: the interrupt is held for the program by a route of
// its own, and ended the shell as the parentheses ended.
func TestAnInterruptFromOutsideHeldForParenthesesInOneDialect(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 2.5' sh "$READY"`
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, c := range []struct {
		dialect, name, script string
		sig                   syscall.Signal
		// dies is the signal the shell must die of, or 0 for an exit at 0.
		dies syscall.Signal
		out  string
	}{
		{
			"ksh", "INT, parentheses", trap + `( ` + wait + `; echo in ); echo after`,
			syscall.SIGINT, syscall.SIGINT, "in\nbye 0\n",
		},
		{
			"ksh", "INT, parentheses that exit", trap + `( ` + wait + `; exit 3 ); echo after`,
			syscall.SIGINT, syscall.SIGINT, "bye 3\n",
		},
		{
			"ksh", "INT, nested parentheses",
			trap + `( ( ` + wait + `; echo inner ); echo outer ); echo after`,
			syscall.SIGINT, syscall.SIGINT, "inner\nbye 258\n",
		},
		{
			"ksh", "TERM, nested parentheses",
			trap + `( ( ` + wait + `; echo inner ); echo outer ); echo after`,
			syscall.SIGTERM, syscall.SIGTERM, "inner\nbye 271\n",
		},
		{
			"bash", "INT, parentheses", trap + `( ` + wait + `; echo in ); echo after`,
			syscall.SIGINT, 0, "in\nafter\nbye 0\n",
		},
		{
			"zsh", "INT, parentheses", trap + `( ` + wait + `; echo in ); echo after`,
			syscall.SIGINT, syscall.SIGINT, "in\n",
		},
	} {
		t.Run(c.dialect+"/"+c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, c.dialect, c.sig, c.script)
			switch {
			case c.dies != 0 && (!ws.Signaled() || ws.Signal() != c.dies):
				t.Errorf("ended %v, want killed by %v", ws, c.dies)
			case c.dies == 0 && (ws.Signaled() || ws.ExitStatus() != 0):
				t.Errorf("ended %v, want an exit at 0", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// What each dialect's EXIT trap reads for a fatal signal from another process
// that lands while the shell waits for a pipeline (#7160).
//
// Measured 2026-11-01, each shell in a session of its own, TERM from
// `/bin/kill` in another process while the `sleep` runs:
//
//	trap 'echo "bye $?"' EXIT; (exit 7); /bin/sleep 2 | /bin/cat; echo after
//	    bash 5.3.20           bye 7, killed by TERM at once
//	    ksh93u+, bash 3.2.57  bye 0, killed by TERM at once
//	    zsh 5.9.2, dash       killed by TERM at once, nothing run
//	the same with (exit 5) before the pipeline
//	    bash 5.3.20           bye 5
//	    ksh93u+               bye 0
//
// bash 5.3 shows the status the pipeline began with and ksh93 shows 0, which
// is Semantics.DeathDuringAPipelineShowsTheStatusBeforeIt read through each
// preset and the front end that owns the process. This shell's bash read 0 for
// the first and 7 for the second, and its ksh 7 for the second.
func TestADeathFromOutsideDuringAPipelineInOneDialect(t *testing.T) {
	const wait = `/bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"`
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	for _, c := range []struct {
		dialect, name, script, out string
	}{
		{"bash", "a program last", trap + wait + ` | /bin/cat; echo after`, "bye 7\n"},
		{"bash", "after another status", trap + `(exit 5); ` + wait + ` | /bin/cat; echo after`, "bye 5\n"},
		{"bash", "a subshell last", trap + `: | ( ` + wait + `; echo in ); echo after`, "bye 7\n"},
		{"ksh", "a program last", trap + wait + ` | /bin/cat; echo after`, "bye 0\n"},
		{"ksh", "after another status", trap + `(exit 5); ` + wait + ` | /bin/cat; echo after`, "bye 0\n"},
		{"ksh", "a read last", trap + wait + ` | read x; echo after`, "bye 0\n"},
		{"zsh", "a program last", trap + wait + ` | /bin/cat; echo after`, ""},
	} {
		t.Run(c.dialect+"/"+c.name, func(t *testing.T) {
			ws, out := diesFromOutside(t, c.dialect, syscall.SIGTERM, c.script)
			if !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
				t.Errorf("ended %v, want killed by TERM", ws)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// outsideDeathTrapAt7 is the script TestADialectsExitTrapAnswersAFatalSignalFromOutside
// measures: an EXIT trap, `$?` at 7, and a program the shell waits for.
const outsideDeathTrapAt7 = `trap 'echo "bye $?"' EXIT; (exit 7); /bin/sh -c 'printf r >"$1"; exec sleep 30' sh "$READY"; echo after`

// diesFromOutside runs this test binary as the dialect's shell in a process
// group of its own, running script — which must start a program that writes
// to `$READY` once it runs, and wait for it — sends it sig from here, and
// reports how it ended and what it wrote.
func diesFromOutside(t *testing.T, dialect string, sig syscall.Signal, script string) (syscall.WaitStatus, string) {
	t.Helper()
	const deadline = 30 * time.Second
	dir := t.TempDir()
	ready, logPath := filepath.Join(dir, "ready"), filepath.Join(dir, "out")
	cmd := exec.Command(os.Args[0], "-test.run=^TestADialectsExitTrapAnswersAFatalSignalFromOutside$")
	cmd.Env = testenv.WithoutRaceExitPause(
		append(os.Environ(), outsideDeathShell+"="+dialect, outsideDeathScript+"="+script, "READY="+ready),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The whole group, so the `sleep` the shell leaves behind goes too.
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	readLog := func() string {
		b, _ := os.ReadFile(logPath)
		return string(b)
	}
	until := time.Now().Add(deadline)
	for {
		if b, err := os.ReadFile(ready); err == nil && len(b) > 0 {
			break
		}
		select {
		case <-reaped:
			t.Fatalf("the shell ended before it was ready: %q", readLog())
		default:
		}
		if time.Now().After(until) {
			t.Fatalf("the shell never started its program: %q", readLog())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reaped:
	case <-time.After(deadline):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
		t.Fatalf("the shell was still running %v after %v: %q", deadline, sig, readLog())
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status")
	}
	return ws, readLog()
}

// What each dialect does with an `exit` in the EXIT trap of a death, and with
// a fatal signal that lands while the EXIT trap runs (#7063, #7064).
//
// Measured 2026-10-30, each shell in a process group of its own:
//
//	                 trap 'echo bye; exit 5' EXIT;   trap 'echo bye; kill -USR1 $$;
//	                 kill -TERM $$                   echo after' EXIT; exit 3
//	bash 5.3, 3.2    bye, killed by TERM             bye, killed by USR1
//	ksh93u+          bye, exit 5                     bye, exit 3
//	zsh 5.9.2        killed by TERM, nothing         bye, killed by USR1
//	dash             killed by TERM, nothing         bye, killed by USR1
//	BusyBox ash      killed by TERM, nothing         bye, killed by USR1
//
// BusyBox ash is the pinned alpine image internal/oracle reaches, started
// under `--init` so that it is not PID 1. Both are the dialect answering
// Semantics.ExitInTheExitTrapOverridesASignalDeath and
// Semantics.FatalSignalInTheExitTrapEndsOnlyTheTrap, read here through each
// preset and the front end that owns the process.
func TestADialectsExitTrapDecidesWhetherASignalEndsTheShell(t *testing.T) {
	type ending struct {
		sig    syscall.Signal
		status int
		out    string
	}
	killed := func(sig syscall.Signal, out string) ending { return ending{sig: sig, out: out} }
	const (
		exitInTheTrap   = `trap 'echo bye; exit 5' EXIT; kill -TERM $$; echo after`
		signalInTheTrap = `trap 'echo bye; kill -USR1 $$; echo after' EXIT; exit 3`
	)
	for _, c := range []struct {
		name, script string
		want         map[string]ending
	}{
		{"an exit in the trap of a death", exitInTheTrap, map[string]ending{
			"bash": killed(syscall.SIGTERM, "bye\n"),
			"ksh":  {status: 5, out: "bye\n"},
			"zsh":  killed(syscall.SIGTERM, ""),
			"dash": killed(syscall.SIGTERM, ""),
			"ash":  killed(syscall.SIGTERM, ""),
		}},
		{"a fatal signal in the trap of an exit", signalInTheTrap, map[string]ending{
			"bash": killed(syscall.SIGUSR1, "bye\n"),
			"ksh":  {status: 3, out: "bye\n"},
			"zsh":  killed(syscall.SIGUSR1, "bye\n"),
			"dash": killed(syscall.SIGUSR1, "bye\n"),
			"ash":  killed(syscall.SIGUSR1, "bye\n"),
		}},
	} {
		for name, want := range c.want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, out := endsOnItsOwn(t, name, c.script)
				switch {
				case want.sig != 0 && (!ws.Signaled() || ws.Signal() != want.sig):
					t.Errorf("ended %v, want killed by %v", ws, want.sig)
				case want.sig == 0 && (ws.Signaled() || ws.ExitStatus() != want.status):
					t.Errorf("ended %v, want an exit at %d", ws, want.status)
				}
				if out != want.out {
					t.Errorf("wrote %q, want %q", out, want.out)
				}
			})
		}
	}
}

// What each dialect does with an untrapped SEGV it sends itself, with an EXIT
// trap set (#7069).
//
// Measured 2026-10-30, each shell in a process group of its own:
//
//	                 trap 'echo "bye $?"' EXIT;   trap 'echo in; kill -SEGV $$;
//	                 (exit 7); kill -SEGV $$      echo still' EXIT; exit 3
//	bash 5.3, 3.2    bye 0                        in
//	ksh93u+          nothing                      in
//	zsh 5.9.2        nothing                      in
//	dash             nothing                      in
//	BusyBox ash      nothing                      in
//
// and every one of them killed by SEGV. ksh93 is the column that runs its
// EXIT trap on every other fatal signal, and that ends only the trap for one
// in it — `exit 3` with TERM there — so this is
// Semantics.UntrappedSegvEndsTheShellWhereItLands, read through each preset
// and the front end that owns the process. BusyBox ash is 1.36.1 in
// alpine:3.20, under `--init`.
func TestADialectsExitTrapAnswersAnUntrappedSegv(t *testing.T) {
	quiet := map[string]string{"ksh": "", "zsh": "", "dash": "", "ash": ""}
	for _, c := range []struct {
		name, script, bash string
		others             map[string]string
	}{
		{
			"at the top", `trap 'echo "bye $?"' EXIT; (exit 7); kill -SEGV $$; echo after`, "bye 0\n", quiet,
		},
		{
			"in the EXIT trap", `trap 'echo in; kill -SEGV $$; echo still' EXIT; exit 3`, "in\n",
			map[string]string{"ksh": "in\n", "zsh": "in\n", "dash": "in\n", "ash": "in\n"},
		},
	} {
		want := map[string]string{"bash": c.bash}
		for name, out := range c.others {
			want[name] = out
		}
		for name, out := range want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, got := endsOnItsOwn(t, name, c.script)
				if !ws.Signaled() || ws.Signal() != syscall.SIGSEGV {
					t.Errorf("ended %v, want killed by %v", ws, syscall.SIGSEGV)
				}
				if got != out {
					t.Errorf("wrote %q, want %q", got, out)
				}
			})
		}
	}
}

// What a trap a function set for itself does when the shell sends itself a
// death from inside the call (#7149).
//
// Measured 2026-11-01 under `-c`, each shell in a session of its own, with
// `f; echo after` after each definition:
//
//	trap 'echo top' EXIT; f() { trap 'echo local' EXIT; kill -TERM $$; }
//	        zsh 5.9.2, dash nothing; bash 5.3.20 local; killed by TERM in all
//	f() { trap 'echo local' EXIT; (kill -TERM $$); echo in; }
//	        zsh nothing, killed by TERM
//	f() { trap 'echo local' EXIT; kill -HUP $$; }
//	        zsh local, exit 1
//	function f { trap 'echo local' EXIT; kill -SEGV $$; }
//	        ksh93u+ nothing, killed by SEGV
//	function f { trap 'echo local' EXIT; kill -TERM $$; }
//	        ksh93u+ local, after, exit 0
//
// zsh runs no EXIT trap on a death, and that includes the call's own; HUP is
// an exit there. Here zsh wrote `local` for both TERM rows, and ksh for SEGV.
func TestAFunctionsOwnExitTrapAnswersADeathItSentTheShell(t *testing.T) {
	const (
		top  = `trap 'echo top' EXIT; `
		term = `f() { trap 'echo local' EXIT; kill -TERM $$; }; f; echo after`
	)
	for _, c := range []struct {
		dialect, script string
		sig             syscall.Signal
		status          int
		out             string
	}{
		{"zsh", top + term, syscall.SIGTERM, 0, ""},
		{"zsh", term, syscall.SIGTERM, 0, ""},
		{"zsh", `f() { trap 'echo local' EXIT; (kill -TERM $$); echo in; }; f; echo after`, syscall.SIGTERM, 0, ""},
		{"zsh", `f() { trap 'echo local' EXIT; kill -HUP $$; }; f; echo after`, 0, 1, "local\n"},
		{"bash", top + term, syscall.SIGTERM, 0, "local\n"},
		{"dash", top + term, syscall.SIGTERM, 0, ""},
		{"ksh", `function f { trap 'echo local' EXIT; kill -SEGV $$; }; f; echo after`, syscall.SIGSEGV, 0, ""},
		{"ksh", `function f { trap 'echo local' EXIT; kill -TERM $$; }; f; echo after`, 0, 0, "local\nafter\n"},
	} {
		t.Run(c.dialect+"/"+c.script, func(t *testing.T) {
			ws, out := endsOnItsOwn(t, c.dialect, c.script)
			switch {
			case c.sig != 0 && (!ws.Signaled() || ws.Signal() != c.sig):
				t.Errorf("ended %v, want killed by %v", ws, c.sig)
			case c.sig == 0 && (ws.Signaled() || ws.ExitStatus() != c.status):
				t.Errorf("ended %v, want an exit at %d", ws, c.status)
			}
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}

// endsOnItsOwn runs this test binary as the dialect's shell running script, in
// a process group of its own, and reports how it ended and what it wrote.
func endsOnItsOwn(t *testing.T, dialect, script string) (syscall.WaitStatus, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "out")
	cmd := exec.Command(os.Args[0], "-test.run=^TestADialectsExitTrapAnswersAFatalSignalFromOutside$")
	cmd.Env = testenv.WithoutRaceExitPause(
		append(os.Environ(), outsideDeathShell+"="+dialect, outsideDeathScript+"="+script),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
		t.Fatal("the shell was still running after 30s")
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status")
	}
	b, _ := os.ReadFile(logPath)
	return ws, string(b)
}

// What each dialect's EXIT trap reads after the shell sent itself a fatal
// signal with `kill` (#7062).
//
// Measured 2026-10-30, each shell in a process group of its own, `trap 'echo
// "bye $?"' EXIT; (exit 7)` and then the line:
//
//	                       kill     ! kill   f(){ kill; }   ( kill )
//	bash 5.3, 3.2, sh      bye 0    bye 1    bye 0          bye 7
//	ksh93u+                bye 7    bye 7    bye 271        bye 271
//	zsh 5.9.2, dash        killed by TERM, nothing written, in all four
//
// BusyBox ash runs no EXIT trap on a death either — its preset's measurement.
// Every one of them dies of TERM. Semantics.SelfAimedDeathIsTakenAtTheNextCommand
// and Semantics.DeathInAFunctionEndsTheCallAsKilled, read here through each
// preset and the front end that owns the process.
func TestADialectsExitTrapReadsWhatItsOwnKillLeft(t *testing.T) {
	const trap = `trap 'echo "bye $?"' EXIT; (exit 7); `
	quiet := map[string]string{"zsh": "", "dash": "", "ash": ""}
	for _, c := range []struct {
		name, script string
		bash, ksh    string
	}{
		{"kill", "kill -TERM $$; echo after", "bye 0\n", "bye 7\n"},
		{"a negated kill", "! kill -TERM $$; echo after", "bye 1\n", "bye 7\n"},
		{"a function", "f() { (exit 7); kill -TERM $$; echo inf; }; f; echo after", "bye 0\n", "bye 271\n"},
		{"a subshell", "(kill -TERM $$); echo after", "bye 7\n", "bye 271\n"},
	} {
		want := map[string]string{"bash": c.bash, "ksh": c.ksh}
		for name, out := range quiet {
			want[name] = out
		}
		for name, out := range want {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				ws, got := endsOnItsOwn(t, name, trap+c.script)
				if !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
					t.Errorf("ended %v, want killed by %v", ws, syscall.SIGTERM)
				}
				if got != out {
					t.Errorf("wrote %q, want %q", got, out)
				}
			})
		}
	}
}
