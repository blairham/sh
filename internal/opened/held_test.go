// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/opened"
)

// A hold is worth having only if it survives the thing a name does not, so the
// rename is the positive this file is built around: every assertion below
// about a hold still answering is worthless unless the control shows the two
// answers actually differ. They do, and the control is the first row.
func TestAHoldFollowsTheDirectoryThroughARename(t *testing.T) {
	base := t.TempDir()
	was := filepath.Join(base, "d")
	if err := os.Mkdir(was, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := opened.Hold(was)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	defer held.Close()

	before, ok := opened.Path(held)
	if !ok {
		t.Fatal("a directory this process just made has no name")
	}
	now := filepath.Join(base, "e")
	if err := os.Rename(was, now); err != nil {
		t.Fatal(err)
	}
	after, ok := opened.Path(held)
	if !ok {
		t.Fatal("the hold lost its name across a rename")
	}
	if before == after {
		t.Fatalf("the answer did not move across a rename: %q both times — "+
			"this test cannot tell a hold from a string", before)
	}
	if filepath.Base(after) != "e" {
		t.Errorf("after a rename the hold is called %q, want the new name", after)
	}
}

// The case no name reaches: the directory's own name is gone and the one thing
// left that can say "here" is the descriptor.
func TestWithinOpensBesideAHoldWhoseNameHasGone(t *testing.T) {
	base := t.TempDir()
	was := filepath.Join(base, "d")
	if err := os.MkdirAll(filepath.Join(was, "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := opened.Hold(was)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	defer held.Close()
	if err := os.Rename(was, filepath.Join(base, "e")); err != nil {
		t.Fatal(err)
	}
	// The control, and it is the positive this test needs: the name really is
	// gone, so an open through it really would fail.
	if _, err := os.Stat(filepath.Join(was, "s")); err == nil {
		t.Fatal("the old name still reaches the subdirectory; the case is not set up")
	}
	sub, err := opened.Within(held, "s")
	if err != nil {
		t.Fatalf("Within through a held directory: %v", err)
	}
	defer sub.Close()
	name, ok := opened.Path(sub)
	if !ok || filepath.Base(name) != "s" {
		t.Errorf("Within reached %q (named %v), want the subdirectory under the new name", name, ok)
	}
	if filepath.Base(filepath.Dir(name)) != "e" {
		t.Errorf("Within reached %q, want it under the directory's new name", name)
	}
}

// `..` is the other half of what a `cd` asks for, and a pop rather than a
// lookup is the only way to ask it once the name is gone.
func TestWithinReachesTheParentOfAHold(t *testing.T) {
	base := t.TempDir()
	was := filepath.Join(base, "d")
	if err := os.Mkdir(was, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := opened.Hold(was)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := os.Rename(was, filepath.Join(base, "e")); err != nil {
		t.Fatal(err)
	}
	up, err := opened.Within(held, "..")
	if err != nil {
		t.Fatalf("Within(..): %v", err)
	}
	defer up.Close()
	name, ok := opened.Path(up)
	if !ok {
		t.Fatal("the parent of a held directory has no name")
	}
	if !sameDirectory(t, name, base) {
		t.Errorf("Within(..) reached %q, want the parent %q", name, base)
	}
}

// A directory whose last link has gone is the harsher case: no name reaches it
// and the process is still in it, which is what `cd .` after an rmdir asks
// about.
func TestWithinStillOpensADirectoryThatHasBeenRemoved(t *testing.T) {
	base := t.TempDir()
	gone := filepath.Join(base, "d")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := opened.Hold(gone)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("the removed directory still stats; the case is not set up")
	}
	self, err := opened.Within(held, ".")
	if err != nil {
		t.Fatalf("Within(.) on a removed directory: %v", err)
	}
	defer self.Close()
	if _, err := self.Stat(); err != nil {
		t.Errorf("the descriptor Within gave back is not a directory: %v", err)
	}
}

// The hold uses the traverse flags rather than a readable open, so a directory
// a shell can sit in and not list is one it can still be held by. This is the
// row walkat_darwin.go's comment is about, put to a test here because the hold
// is a second caller of those flags and a change to them would have to keep
// both callers true.
func TestAHoldTakesADirectoryThatMayBeEnteredAndNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this row is about")
	}
	base := t.TempDir()
	thin := filepath.Join(base, "thin")
	if err := os.Mkdir(thin, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(thin, 0o755) })
	if _, err := os.ReadDir(thin); err == nil {
		t.Fatal("the directory is readable; the case is not set up")
	}
	held, err := opened.Hold(thin)
	if err != nil {
		t.Fatalf("Hold on a traversable directory that cannot be read: %v", err)
	}
	held.Close()
}

func sameDirectory(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}
