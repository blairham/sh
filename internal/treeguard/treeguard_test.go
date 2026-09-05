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
