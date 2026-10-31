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

// One Runner's ignore or reset leaves another Runner's handler alone, whichever
// of the two ran first — #7044, in the shape it was found in.
//
// Two Runners share this test binary, neither handed the process's
// dispositions. The *listener* traps SIGUSR1 and waits on a job; the *other*
// does one thing to SIGUSR1 and parks. Then SIGUSR1 arrives from another
// process, which is the arrival os/signal fans out to every subscriber: when
// `trap ""` was signal.Ignore and `trap -` was signal.Reset, the other's line
// took the listener's subscription with it, and a listener that had trapped
// first heard nothing while one that trapped second heard it.
//
// The listener's wait is what makes the rows cheap both ways. A handler that
// runs ends it at once, by killing the job, wherever the arrival lands — before
// the wait starts or during it — so a passing row takes no time; a listener
// that heard nothing waits the job out and writes `done` without `heard`, so a
// failing one is a few seconds and a plain message rather than a deadline.
//
// The other Runner's rows are every line that reached the process in the old
// arrangement: an ignore, a reset of nothing, and a reset of each of the two
// things a reset can take back.
func TestOneRunnersIgnoreLeavesAnotherRunnersHandlerAlone(t *testing.T) {
	kill := externalKill(t)
	const listen = "trap 'echo heard; kill $! 2>/dev/null' USR1\n" +
		"/bin/sleep 5 &\n" +
		"step ready\n" +
		"wait $!\n" +
		"echo done\n"
	for _, other := range []struct{ name, src string }{
		{"an ignore", "trap '' USR1"},
		{"a reset of nothing", "trap - USR1"},
		{"a handler reset", "trap 'echo other' USR1\ntrap - USR1"},
		{"an ignore reset", "trap '' USR1\ntrap - USR1"},
	} {
		for _, listenerFirst := range []bool{true, false} {
			order := "the listener first"
			if !listenerFirst {
				order = "the other first"
			}
			t.Run(other.name+", "+order, func(t *testing.T) {
				restoreDispositionsAfter(t, []string{"USR1"})
				start := func(src string) *stepped {
					s := startSteppedWith(t, src, reportsAKilledJob)
					s.reach(t, "ready")
					return s
				}
				var listener, ignorer *stepped
				if listenerFirst {
					listener = start(listen)
					ignorer = start(other.src + "\nstep ready\necho finished\n")
				} else {
					ignorer = start(other.src + "\nstep ready\necho finished\n")
					listener = start(listen)
				}

				// From another process, so that it is the kernel's arrival and
				// not a shell's own send — which never leaves the shell that
				// made it (#7040), and so could not show this.
				if out, err := exec.Command(kill, "-USR1", strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
					t.Fatalf("%s -USR1: %v: %s", kill, err, out)
				}

				listener.release()
				listener.wait(t)
				ignorer.release()
				ignorer.wait(t)

				if got, want := listener.out.String(), "heard\ndone\n"; got != want {
					t.Errorf("the listener wrote %q, want %q: the other Runner's line took its subscription away",
						got, want)
				}
				if got, want := ignorer.out.String(), "finished\n"; got != want {
					t.Errorf("the other Runner wrote %q, want %q", got, want)
				}
			})
		}
	}
}

// The shell an embedder builds keeps its ignore to itself, and the process
// it is running in is left exactly as it was found.
//
// That is the other half of the fix and the half a library can see: an
// ignore in the old arrangement was the process's, so it outlived the Runner
// that set it until restoreDispositions put it back, and in between every
// program the *embedder* started inherited it. Asked of the process directly
// rather than of a child, because a Runner with no SetSignalDisposition never
// sets SIG_IGN at all — and asked while the Runner is still running, which is
// when the old ignore was in force.
func TestAnIgnoreInARunnerThatMayNotTouchTheProcessStaysInTheRunner(t *testing.T) {
	restoreDispositionsAfter(t, []string{"USR2"})
	if signal.Ignored(syscall.SIGUSR2) {
		t.Fatal("this test needs SIGUSR2 not ignored to start with")
	}
	s := startStepped(t, "trap '' USR2\nstep ignoring\necho finished\n")
	s.reach(t, "ignoring")
	ignoredMeanwhile := signal.Ignored(syscall.SIGUSR2)
	s.release()
	s.wait(t)
	if ignoredMeanwhile {
		t.Error("`trap '' USR2` in a Runner that may not touch the process made the process ignore SIGUSR2")
	}
	if got, want := s.out.String(), "finished\n"; got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

// reportsAKilledJob answers the one axis a listener's wait reaches, for a job
// its own handler killed. Which way does not matter here.
func reportsAKilledJob(r *interp.Runner) {
	r.Semantics.WaitReportsTheSignalThatEndedTheJob = interp.No
}

// ownsItsDispositions is the front end's SetSignalDisposition, written out
// again for the tests that are about a shell which owns its process: driver
// imports interp, and the shipped hook is graded by driver's own test against
// a real child. Both halves are os/signal's, for the reason the hook gives.
func ownsItsDispositions(sig syscall.Signal, ignore bool) {
	if ignore {
		signal.Ignore(sig)
		return
	}
	signal.Reset(sig)
}

// A shebang-less script is a second Runner in the shell's own process, and it
// used to be handed SetSignalDisposition so that its commands would inherit
// what it ignored. The cost was the caller's subscriptions, which the
// script's ignore took away and its end did not give back: measured
// 2026-10-29, `trap 'echo caught' USR1`, a shebang-less script ignoring USR1,
// then a USR1 from another process — `caught` in bash 5.3, bash 3.2, ksh93,
// zsh and dash, and nothing at all from this shell's five binaries (#7044).
// The script keeps its ignore to itself now (#7051); this is the row that
// says the signal after it still reaches the caller.
func TestAShebangLessScriptsIgnoreLeavesTheCallersHandler(t *testing.T) {
	kill := externalKill(t)
	restoreDispositionsAfter(t, []string{"USR1"})
	dir := t.TempDir()
	script := dir + "/ignores"
	if err := os.WriteFile(script, []byte("trap '' USR1\necho inner\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	src := "trap 'echo caught; kill $! 2>/dev/null' USR1\n" +
		script + "\n" +
		"/bin/sleep 5 &\n" +
		"step ready\n" +
		"wait $!\n" +
		"echo done\n"
	s := startSteppedWith(t, src, func(r *interp.Runner) {
		reportsAKilledJob(r)
		ownsTheProcess(r)
	})
	s.reach(t, "ready")
	if out, err := exec.Command(kill, "-USR1", strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
		t.Fatalf("%s -USR1: %v: %s", kill, err, out)
	}
	s.release()
	s.wait(t)
	if got, want := s.out.String(), "inner\ncaught\ndone\n"; got != want {
		t.Errorf("wrote %q, want %q — the script's ignore took the caller's handler with it", got, want)
	}
}

// And the other half: the commands the script starts inherit what it
// ignores, which a script that keeps its ignore to itself gives them through
// StartWithDispositions, a start at a time. Measured 2026-10-29, a shebang-less script ignoring INT and
// then starting a child that raises it at itself: the child survives in bash
// 5.3, bash 3.2, ksh93, zsh and dash. The control is the same script with no
// ignore, which is what says the probe can report a death.
func TestAShebangLessScriptsIgnoreReachesItsCommands(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	restoreDispositionsAfter(t, []string{"USR1"})
	// Computed, so that a notice quoting the child's command line cannot be
	// mistaken for the child having survived.
	const probe = "/bin/sh -c 'kill -USR1 $$; echo alive-$((6*7))'\n"
	for _, tc := range []struct {
		name, prep string
		survives   bool
	}{
		{"nothing arranged", "", false},
		{"ignored", "trap '' USR1\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := t.TempDir() + "/script"
			if err := os.WriteFile(script, []byte(tc.prep+probe+"echo ran\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			out, _ := runOwningTheProcess(t, script+"\n")
			if !strings.Contains(out, "ran") {
				t.Fatalf("the script never ran to its end: %q", out)
			}
			if got := strings.Contains(out, "alive-42"); got != tc.survives {
				t.Errorf("the script's child survived = %v, want %v (wrote %q)", got, tc.survives, out)
			}
		})
	}
}

// A reset gives up the one subscription it names, and the Runner goes on
// hearing everything else it trapped.
//
// os/signal cannot take one signal off one channel, so a reset in a Runner
// that keeps to itself stops its channel and subscribes it again to what the
// table still holds. This is the row that says the second half happens.
func TestAResetKeepsTheRunnersOtherHandlers(t *testing.T) {
	kill := externalKill(t)
	restoreDispositionsAfter(t, []string{"USR1", "USR2"})
	s := startSteppedWith(t, "trap 'echo heard; kill $! 2>/dev/null' USR1\n"+
		"trap 'echo two' USR2\n"+
		"trap - USR2\n"+
		"/bin/sleep 5 &\n"+
		"step ready\n"+
		"wait $!\n"+
		"echo done\n", reportsAKilledJob)
	s.reach(t, "ready")
	if out, err := exec.Command(kill, "-USR1", strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
		t.Fatalf("%s -USR1: %v: %s", kill, err, out)
	}
	s.release()
	s.wait(t)
	if got, want := s.out.String(), "heard\ndone\n"; got != want {
		t.Errorf("wrote %q, want %q — resetting USR2 took the USR1 handler with it", got, want)
	}
}

// A Runner that may not touch the process does not take off an ignore it
// finds there either.
//
// Its end puts back what its own subscriptions took over, and an ignore that
// appeared while it ran is not one of those: something else in the program
// set it, and taking it off would be the Runner reaching into the process
// after all — the embedder's `nohup` undone by a script it happened to run.
func TestARunnerLeavesAloneAnIgnoreItDidNotSet(t *testing.T) {
	restoreDispositionsAfter(t, []string{"USR2"})
	if signal.Ignored(syscall.SIGUSR2) {
		t.Fatal("this test needs SIGUSR2 not ignored to start with")
	}
	s := startStepped(t, "trap 'echo two' USR2\nstep ready\necho finished\n")
	s.reach(t, "ready")
	// The program the Runner is embedded in, deciding something of its own.
	signal.Ignore(syscall.SIGUSR2)
	s.release()
	s.wait(t)
	if !signal.Ignored(syscall.SIGUSR2) {
		t.Error("a Runner that never ignored SIGUSR2 took away an ignore the program set while it ran")
	}
}

// An ignore the process already had goes back when the last Runner holding the
// signal finishes, and not before — #7052, in the shape it was found in.
//
// The process starts this row ignoring SIGUSR2, as one started under `nohup`
// or with an ignore its parent set would. The *first* Runner does one thing to
// USR2 and parks; the *listener* traps it and waits on a job. Each Runner used
// to keep its own note of what the process was doing before its first `trap`,
// and the two notes disagreed: the first found the ignore, and the listener
// found the first's subscription. So the first to finish put the ignore back
// with signal.Ignore while the listener was still waiting, which took the
// listener's subscription with it, and a USR2 from another process was then
// dropped by the kernel — `done` without `heard`. Finishing in the other order
// heard it, so the outcome hung on an order nothing in either script can see.
//
// The second half is the other side of the same note: whichever Runner is the
// last to let go, the process is ignoring USR2 again afterwards, because the
// ignore was the process's and not either Runner's.
func TestAnInheritedIgnoreGoesBackOnlyWhenTheLastRunnerHoldingTheSignalFinishes(t *testing.T) {
	kill := externalKill(t)
	const listen = "trap 'echo heard; kill $! 2>/dev/null' USR2\n" +
		"/bin/sleep 5 &\n" +
		"step ready\n" +
		"wait $!\n" +
		"echo done\n"
	for _, first := range []struct{ name, src string }{
		{"a handler", "trap : USR2"},
		{"an ignore", "trap '' USR2"},
		{"a reset of nothing", "trap - USR2"},
	} {
		for _, firstFinishesFirst := range []bool{true, false} {
			order := "the first finishing first"
			if !firstFinishesFirst {
				order = "the first finishing last"
			}
			t.Run(first.name+", "+order, func(t *testing.T) {
				restoreDispositionsAfter(t, []string{"USR2"})
				signal.Ignore(syscall.SIGUSR2)

				other := startSteppedWith(t, first.src+"\nstep ready\necho finished\n", reportsAKilledJob)
				other.reach(t, "ready")
				listener := startSteppedWith(t, listen, reportsAKilledJob)
				listener.reach(t, "ready")

				if firstFinishesFirst {
					other.release()
					other.wait(t)
				}
				if out, err := exec.Command(kill, "-USR2", strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
					t.Fatalf("%s -USR2: %v: %s", kill, err, out)
				}
				listener.release()
				listener.wait(t)
				if !firstFinishesFirst {
					other.release()
					other.wait(t)
				}

				if got, want := listener.out.String(), "heard\ndone\n"; got != want {
					t.Errorf("the listener wrote %q, want %q: the Runner that finished first put the "+
						"process's ignore back under it", got, want)
				}
				if got, want := other.out.String(), "finished\n"; got != want {
					t.Errorf("the first Runner wrote %q, want %q", got, want)
				}
				if !signal.Ignored(syscall.SIGUSR2) {
					t.Error("both Runners finished and the process no longer ignores SIGUSR2, which it did before either touched it")
				}
			})
		}
	}
}
