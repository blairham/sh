// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// trappedAfterARun is what a library Runner runs in a process a front-end run
// has just returned from: it traps USR1, has another process send this one a
// real USR1, and waits for its handler. Bounded, so a handler that never runs
// ends the script rather than the test.
const trappedAfterARun = `trap 'echo U; got=1' USR1
/bin/sh -c 'kill -USR1 "$1"' sh $$
n=0
while [ -z "$got" ] && [ "$n" -lt 100 ]; do /bin/sleep 0.05; n=$((n+1)); done
echo after
`

// A run of MainArgs in a test binary's own process leaves nothing behind that
// answers the binary's signals (#7370). The watch for fatal signals it set up
// asked the shell it built, and once that shell's run was over it went on
// asking it: a later test that trapped USR1 in a library Runner and was sent a
// real one from another process had the dead shell answer "not trapped", and
// the watch killed the whole test binary with no test named.
//
// Each case is that order exactly, in a re-executed copy of this binary,
// because the failure this is about is a test binary being killed and this one
// must outlive it to say so. The two differ only in whether the copy keeps the
// watch as a binary's main does, and that is the control: a kept watch is
// still up after the run and dies of the USR1, which is what shows the case
// reaches the watch at all — so the one where it is not kept surviving is the
// watch having gone, and not a signal that never arrived.
func TestARunThatReturnedLeavesNoWatchBehind(t *testing.T) {
	for _, c := range []struct {
		name string
		keep bool
	}{
		{name: "released", keep: false},
		{name: "kept", keep: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if src := os.Getenv(dyingScript); src != "" {
				trapAfterARun(t, c.keep, src)
				return
			}
			ws, out := waitStatusOfAShell(t, "^TestARunThatReturnedLeavesNoWatchBehind$/^"+c.name+"$", trappedAfterARun)
			if c.keep {
				if !ws.Signaled() || ws.Signal() != syscall.SIGUSR1 {
					t.Fatalf("with the watch kept past the run, the process ended %v, want killed by USR1 — "+
						"the case never reached the watch, so the other one proves nothing\n%s", ws, out)
				}
				return
			}
			if ws.Signaled() {
				t.Fatalf("the test binary was killed by %v after a front-end run had returned: "+
					"the run's watch for fatal signals outlived it\n%s", ws.Signal(), out)
			}
			if ws.ExitStatus() != 0 || !strings.Contains(out, "U\nafter\n") {
				t.Fatalf("exited %d writing\n%s\nwant 0, and the library Runner's handler run before `after`",
					ws.ExitStatus(), out)
			}
		})
	}
}

// trapAfterARun is the re-executed half: a front-end run in this process, and
// then a library Runner running src, with what it writes on standard output.
func trapAfterARun(t *testing.T, keep bool, src string) {
	if keep {
		driver.KeepFatalSignalWatch()
	}
	sh := shell()
	sh.Stdout, sh.Stderr = io.Discard, io.Discard
	if code := driver.MainArgs(sh, []string{"testsh", "-c", ":"}); code != 0 {
		t.Fatalf("the front-end run exited %d", code)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	// The two axes a trap asks, answered as the shell half in die_test.go
	// answers them; which way does not matter here.
	sem := interp.PosixSemantics()
	sem.TrapBodyRunsWhatParsed = interp.Yes
	sem.SignalHandlerSeesEarlierStatus = interp.Yes
	d := syntax.Core()
	r := &interp.Runner{Stdout: os.Stdout, Stderr: os.Stderr, Semantics: &sem, Dialect: &d}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// interruptProbe is a child that sends itself an interrupt: it says `survived`
// only where it was born ignoring one.
const interruptProbe = `kill -INT $$; echo survived`

// And taking the watch down hands an interrupt back as it found it.
//
// The runtime lets an inherited ignore of HUP or INT stand, and tells one from
// its own handler by what the disposition was when a subscription last put
// that handler on. A shell that ignores INT and then hands it back to the
// watch — `trap ” INT; trap - INT`, or the window a background job is started
// in — leaves the ignore recorded there, so stopping the watch put SIG_IGN back
// on the process. Every command it started afterwards was born ignoring INT,
// and in this package that was every shell half a later test sent one to:
// three tests' worth of shells lived through the interrupt they were to die of.
//
// The control is the test ignoring INT itself, which is what says the probe
// can see an ignore at all.
func TestARunThatReturnedLeavesNoInterruptIgnored(t *testing.T) {
	for _, c := range []struct {
		name     string
		survives bool
	}{
		{name: "after a run that ignored and reset it", survives: false},
		{name: "ignored by the test itself", survives: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if os.Getenv(dyingScript) != "" {
				probeAfterARun(t, c.survives)
				return
			}
			name := "^TestARunThatReturnedLeavesNoInterruptIgnored$/^" + strings.ReplaceAll(c.name, " ", "_") + "$"
			ws, out := waitStatusOfAShell(t, name, "trap '' INT; trap - INT")
			if ws.Signaled() || ws.ExitStatus() != 0 {
				t.Fatalf("the re-executed test ended %v\n%s", ws, out)
			}
			if got := strings.Contains(out, "survived"); got != c.survives {
				t.Errorf("a child sent itself an interrupt and survived = %v, want %v\n%s", got, c.survives, out)
			}
		})
	}
}

// probeAfterARun is the re-executed half: a front-end run of src in this
// process, and then a child that sends itself an interrupt.
func probeAfterARun(t *testing.T, ignore bool) {
	sh := shell()
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes
	sh.Semantics.SignalHandlerSeesEarlierStatus = interp.Yes
	sh.Stdout, sh.Stderr = io.Discard, io.Discard
	if code := driver.MainArgs(sh, []string{"testsh", "-c", os.Getenv(dyingScript)}); code != 0 {
		t.Fatalf("the front-end run exited %d", code)
	}
	if ignore {
		signal.Ignore(syscall.SIGINT)
	}
	out, _ := exec.Command("/bin/sh", "-c", interruptProbe).CombinedOutput()
	_, _ = os.Stdout.Write(out)
}
