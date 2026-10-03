// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A bare `return` written in a trap action's own body —
// Semantics.TrapReturnStatus, and two of the four lines `suite: trap.tests`
// was still differing by (#4157).
//
// The probe puts every shell in front of the *same* number so that delivery
// timing cannot be what splits them: the action's first command prints `$?`
// and every column prints 4. What they then answer to `return` is the axis.
//
// Measured 2026-09-23, `env -i PATH=/usr/bin:/bin <shell> x.sh` over a script
// file, BusyBox ash in a busybox:latest container:
//
//	bash 5.3.20            entry 4    4      the status it was entered at
//	dash 0.5.12            entry 4    4      the same
//	BusyBox ash 1.37       entry 4    4      the same
//	zsh 5.9                entry 4    123    the action's own last command
//	ksh93u+ 2012-08-01     entry 4    0      zero
//
// The interrupted function never resumes in any of them — the `return` ends
// it — so what the caller reads is the number the axis decides.
func trapReturnPresets() []struct {
	dialecttest.Preset
	reading interp.TrapReturnReading
	want    string
} {
	return []struct {
		dialecttest.Preset
		reading interp.TrapReturnReading
		want    string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			// `entry 0` and not `entry 4`, which is **this shell's** zsh and
			// not zsh's: real zsh 5.9 prints `entry 4` here like every other
			// column. That half is Semantics.SignalHandlerSeesEarlierStatus
			// — what `$?` a handler is entered with — and it is measured to
			// be unchanged by this axis: the zsh binary built before and
			// after this change writes the same bytes for all three probes.
			// Written down rather than smoothed to `entry 4`, so the gap is
			// visible to whoever reaches that axis next.
			interp.TrapReturnTakesTheHandlersLastStatus, "entry 0 exit 123\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.TrapReturnIsZero, "entry 4 exit 0\n",
		},
		{
			dialecttest.Preset{
				Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
				Diagnostics: dash.Diagnostics, Apply: dash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
		{
			dialecttest.Preset{
				Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
				Diagnostics: ash.Diagnostics, Apply: ash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
	}
}

// The entry status is 4 for everybody by construction: the action interrupts a
// subshell that exits 4, and the signal is sent from a background job before
// that subshell can end. Without that the columns differ about *when* the
// action runs and the axis cannot be read at all — and without it bash and
// ksh93 would both answer 0 and the probe could not tell them apart, which is
// the whole reason this one waits rather than signaling itself.
//
// "Before it can end" is an ordering and not a pause, and it is kept in both
// directions with two FIFOs. The job waits on `ready` until the subshell has
// started, and only then sends the signal. The subshell waits on `go`, which
// the job writes only after its `kill` has returned, so the subshell cannot
// exit first. The first version used two sleeps, 0.1s and 0.5s. Under load
// the gap between them closed, and ash's column wrote `entry 0 exit 0`
// (#5570).
//
// **Each waiter holds its FIFO open read-write, and the other side writes a
// line**, rather than the waiter opening it for reading and the other side
// opening it for writing and closing it at once (`: >ready`). That second
// handshake hung the dash column on the macOS runner for the full ten minutes
// (#5697). The goroutine dump showed the subshell already past `: >ready`
// and waiting in `cat go`, with the job's `/bin/cat ready` still alive, so the
// job never reached its `kill` and nobody ever wrote `go`. That a reader can
// miss a writer that opened and closed while it was still waking up in open(2)
// is an inference from the dump, not reproduced locally (0 stuck in 100 tries
// on an idle machine). The new handshake has no such window: `exec 3<>` never
// waits on a FIFO and keeps a reader open for as long as the waiter lives, so
// the writer's open waits only until the waiter exists, and the line sits in
// the FIFO until it is read, whoever got there first.
//
// `exec 3<>` and `<&3`, and not `head <>fifo`: ksh93u+ hangs on the second
// spelling when the writer is a background job, and takes the first.
//
// The wait is in an external program and not a `read`, because ksh93 runs
// the action at once when the signal interrupts a `read` in its own process
// (`entry 0`). It waits for a child, as the others do. Measured 2026-10-03,
// five runs each with this probe under `env -i PATH=/usr/bin:/bin`: the table
// above, and BusyBox ash 1.37.0 in the pinned alpine digest five of five.
//
// `/usr/bin/head` and `/usr/bin/mkfifo` by path because the harness runs with
// no PATH of its own.
const trapReturnProbe = "/usr/bin/mkfifo go ready\n" +
	"f() { return $1; }\n" +
	trapReturnHandshake +
	"trap 'printf \"entry %s \" \"$?\"; f 123; return' USR1\n" +
	"h\n" +
	"printf 'exit %s\\n' \"$?\"\n" +
	"wait 2>/dev/null\n"

// trapReturnHandshake is `h`: a job that signals the shell while the
// foreground subshell is certainly running, and a subshell that cannot exit
// until the signal has been sent. Shared by both probes in this file.
const trapReturnHandshake = "h() { ( exec 3<>ready; /usr/bin/head -n 1 <&3 >/dev/null; kill -USR1 $$; echo x >go ) & " +
	"( echo x >ready; exec 3<>go; /usr/bin/head -n 1 <&3 >/dev/null; exit 4 ); return 7; }\n"

// trapReturnDeadline is how long the probe may take before the test gives up on
// it. A run that works takes milliseconds; a handshake that cannot complete is
// a test that fails here in seconds rather than one that holds the package
// until `go test`'s ten-minute timeout (#5697).
const trapReturnDeadline = 30 * time.Second

// runTrapReturnProbe runs the probe with a deadline. On expiry it writes a line
// into both FIFOs, which releases whichever side is still waiting, gives the
// run a few seconds to finish so nothing it started is left behind, and fails.
func runTrapReturnProbe(t *testing.T, p dialecttest.Preset, src string) string {
	t.Helper()
	dir := t.TempDir()
	var buf lockedOutput
	r := p.Runner(dialecttest.Base{Dir: dir, Stdout: &buf, Stderr: &buf})
	f := p.ParseThrough(t, r, src)
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), f)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err %v: %s", err, buf.String())
		}
		return buf.String()
	case <-time.After(trapReturnDeadline):
	}
	for _, name := range []string{"ready", "go"} {
		if w, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR, 0); err == nil {
			_, _ = w.WriteString("x\n")
			_ = w.Close()
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	t.Fatalf("the probe did not finish within %v; a FIFO handshake never completed. Output so far: %q",
		trapReturnDeadline, buf.String())
	return ""
}

// lockedOutput is the probe's stdout and stderr, written from the job's
// goroutine and the shell's at once.
type lockedOutput struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestEachDialectAnswersTheTrapReturnAxis(t *testing.T) {
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().TrapReturnStatus; got != p.reading {
				t.Errorf("TrapReturnStatus = %v, want %v", got, p.reading)
			}
		})
	}
}

// The bytes, which is what the field is for: a preset holding the right value
// and writing the wrong number would pass the test above.
func TestTheTrapReturnStatusInEveryDialect(t *testing.T) {
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if out := runTrapReturnProbe(t, p.Preset, trapReturnProbe); out != p.want {
				t.Errorf("wrote %q, want %q", out, p.want)
			}
		})
	}
}

// How far the reading reaches, which is measured and is not the same for all
// of them. `inner() { false; return; }` called *from* the action:
//
//	bash 5.3.20, dash 0.5.12, zsh 5.9   inner=1   the function's own last
//	ksh93u+ 2012-08-01                  inner=0   the same zero as the action
//
// So ksh93's answer covers the whole extent of the action and the other two
// readings stop at the action's own commands. Outside a trap entirely all four
// answer 1 — the second case here — which is what says this belongs to the
// trap and not to `return`.
func TestHowFarTheTrapReturnReadingReaches(t *testing.T) {
	// The same handshake as the probe above, where this used two sleeps and
	// was open to the reordering #5570 found there.
	const called = "/usr/bin/mkfifo go ready\n" +
		"inner() { /usr/bin/false; return; }\n" +
		trapReturnHandshake +
		"trap 'inner; printf \"inner=%s \" \"$?\"' USR1\n" +
		"h\n" +
		"printf 'exit %s' \"$?\"\n" +
		"wait 2>/dev/null\n"
	const noTrap = "inner() { /usr/bin/false; return; }\ninner\nprintf 'inner=%s' \"$?\"\n"
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			want := "inner=1 exit 7"
			if p.reading == interp.TrapReturnIsZero {
				want = "inner=0 exit 7"
			}
			if out := runTrapReturnProbe(t, p.Preset, called); out != want {
				t.Errorf("called from the action: wrote %q, want %q", out, want)
			}
			// And the same function with no trap anywhere is the ordinary
			// reading in every column, which is the control: without it a
			// shell that always answered the function's last command would
			// pass the row above for four of the five.
			out, _, err := p.Combined(t, dialecttest.Base{}, noTrap)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != "inner=1" {
				t.Errorf("with no trap: wrote %q, want %q", out, "inner=1")
			}
		})
	}
}
