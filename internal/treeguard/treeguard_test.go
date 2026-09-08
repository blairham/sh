// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package treeguard

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestSnapshotNamesFilesAndNotDirectories: a directory a test makes and then
// empties has left nothing behind, so counting directories would report a
// stray where there is none.
func TestSnapshotNamesFilesAndNotDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "top.txt"))
	write(t, filepath.Join(dir, "sub", "nested.txt"))

	got, err := snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"top.txt": true, filepath.Join("sub", "nested.txt"): true}
	if len(got) != len(want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
	for p := range want {
		if !got[p] {
			t.Errorf("snapshot did not name %q", p)
		}
	}
}

// TestOnlyWhatArrivedIsReported. A file that was already there when the run
// started belongs to whoever put it there — a scratch file in a checkout is
// not the suite's doing — and reporting it would make the guard cry wolf
// exactly where a real stray is easiest to miss.
func TestOnlyWhatArrivedIsReported(t *testing.T) {
	before := map[string]bool{"kept.txt": true, "gone.txt": true}
	after := map[string]bool{"kept.txt": true, "stray.txt": true, "sub/stray.txt": true}
	got := addedPaths(before, after)
	want := []string{"stray.txt", "sub/stray.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("added = %v, want %v", got, want)
	}
}

// TestNothingAddedIsNothingReported, which is the state every run is supposed
// to be in — a guard that fires on a clean run is a guard people turn off.
func TestNothingAddedIsNothingReported(t *testing.T) {
	same := map[string]bool{"a": true, "b": true}
	if got := addedPaths(same, same); len(got) != 0 {
		t.Errorf("added = %v, want nothing", got)
	}
}

// TestAPassingRunStillFailsWhenItLeavesAFile: the whole point. The damage this
// guards against is silent precisely because the test that does it passes, so
// a status that only reflects the tests would report success on the run that
// wrote the file.
func TestAPassingRunStillFailsWhenItLeavesAFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if got := Run(passingRun{}); got != 0 {
		t.Errorf("a clean passing run returned %d, want 0", got)
	}
	if got := Run(passingRun{leaves: filepath.Join(dir, "stray.txt")}); got == 0 {
		t.Error("a passing run that left a file returned 0, want a failure")
	}
	// A failing run keeps its own status: the tests failing is the more
	// useful report, and overwriting it with the guard's would hide it.
	if got := Run(runner{code: 2}); got != 2 {
		t.Errorf("a failing run returned %d, want its own 2", got)
	}
}

type runner struct{ code int }

func (r runner) Run() int { return r.code }

type passingRun struct{ leaves string }

func (p passingRun) Run() int {
	if p.leaves != "" {
		_ = os.WriteFile(p.leaves, []byte("x"), 0o600)
	}
	return 0
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestTempCatchesTheEmptyDirectoryRunCannotSee, which is the reason Temp
// exists rather than a second call to Run.
//
// Both halves matter. That Run reports nothing for an empty directory is what
// makes it the wrong instrument for #1284 — a guard the leak walks straight
// past — and that Temp reports it is what makes the pair worth keeping.
func TestTempCatchesTheEmptyDirectoryRunCannotSee(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sh-procsub123"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("snapshot named %v, want nothing — Run counts files, and an empty directory is none", before)
	}
	if got := strays(dir); len(got) != 1 {
		t.Errorf("strays = %v, want the one empty directory", got)
	}
}

// TestTempFailsARunThatLeavesADirectoryInItsTemporaryOne: the guard doing its
// job, end to end, on a passing run — because that is the shape the leak had.
func TestTempFailsARunThatLeavesADirectoryInItsTemporaryOne(t *testing.T) {
	if got := Temp(passingRun{}).Run(); got != 0 {
		t.Errorf("a clean passing run returned %d, want 0", got)
	}
	if got := Temp(leavesADirInTemp{}).Run(); got == 0 {
		t.Error("a passing run that left a directory in TMPDIR returned 0, want a failure")
	}
	// A failing run keeps its own status, the same rule Run follows.
	if got := Temp(runner{code: 2}).Run(); got != 2 {
		t.Errorf("a failing run returned %d, want its own 2", got)
	}
}

// TestTempPutsTMPDIRBack. A wrapper outside this one goes on running once the
// scratch directory has been removed, and leaving TMPDIR pointing at a path
// that is no longer there would break it in a way nothing here would report.
func TestTempPutsTMPDIRBack(t *testing.T) {
	// A real directory, because the scratch one is made under it: the
	// guard degrades to not guarding when it cannot make one, which is
	// deliberate but would make this test pass for the wrong reason.
	outer := t.TempDir()
	t.Setenv("TMPDIR", outer)
	var inside string
	Temp(runFunc(func() int { inside = os.Getenv("TMPDIR"); return 0 })).Run()
	if inside == outer || inside == "" {
		t.Errorf("TMPDIR during the run was %q, want a scratch directory of its own", inside)
	}
	if got := os.Getenv("TMPDIR"); got != outer {
		t.Errorf("TMPDIR afterwards = %q, want the original back", got)
	}
}

// leavesADirInTemp is a passing run that makes an empty directory in TMPDIR
// and does not remove it — a shell with a process substitution, in miniature.
type leavesADirInTemp struct{}

func (leavesADirInTemp) Run() int {
	_, _ = os.MkdirTemp(os.Getenv("TMPDIR"), "sh-procsub")
	return 0
}

type runFunc func() int

func (f runFunc) Run() int { return f() }
