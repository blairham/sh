// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package childguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOnlyThisRunsProcessesAreReported. Several agents run these suites at
// once on one machine, so a process holding a pipe of somebody else's is not
// this run's leak — and reporting it is how a guard teaches people to ignore
// it.
func TestOnlyThisRunsProcessesAreReported(t *testing.T) {
	all := []Process{
		{PID: 100, PPID: 1, Command: "cat /tmp/sh-procsub9/sub1"},
		{PID: 200, PPID: 100, Command: "cat /tmp/sh-procsub9/sub2"},
		{PID: 300, PPID: 200, Command: "cat /tmp/sh-procsub9/sub3"},
	}
	got := strays(all, 100, "sh-procsub")
	if len(got) != 2 || got[0].PID != 200 || got[1].PID != 300 {
		t.Errorf("strays = %v, want the two below 100 and not 100 itself", got)
	}
	if n := len(strays(all, 999, "sh-procsub")); n != 0 {
		t.Errorf("%d strays under a pid that is nobody's parent, want none", n)
	}
}

// TestAGrandchildIsFoundHoweverThePidsAreOrdered. `ps` orders by pid, so a
// child can be listed before its parent — and a single pass down the table
// would then miss the generation below it.
func TestAGrandchildIsFoundHoweverThePidsAreOrdered(t *testing.T) {
	all := []Process{
		{PID: 5, PPID: 9, Command: "cat /tmp/sh-procsub9/sub2"},
		{PID: 9, PPID: 100, Command: "sh -c :"},
	}
	got := strays(all, 100, "sh-procsub")
	if len(got) != 1 || got[0].PID != 5 {
		t.Errorf("strays = %v, want the grandchild listed before its parent", got)
	}
}

// TestOnlyWhatHoldsAPipeIsReported. Some tests mean to leave a process
// running: a job started with `&` and not waited for is the subject of several
// of them. A guard that reported those would be turned off within the day.
func TestOnlyWhatHoldsAPipeIsReported(t *testing.T) {
	all := []Process{
		{PID: 200, PPID: 100, Command: "/bin/sleep 30"},
		{PID: 201, PPID: 100, Command: "cat /tmp/sh-procsub9/sub1"},
	}
	got := strays(all, 100, "sh-procsub")
	if len(got) != 1 || got[0].PID != 201 {
		t.Errorf("strays = %v, want only the one holding a pipe", got)
	}
}

// TestAPassingRunStillFailsWhenItLeavesAProcess: the whole point, and the
// shape of the leak this was written for. The test that leaked most sharply
// was the one asserting the cleanup, which passed while leaking — so a status
// that only reflects the tests reports success on exactly the run that did the
// damage.
func TestAPassingRunStillFailsWhenItLeavesAProcess(t *testing.T) {
	if got := Wrap(runner{code: 0}, "sh-procsub").Run(); got != 0 {
		t.Errorf("a clean passing run returned %d, want 0", got)
	}
	// A real process, holding a real FIFO, started by this test binary — the
	// arrangement the guard exists to name, built rather than faked, because
	// a fake would prove only that the filter works.
	marker := "childguard-leak"
	dir := t.TempDir()
	held := startAStrayHoldingAPipe(t, dir, marker)
	if got := (guarded{m: runner{code: 0}, marker: marker, grace: 50 * time.Millisecond}).Run(); got == 0 {
		t.Error("a passing run that left a process holding a pipe returned 0, want a failure")
	}
	// A failing run keeps its own status: the tests failing is the more
	// useful report, and overwriting it with the guard's would hide it.
	if got := (guarded{m: runner{code: 2}, marker: marker, grace: 50 * time.Millisecond}).Run(); got != 2 {
		t.Errorf("a failing run returned %d, want its own 2", got)
	}
	_ = held.Process.Kill()
	_, _ = held.Process.Wait()
}

// startAStrayHoldingAPipe leaves a `cat` blocked in open(2) on a FIFO nothing
// will ever write to, which is the state every one of the leaked processes was
// found in.
func startAStrayHoldingAPipe(t *testing.T, dir, marker string) *exec.Cmd {
	t.Helper()
	path := filepath.Join(dir, marker)
	if err := mkfifo(path); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	cmd := exec.Command("cat", path)
	if err := cmd.Start(); err != nil {
		t.Skipf("no cat here: %v", err)
	}
	// Started is not yet blocked in open, and the guard would report nothing
	// for a process that has not been listed by `ps` yet.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		found, err := Holding(os.Getpid(), marker)
		if err != nil {
			t.Skipf("no process table here: %v", err)
		}
		if len(found) > 0 {
			return cmd
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	t.Fatal("the stray never appeared in the process table")
	return nil
}

type runner struct{ code int }

func (r runner) Run() int { return r.code }

// TestTheMarkerIsMatchedAnywhereInTheCommandLine, because the pipe is an
// argument to the command rather than the command itself: what was found was
// `cat <TMPDIR>/sh-procsubNNNN/sub1`, and matching only the first word would
// have seen `cat`.
func TestTheMarkerIsMatchedAnywhereInTheCommandLine(t *testing.T) {
	all := []Process{{PID: 2, PPID: 1, Command: "cat /tmp/T/x/sh-procsub12/sub1 /tmp/T/x/sh-procsub12/sub2"}}
	if got := strays(all, 1, "sh-procsub"); len(got) != 1 {
		t.Errorf("strays = %v, want the command whose argument names the pipe", got)
	}
}

// TestACensusNamesThisProcess, which is the one row every run is guaranteed to
// have and so the only assertion that can be made about a real table.
func TestACensusNamesThisProcess(t *testing.T) {
	all, err := census()
	if err != nil {
		t.Skipf("no process table here: %v", err)
	}
	for _, p := range all {
		if p.PID == os.Getpid() {
			if p.PPID <= 0 {
				t.Errorf("this process is listed with parent %d, want a real one", p.PPID)
			}
			if !strings.Contains(p.Command, "childguard") {
				t.Errorf("this process is listed as %q, want the test binary's own name in it", p.Command)
			}
			return
		}
	}
	t.Errorf("the census of %d processes did not name this one (%d)", len(all), os.Getpid())
}
