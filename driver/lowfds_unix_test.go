// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// lowFdScript names the script for the half of a test that has to *be* the
// shell, and lowFdStall is how long that shell holds the window open.
const (
	lowFdScript = "SH_TEST_LOW_FD_SCRIPT"
	lowFdStall  = "SH_TEST_LOW_FD_STALL"
)

// windowStall is long enough for `sysmon` to poll inside the window.
//
// The Linux instance of this fault is a race and the race is not one a test
// can wait for: what notices the overwritten descriptor is another thread on a
// timer of its own, so the failure needs the window to be *long* rather than
// to be reached in some particular order. sysmon polls the netpoller at most
// every ten milliseconds, so twenty is two of its chances.
//
// Measured on Linux, before this file's fix: with the stall, a script parking
// on the poller's number lost the replacement 25 times out of 25. Without it,
// 300 runs produced none — which is why the bare rate could not be used, and
// why the stall is here rather than a repetition count.
const windowStall = 20 * time.Millisecond

// beTheLowFdShell turns this process into the shell when it was re-executed as
// one, with two things a plain shell does not have.
//
// **bash's descriptor numbering**, because bash is the only shell in the panel
// that can *name* a descriptor above 9: `exec 20>f` is a redirection there and
// a command word in dash, ksh93 and zsh, which answer "20: not found". The
// numbers this is about are the ones above the ceiling, so the dialect that
// can reach them is the one to ask with.
//
// **A stall in the window**, because the Linux half of this is a race against
// another thread. See windowStall.
func beTheLowFdShell() {
	path := os.Getenv(lowFdScript)
	if path == "" {
		return
	}
	if d, err := time.ParseDuration(os.Getenv(lowFdStall)); err == nil && d > 0 {
		driver.AtPlacementForTest(func(where string) {
			if where == "after" {
				// After the table is placed and before the execve: the
				// interval a real shell crosses in microseconds, held open
				// so that what would land in it lands in it every time.
				time.Sleep(d)
			}
		})
	}
	sh := shell()
	sh.Dialect.MultiDigitFdNumber = true
	os.Exit(driver.MainArgs(sh, []string{"testsh", path}))
}

// runAsShellParkingOn re-executes this test binary as a shell that parks a
// file on one descriptor and then replaces itself, and reports what the
// replacement wrote and whether the file was written.
//
// A second process is unavoidable: `exec cmd` is a real execve and the test
// binary would stop being a test.
//
// The script traps a signal first, and that line is the darwin half of this
// bug rather than decoration. Asking os/signal for anything makes the runtime
// open a *pipe* to carry the arrival on that platform, on two low descriptors,
// and the goroutine reading it is already blocked there when a placement
// overwrites it — so it throws immediately rather than racing. One `trap` line
// was the whole difference between a shell that execs and one that dies.
func runAsShellParkingOn(t *testing.T, name string, fd int) (out string, placed bool) {
	t.Helper()
	dir := t.TempDir()
	parked := filepath.Join(dir, "parked")
	// The replacement's error stream is kept rather than discarded, and that
	// is not tidiness. This test sends it to a file because the placement
	// puts descriptor 2 wherever the script said, so a `2>/dev/null` would
	// send the *shell's* own fatal error there too — which is exactly how the
	// sentence naming this bug was thrown away on all three of its earlier
	// sightings. See #695.
	complaint := filepath.Join(dir, "complaint")
	script := filepath.Join(dir, "case.sh")
	src := "trap 'echo trapped' USR1\n" +
		"exec " + strconv.Itoa(fd) + ">" + parked + "\n" +
		"exec /bin/sh -c 'echo ran; echo parked >&" + strconv.Itoa(fd) + "' 2>" + complaint + "\n"
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), lowFdScript+"="+script, lowFdStall+"="+windowStall.String())
	// Deliberately no ExtraFiles: every number this asks about has to be
	// unopened at the start, so what ends up on it is the script's doing.
	b, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("the shell half could not be started: %v\n%s", err, b)
	}
	said, _ := os.ReadFile(complaint)
	got, _ := os.ReadFile(parked)
	out = string(b)
	if len(said) > 0 {
		out += "\nthe replacement said: " + string(said)
	}
	return out, strings.TrimSpace(string(got)) == "parked"
}

// Every descriptor a script can name carries its file across a replacement.
//
// This is #695, #731 and #799, which are three reports of one fault: the Go
// runtime holds descriptors of its own, a script parks over the numbers they
// landed on, and the runtime finding out is a fatal error rather than a
// reportable one. Which structure gets overwritten is the platform's — a
// poller on Linux, a signal pipe on darwin — and the shell dies either way,
// where it was about to become the command.
//
// The numbers are the ones a script can reach. 3 through 9 is what the whole
// panel can name; 99 is the ceiling, and bash can name every number up to it.
// Before lowfds_unix.go the runtime sat at 3 and 4 on darwin and at 5 and 6 on
// Linux — inside this list, by construction rather than by luck, because
// opening the script is what leaves those numbers free for it.
func TestAReplacementFindsItsFileOnEveryDescriptorAScriptCanName(t *testing.T) {
	beTheLowFdShell()
	// Single digits carry both halves of the claim; the rest carry one.
	// What the replacement is asked to do with the descriptor it was given is
	// bounded by whatever /bin/sh is on the machine, and that shell is dash on
	// Debian, which answers `>&99` with "Bad fd number" — the same
	// single-digit rule the panel measurement found from the other side. So
	// above 9 the assertion is that the shell reached the exec at all, which
	// is the half this is about.
	for _, fd := range []int{3, 4, 5, 6, 7, 8, 9, 10, 63, 99} {
		t.Run(strconv.Itoa(fd), func(t *testing.T) {
			out, placed := runAsShellParkingOn(t,
				"TestAReplacementFindsItsFileOnEveryDescriptorAScriptCanName", fd)
			if !strings.Contains(out, "ran") {
				t.Fatalf("the replacement never ran with a file parked on %d — the shell died"+
					" where it was about to become the command:\n%s", fd, out)
			}
			if fd <= 9 && !placed {
				t.Errorf("the replacement did not find its file on descriptor %d:\n%s", fd, out)
			}
		})
	}
}

// And a number the runtime does hold costs the command that number rather than
// costing it the shell.
//
// The ceiling moves the runtime out of the range a script names; it cannot
// move it out of the range bash can *write*, because bash can write any
// number. So the descriptors immediately above the ceiling are the runtime's,
// they are still nameable by hand, and this is what happens when one is named:
// placeFiles leaves the number alone, the exec goes ahead, and the command
// finds that one descriptor closed. Measured before the backstop: on darwin
// `exec 100>f` died exactly the way `exec 3>f` did.
//
// The whole band is walked rather than the numbers the runtime took, and that
// is deliberate — which of them it took differs by platform and by Go version,
// and the claim is about the shell surviving rather than about a number.
func TestADescriptorTheRuntimeHoldsCostsTheCommandTheNumberAndNotTheShell(t *testing.T) {
	beTheLowFdShell()
	ceiling := driver.LowDescriptorCeilingForTest()
	for fd := ceiling + 1; fd <= ceiling+4; fd++ {
		t.Run(strconv.Itoa(fd), func(t *testing.T) {
			out, _ := runAsShellParkingOn(t,
				"TestADescriptorTheRuntimeHoldsCostsTheCommandTheNumberAndNotTheShell", fd)
			if !strings.Contains(out, "ran") {
				t.Fatalf("naming descriptor %d killed the shell instead of costing the"+
					" command a number:\n%s", fd, out)
			}
		})
	}
}

// The runtime's own descriptors are above every number a script can name.
//
// The invariant the two tests above are the behavior of, asserted where it is
// cheap: the numbers are recorded as the runtime takes them, so this costs
// nothing and runs on every platform this file builds on.
//
// It is not vacuous, and the emptiness check is what keeps it that way: an
// empty census means the hold was never completed and the runtime was never
// made to open anything, which is the state this whole file exists to avoid.
// A test binary has descriptors to spare, so there it is a failure.
func TestTheRuntimeHoldsNoDescriptorAScriptCanName(t *testing.T) {
	held := driver.RuntimeDescriptorsForTest()
	if len(held) == 0 {
		t.Fatal("the runtime opened nothing while the low descriptors were held," +
			" so nothing was moved out of a script's reach")
	}
	for _, fd := range held {
		if fd <= nameableCeiling {
			t.Errorf("the runtime holds descriptor %d, which is one a script can name"+
				" — parking there kills the shell", fd)
		}
	}
	if driver.LowDescriptorCeilingForTest() < nameableCeiling {
		t.Errorf("the shell keeps the runtime off descriptors up to %d, want at least %d",
			driver.LowDescriptorCeilingForTest(), nameableCeiling)
	}
}

// nameableCeiling is the highest descriptor this test insists the runtime
// stays above.
//
// Written out rather than read from the package, and that is the whole point
// of it being here: a test that took the ceiling from the code it is checking
// would follow the ceiling down. Lowering it to 9 — which is all the panel can
// name, and a defensible-looking number — would leave the runtime on 10, 11
// and 12, which is where bash's own `exec {v}>f` allocates from, and every
// assertion in this file would still pass. This one would not.
const nameableCeiling = 99
