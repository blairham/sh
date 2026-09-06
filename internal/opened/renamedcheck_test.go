// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestARenameInTheCheckWindowCarriesADeniedObjectPastTheGate is the #1050
// escape, at the one layer where it can be written down without a coin.
//
// The window is between the open and the question, and it is a window because
// the question is asked of the *platform*: Verified opens the file, and the
// check it then calls reads back what the kernel says the descriptor holds. A
// rename that lands in between moves that answer — 2000 out of 2000 on Darwin
// and 4000 out of 4000 on Linux, measured in the test beside this one — so the
// gate is consulted about a name the object no longer has while the descriptor
// holds the object it did have.
//
// Nothing in the interpreter or in internal/boundary can be hooked in that
// window, which is why an end-to-end version of this is a race and measures
// 129 runs in 600 on Linux and 91 in 600 on Darwin rather than 600 in 600. The
// check callback *is* the window, so a test that renames inside it states the
// same fact once instead of sampling it, and states it identically on both
// platforms.
//
// It asserts the escape happens. That is the uncomfortable direction and it is
// the point: closing this makes this test red, which is what keeps it and
// docs/design/sandboxing.md from being able to disagree. A version tolerant of
// either answer would pass whether or not the gate held, which for a boundary
// is a test that asserts nothing.
func TestARenameInTheCheckWindowCarriesADeniedObjectPastTheGate(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := t.TempDir()
	for _, sub := range []string{"secret", "pub", "work"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The object, at the name a rule would speak about, and a symbolic link
	// pointing straight at it. This is #703's fixture: the name the caller
	// writes is innocent and the object it reaches is not.
	denied := filepath.Join(dir, "secret", "data")
	if err := os.WriteFile(denied, []byte("classified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(dir, "pub", "data")
	link := filepath.Join(dir, "work", "innocent")
	if err := os.Symlink(denied, link); err != nil {
		t.Fatal(err)
	}

	var asked string
	var elsewhere bool
	f, err := Verified(link, os.O_RDONLY, 0, func(open *os.File) error {
		// The attacker's move, landing where it has to land. No privilege the
		// opener does not have, and the descriptor is not touched.
		if err := os.Rename(denied, parked); err != nil {
			return err
		}
		asked, elsewhere = Elsewhere(open, link)
		return nil
	})
	if err != nil {
		t.Fatalf("Verified failed: %v", err)
	}
	defer func() { _ = f.Close() }()

	if !elsewhere {
		t.Fatal("Elsewhere reported the open as having stayed at the name it was asked about")
	}
	physical, err := filepath.EvalSymlinks(parked)
	if err != nil {
		t.Fatal(err)
	}
	if asked != physical {
		t.Fatalf("a gate would be consulted about %q, want the parked name %q — this test "+
			"records the escape, and a change that closed it belongs in "+
			"docs/design/sandboxing.md in the same commit", asked, physical)
	}

	// The two halves that make it an escape rather than a file having moved.
	// The descriptor still holds the denied object, so the bytes a gate has
	// just been told to allow are the bytes the rule was about.
	body := make([]byte, 32)
	n, err := f.Read(body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body[:n]); got != "classified\n" {
		t.Errorf("the descriptor holds %q, want the denied object's contents", got)
	}
	// And putting the name back leaves no trace: the object is where the rule
	// speaks about it, and the name the gate was consulted about is gone.
	if err := os.Rename(parked, denied); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(parked); !os.IsNotExist(err) {
		t.Errorf("the parked name still exists (%v)", err)
	}
}
