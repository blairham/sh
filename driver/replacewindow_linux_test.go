// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package driver_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/driver"
)

// Nothing may allocate between the descriptor placement and the execve, and
// this counts rather than trusts.
//
// It is the rule the whole of #731 comes down to. The window is one where this
// process's descriptor table says something the Go runtime does not expect —
// a script's file sitting on the number the netpoller's epoll descriptor was
// on — and the runtime finding out is fatal rather than reportable. Stopping
// the collector removes the collections; it cannot remove `sysmon`, which
// polls on a timer from another thread and only needs the window to be *long*.
// So the window is emptied.
//
// A conversion moved back below the placement would compile and would pass
// every test that runs a shell, because the failure is a timer landing in a
// microsecond gap. What it would not do is keep this count at zero.
//
// Linux only, which is where the claim is: everywhere else the execve still
// goes through syscall.Exec and the conversions are still inside the window.
// See replaceexec_other.go for why that is a decision rather than a gap.
func TestNothingAllocatesInTheWindow(t *testing.T) {
	// An environment big enough that the conversions could not hide in the
	// noise if they were in here: each entry is its own allocation.
	env := make([]string, 0, 2000)
	for i := range 2000 {
		env = append(env, "SH_PAD_"+strconv.Itoa(i)+"=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	}
	argv := []string{"x", "-c", "true"}

	// Everything the seam needs is built before the seam runs, because the
	// seam is inside the window it is measuring: a map grown or a struct
	// escaping while it counts would be counted, and was — the first version
	// of this test wrote into a map and reported its own allocation as the
	// shell's.
	var stats runtime.MemStats
	var atBefore, atExecve uint64
	var sawBefore, sawExecve bool
	driver.AtPlacementForTest(func(where string) {
		// Safe to ask here, and only here: the table handed over is empty, so
		// nothing has been overwritten and the runtime still owns everything
		// it thinks it owns. A shell that had just placed a real table could
		// not afford this call.
		switch where {
		case "before":
			runtime.ReadMemStats(&stats)
			atBefore, sawBefore = stats.Mallocs, true
		case "execve":
			runtime.ReadMemStats(&stats)
			atExecve, sawExecve = stats.Mallocs, true
		}
	})
	t.Cleanup(func() { driver.AtPlacementForTest(nil) })

	err := driver.ReplaceProcessForTest(t.TempDir()+"/not-a-program", argv, env, nil)
	if err == nil {
		t.Fatal("the exec did not fail, so the failure path was never taken")
	}
	if !sawBefore || !sawExecve {
		t.Fatalf("the replacement did not reach both ends of the window (before=%v execve=%v)",
			sawBefore, sawExecve)
	}
	if got := atExecve - atBefore; got != 0 {
		t.Errorf("%d allocation(s) between the placement and the execve, want none —"+
			" something the exec needs is being built after the descriptors are"+
			" overwritten, which is #731", got)
	}
}

// A replacement inherits the open-file limit an ordinary child gets.
//
// Going around syscall.Exec means going around the one thing it does besides
// the execve: it puts RLIMIT_NOFILE back to what the process started with,
// because the Go runtime raised it before any of this ran. Losing that would
// hand a command a soft limit a thousand times the one bash hands it, which is
// exactly what a low soft limit exists to prevent, and nothing else in this
// package would have noticed.
//
// So it is asserted from the outside, on the two routes agreeing, rather than
// on the mechanism that keeps them agreeing. `driver/rlimit.go` records the
// same asymmetry from the reading side and is worth reading beside this.
func TestAReplacementInheritsTheOpenFileLimitAChildGets(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	replaced := filepath.Join(dir, "replaced")
	runAsShellReplacingItself(t, "TestAReplacementInheritsTheOpenFileLimitAChildGets",
		"/bin/sh -c 'ulimit -n' >"+child+"\n"+
			"exec /bin/sh -c 'ulimit -n' >"+replaced+"\n",
		func(string) bool { return wrote(replaced) })

	a, _ := os.ReadFile(child)
	b, _ := os.ReadFile(replaced)
	got, want := strings.TrimSpace(string(b)), strings.TrimSpace(string(a))
	if want == "" {
		t.Skip("this machine's /bin/sh did not report a limit, so there is nothing to compare")
	}
	if got != want {
		t.Errorf("a replacement runs at ulimit -n %q where an ordinary child gets %q —"+
			" the runtime's raised limit is reaching the command", got, want)
	}
}

// A failed exec leaves the shell as it found it, the open-file limit included.
//
// The limit is lowered for the command's sake before the descriptors are
// placed, and a command that never ran has no claim on it — the same rule the
// collector two lines above already follows, and the same reason: on this path
// there is still a program to put things back for.
//
// In a process of its own, and that is not tidiness. The runtime raises this
// limit once and `syscall.Exec` restores it once, so only the *first*
// replacement in a process has anything to lower — a second one finds nothing
// to do and would pass whatever the code did. Asserting it in this test binary
// would make the answer depend on which test ran first.
//
// Not observable from a script either way, because a failed `exec` ends the
// shell in every shell measured. It is observable in a program that holds a
// Runner and carries on, and in this test binary, which would otherwise spend
// the rest of its run at the lowered limit.
func TestAFailedExecPutsTheOpenFileLimitBack(t *testing.T) {
	if os.Getenv(limitProbeChild) != "" {
		reportLimitAcrossAFailedExec()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestAFailedExecPutsTheOpenFileLimitBack")
	cmd.Env = append(os.Environ(), limitProbeChild+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the child could not be run: %v\n%s", err, out)
	}
	var entry, seam, back uint64
	if _, err := fmt.Sscanf(limitLine(string(out)), "limits entry=%d seam=%d back=%d", &entry, &seam, &back); err != nil {
		t.Fatalf("the child did not report its limits: %v\n%s", err, out)
	}
	if seam == entry {
		t.Skipf("nothing was raised on this machine, so there was nothing to put back (limit %d)", entry)
	}
	if back != entry {
		t.Errorf("after a failed exec the soft open-file limit is %d, want the %d it was at before"+
			" — it was lowered to %d for a command that never ran", back, entry, seam)
	}
}

// limitProbeChild names the run that is the subject rather than the observer.
const limitProbeChild = "SH_TEST_LIMIT_ACROSS_A_FAILED_EXEC"

func limitLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "limits ") {
			return line
		}
	}
	return ""
}

// reportLimitAcrossAFailedExec is the child half: the limit as this process
// found it, as it stood once the placement was reached, and as it stands after
// the exec failed.
func reportLimitAcrossAFailedExec() {
	soft := func() uint64 {
		var l syscall.Rlimit
		if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &l) != nil {
			return 0
		}
		return l.Cur
	}
	entry := soft()
	var seam uint64
	driver.AtPlacementForTest(func(where string) {
		if where == "before" {
			seam = soft()
		}
	})
	defer driver.AtPlacementForTest(nil)
	dir, err := os.MkdirTemp("", "limit")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)
	_ = driver.ReplaceProcessForTest(filepath.Join(dir, "not-a-program"), []string{"x"}, nil, nil)
	fmt.Printf("limits entry=%d seam=%d back=%d\n", entry, seam, soft())
}
