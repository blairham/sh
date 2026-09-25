// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// highestInheritedFd is how far up the table the sweep below looks.
//
// A number the host handed us above this one is above everything any rule
// here allocates from, so leaving it open changes no measurement in this
// package. A bound rather than the open-file limit because that limit is
// 1048576 on a stock Linux and the sweep is one fcntl per number.
const highestInheritedFd = 1024

// quietFdTableEnv marks the child run below, so that a case it is measuring
// measures rather than starting a child of its own.
const quietFdTableEnv = "SH_TEST_QUIET_FD_TABLE"

// quietFdTableTimeout bounds the child. It is a placement case in a process
// with nothing else in it, which is milliseconds; the number is here so that a
// regression that hangs is a failure with the child's output rather than the
// parent's own timeout panic, which would leave the child behind.
const quietFdTableTimeout = 2 * time.Minute

// measuredInAQuietDescriptorTable runs this case in a child of this test
// binary, whose descriptor table holds nothing but stdio, and reports the
// child's result here. It answers true when it has done that, and the caller
// returns rather than measuring anything in this process.
//
// # Why a placement case cannot be measured where the other cases run
//
// The placement cases ask where a process substitution's far end lands, and a
// `/dev/fd/N` is a number in the kernel's table for the **whole test binary**.
// They can only see their rule in a table that has room for it: the descent
// from the top of the table walks [10,63] and every upward rule offers
// sixty-four numbers from its base, so a process holding more than that
// answers by none of them.
//
// The binary holds far more than that by the time these cases run. Measured
// 2026-09-25 with a census goroutine over `go test ./interp/`: **about a
// hundred descriptors open**, and the whole of [10,63] taken on the macOS
// runner. They are this package's own. A finished Runner's coprocess ends are
// taken out of its table without being closed — deliberately, see
// forgetCoprocFd, because a script may have duplicated one onto a number of
// its own — so the open file is left for the garbage collector to reclaim, and
// the coprocess cases alone put some sixty of them there. How many are still
// open at any moment is therefore a question about **when the collector last
// ran**, which is why this is a required check that fails and then passes on a
// rerun (#4493). `GOGC=off` is the whole of the reproduction.
//
// # Why a child and not a restart
//
// #4462 made this binary exec *itself* at startup with every inherited
// descriptor marked close-on-exec, which answered the table the **host** hands
// down (#4459) and is what that issue was. It cannot answer this one: it runs
// before any test does, and what fills the table happens afterwards. There is
// no moment during the run when the table can be emptied either — the numbers
// belong to files this process still references, and closing a descriptor
// another test is using is not a measurement, it is a fault.
//
// So the measurement moves to a process where the question can be asked. A
// child starts with stdio, its own runtime's descriptors and nothing else,
// whatever this binary has accumulated, and it stays that way however much
// more it accumulates: each case gets its own. That is the same discipline
// #4462 was reaching for — the environment is an input, so pin it rather than
// measure around it — applied where it holds for the whole run rather than for
// the first instant of it.
//
// The one thing a child does not get for free is the descriptors the *host*
// handed down: os/exec closes nothing it did not open, and an inherited number
// without close-on-exec is inherited again. keepInheritedDescriptorsOutOfChildren
// is what covers that, and it is the surviving half of #4462's restart.
//
// # A child that ran nothing exits 0
//
// Which is the same exit status, and very nearly the same output, as a child
// that ran the case and passed — so the parent requires the child to say it
// started this case by name before it will believe anything it exited with. A
// `-test.run` that matches nothing is otherwise a silent pass for every case
// in this file at once.
func measuredInAQuietDescriptorTable(t *testing.T) bool {
	t.Helper()
	if os.Getenv(quietFdTableEnv) != "" {
		return false
	}
	name := t.Name()
	if strings.Contains(name, "/") {
		t.Fatalf("%s: a quiet table is asked for by the case, not by a subtest — "+
			"the child is selected by the whole of -test.run", name)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("finding this test binary to run %s in a quiet table: %v", name, err)
	}
	cmd := exec.Command(exe, "-test.run=^"+regexp.QuoteMeta(name)+"$", "-test.v",
		"-test.timeout="+quietFdTableTimeout.String())
	cmd.Env = append(os.Environ(), quietFdTableEnv+"=1")
	out, runErr := cmd.CombinedOutput()
	if !strings.Contains(string(out), "=== RUN   "+name+"\n") {
		t.Fatalf("the quiet-table run of %s never started it (%v) — a child that "+
			"matched no test exits like one that passed:\n%s", name, runErr, out)
	}
	if runErr != nil {
		t.Errorf("in a descriptor table holding nothing but stdio (%v):\n%s", runErr, out)
	}
	return true
}

// keepInheritedDescriptorsOutOfChildren marks every descriptor this binary was
// handed close-on-exec, so that none of them reaches the children the cases
// above measure in.
//
// os/exec gives a child stdio and whatever it was told to pass, and closes
// nothing else — it does not have to, because the Go runtime opens everything
// close-on-exec. A descriptor the **host** opened carries no such flag, so a
// runner that starts `go test` holding seventy of them hands those same
// seventy to every child of it as well (#4459), and the quiet table would not
// be quiet.
//
// Called once, from TestMain, before anything is running: at that point every
// number above stdio is either the host's or the runtime's, the flag is what
// the runtime's already have, and setting it changes nothing about how this
// process may use either. A refusal is an empty number and not an error.
func keepInheritedDescriptorsOutOfChildren() {
	for fd := 3; fd < highestInheritedFd; fd++ {
		_, _, _ = syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
			uintptr(syscall.F_SETFD), uintptr(syscall.FD_CLOEXEC))
	}
}

// fdIsOpen says whether this process holds the number fd.
//
// F_GETFD is the cheapest question that distinguishes a live entry from an
// empty one, and it takes nothing: a probe that duplicated the number to find
// out would be holding it while the next one is asked about.
func fdIsOpen(fd int) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
		uintptr(syscall.F_GETFD), 0)
	return errno == 0
}
