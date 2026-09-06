// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestARenameMovesTheAnswerForADescriptorThatHasNotMoved is the second half of
// the measurement this package rests on, and it is the half that is not about
// hard links and not about one platform.
//
// The sentence the check is built on is that the kernel answers with whichever
// name the descriptor was opened by. TestTwoNamesForOneObjectAndWhetherTheAnswerHolds
// measures where that fails for an object with *two* names — Darwin only,
// because Linux keeps one dentry per name. This measures where it fails for an
// object with **one**, and there both platforms fail, for the same reason
// stated two ways: a name is a property of the filesystem and not of the
// descriptor, so moving the name moves the answer.
//
//   - Linux answers from the dentry the descriptor was opened through, and a
//     rename *moves that dentry*. /proc reports where it is now.
//   - Darwin answers from the vnode's single name, and a rename re-stamps it.
//
// There is no concurrency here on purpose. The escape in #1050 needs a race —
// the rename has to land between the open and the question, which an
// unprivileged loop hits 21.5% of the time on Linux 6.8 and 15.2% on Darwin
// 25.5.0 through the whole interpreter — but the *premise failure* needs
// nothing at all, and a test of a race would be the flake #1029 was. Done in
// order, with one rename, the answer moves 2000 times out of 2000 on Darwin
// and 4000 out of 4000 on Linux across two filesystems.
//
// So this asserts the premise is false, in one direction, on both platforms.
// A kernel that pinned the name would fail here, and that is the moment to
// revisit docs/design/sandboxing.md rather than the moment to discover the
// claim had quietly become true and the limitation had quietly gone away.
func TestARenameMovesTheAnswerForADescriptorThatHasNotMoved(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"denied", "allowed"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := filepath.Join(dir, "denied", "data")
	after := filepath.Join(dir, "allowed", "data")
	if err := os.WriteFile(before, []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(before)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	opened, ok := Path(f)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if ok {
			t.Fatalf("Path answered %q on a platform with no way to ask", opened)
		}
		return
	}
	if !ok {
		t.Fatal("Path had no answer for an ordinary file")
	}
	if filepath.Base(filepath.Dir(opened)) != "denied" {
		t.Fatalf("Path = %q before the rename, want the name it was opened by", opened)
	}

	// One rename, by the ordinary call, needing no privilege the opener does
	// not have and touching neither the descriptor nor the object's contents.
	if err := os.Rename(before, after); err != nil {
		t.Fatal(err)
	}

	moved, ok := Path(f)
	if !ok {
		t.Fatal("Path stopped having an answer for a descriptor still open")
	}
	if filepath.Base(filepath.Dir(moved)) != "allowed" {
		t.Errorf("Path = %q after the object's only name was moved, want the new name — a "+
			"descriptor does not pin a name on either platform, and that is what the "+
			"rename entry in docs/design/sandboxing.md is about; this kernel no longer "+
			"behaves that way", moved)
	}
}
