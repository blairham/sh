// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
)

// startsWithItsDispositions is the front end's StartWithDispositions, written
// out again for the tests about a shell that owns its process, as
// ownsItsDispositions is for the other hook. The shipped one is graded by
// driver's own test against a real child.
//
// What it hands back after an ignore is the runtime's handling, by a
// subscription taken and dropped — except for HUP and INT, which the runtime
// leaves ignored when the last subscription goes, and which get a drain the
// way interp's own dispositionSink is one: the test binary has no watch to
// answer them.
func startsWithItsDispositions(ignore, reset []syscall.Signal, start func() error) error {
	var ignored, defaulted []os.Signal
	for _, sig := range ignore {
		if !signal.Ignored(sig) {
			ignored = append(ignored, sig)
		}
	}
	for _, sig := range reset {
		if signal.Ignored(sig) {
			defaulted = append(defaulted, sig)
		}
	}
	if len(ignored) > 0 {
		signal.Ignore(ignored...)
	}
	if len(defaulted) > 0 {
		signal.Notify(make(chan os.Signal, 1), defaulted...)
	}
	err := start()
	if len(defaulted) > 0 {
		signal.Ignore(defaulted...)
	}
	for _, sig := range ignored {
		if sig == syscall.SIGHUP || sig == syscall.SIGINT {
			signal.Notify(dispositionDrain, sig)
			continue
		}
		c := make(chan os.Signal, 1)
		signal.Notify(c, sig)
		signal.Stop(c)
	}
	return err
}

// What a command a subshell starts inherits is what the subshell arranged,
// not what the process happens to hold — #7049, and its mirror.
//
// Measured 2026-10-30 with a child that raises USR1 at itself, inside the
// parentheses: bash 5.3.20, bash 3.2.57, ksh93u+, zsh 5.9.2 and dash all have
// the child survive a subshell's own `trap ""`, and all have it killed when
// the shell ignores USR1 and the subshell resets or handles it. Here the
// first was killed and the other two survived, because a subshell is a clone
// and its table was nowhere a child could see. The last row is the window
// closing: the subshell's ignore is gone once the subshell is.
func TestASubshellsDispositionsReachTheCommandsItStarts(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	restoreDispositionsAfter(t, []string{"USR1"})
	// Computed, so that a notice quoting the child's command line cannot be
	// mistaken for the child having survived.
	const probe = "/bin/sh -c 'kill -USR1 $$; echo alive-$((6*7))'"
	for _, tc := range []struct {
		name, src string
		survives  bool
	}{
		// The control, and the reason the other rows mean anything.
		{"nothing arranged", "(" + probe + ")", false},
		{"ignored in the subshell", "(trap '' USR1; " + probe + ")", true},
		{"ignored by the shell, reset in the subshell", "trap '' USR1; (trap - USR1; " + probe + ")", false},
		{"ignored by the shell, handled in the subshell", "trap '' USR1; (trap : USR1; " + probe + ")", false},
		{"ignored by the shell, nothing in the subshell", "trap '' USR1; (" + probe + ")", true},
		{"ignored in an earlier subshell", "(trap '' USR1); " + probe, false},
		{"ignored in a background job", "(trap '' USR1; " + probe + ") & wait", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runOwningTheProcess(t, tc.src+"\necho ran\n")
			if !strings.Contains(out, "ran") {
				t.Fatalf("%s: never ran to its end: %q", tc.src, out)
			}
			if got := strings.Contains(out, "alive-42"); got != tc.survives {
				t.Errorf("%s: the child survived = %v, want %v (wrote %q)", tc.src, got, tc.survives, out)
			}
			if signal.Ignored(syscall.SIGUSR1) {
				t.Errorf("%s: left the process ignoring SIGUSR1", tc.src)
			}
		})
	}
}

// A job forked before the shell ignored a signal keeps the default it was
// forked with. Measured 2026-10-30, `(sleep 0.3; <child raising USR1>) & trap
// "" USR1; wait`: the child is killed in bash 5.3, bash 3.2, ksh93, zsh and
// dash. Here the job is a goroutine and the ignore the shell set later was the
// process's by the time the job started its child.
func TestAJobStartedBeforeAnIgnoreKeepsTheDefault(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	restoreDispositionsAfter(t, []string{"USR1"})
	goAhead := t.TempDir() + "/go"
	src := "(while [ ! -e " + goAhead + " ]; do /bin/sleep 0.02; done\n" +
		"/bin/sh -c 'kill -USR1 $$; echo alive-$((6*7))') &\n" +
		"trap '' USR1\n" +
		"step ignoring\n" +
		"wait\n" +
		"echo ran\n"
	s := startSteppedWith(t, src, ownsTheProcess)
	s.reach(t, "ignoring")
	// The job starts its child only now, once the shell has ignored USR1.
	if err := os.WriteFile(goAhead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.release()
	s.wait(t)
	out := s.out.String()
	if !strings.Contains(out, "ran") {
		t.Fatalf("never ran to its end: %q", out)
	}
	if strings.Contains(out, "alive-42") {
		t.Errorf("a job forked before `trap '' USR1` started a child that inherited it: %q", out)
	}
}

// A signal that arrives while a shebang-less script that ignores it is
// running reaches the caller's trap — #7051.
//
// Measured 2026-10-30, a script holding `trap "" USR1; /bin/sleep 1; echo
// inner`, run under `trap 'echo caught' USR1` with a USR1 from another process
// 0.3s in: `inner`, `caught`, `after` in bash 5.3, bash 3.2, ksh93, zsh and
// dash. Here the script's ignore was the process's for as long as it ran, and
// the kernel dropped the arrival before any trap could hear it.
//
// The script starts the sender itself, so that the signal lands inside the
// script without a clock outside it deciding that, and the sender waits a
// moment before sending so that the start it was made by is long over. A
// caller that heard it ends its wait at once by killing the job; one that did
// not waits the job out and writes no `caught`.
func TestASignalDuringAShebangLessScriptThatIgnoresItReachesTheCaller(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to send the signal from: %v", err)
	}
	restoreDispositionsAfter(t, []string{"USR1"})
	script := t.TempDir() + "/ignores"
	body := "trap '' USR1\n" +
		"/bin/sh -c '/bin/sleep 0.1; kill -USR1 " + strconv.Itoa(os.Getpid()) + "'\n" +
		"echo inner\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	src := "trap 'echo caught; kill $! 2>/dev/null' USR1\n" +
		"/bin/sleep 5 &\n" +
		script + "\n" +
		"wait $!\n" +
		"echo done\n"
	s := startSteppedWith(t, src, func(r *interp.Runner) {
		reportsAKilledJob(r)
		ownsTheProcess(r)
	})
	s.wait(t)
	if got, want := s.out.String(), "inner\ncaught\ndone\n"; got != want {
		t.Errorf("wrote %q, want %q — the script's ignore kept the signal from the caller", got, want)
	}
}

// The window an ignore is put on the process for hands back every
// subscription it took: here, another Runner's handler for the same signal,
// which nothing in the Runner that started the command knows about.
func TestAWindowGivesBackAnotherRunnersHandler(t *testing.T) {
	kill := externalKill(t)
	restoreDispositionsAfter(t, []string{"USR1"})
	listener := startSteppedWith(t, "trap 'echo heard; kill $! 2>/dev/null' USR1\n"+
		"/bin/sleep 5 &\n"+
		"step ready\n"+
		"wait $!\n"+
		"echo done\n", reportsAKilledJob)
	listener.reach(t, "ready")
	// A subshell's ignore, handed to the command it starts through a window
	// on the process, which took the listener's subscription for its length.
	out, _ := runOwningTheProcess(t, "(trap '' USR1; /bin/sh -c ':')\necho ran\n")
	if !strings.Contains(out, "ran") {
		t.Fatalf("the starting shell never ran to its end: %q", out)
	}
	if outK, err := exec.Command(kill, "-USR1", strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
		t.Fatalf("%s -USR1: %v: %s", kill, err, outK)
	}
	listener.release()
	listener.wait(t)
	if got, want := listener.out.String(), "heard\ndone\n"; got != want {
		t.Errorf("the listener wrote %q, want %q: the window did not give its subscription back", got, want)
	}
}
