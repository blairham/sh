// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestARenameInTheCheckWindowDoesNotCarryADeniedObjectPastTheGate is the same
// fixture, the same instant and the same rename as the test this replaces, with
// the assertion turned around because the answer turned around.
//
// It was `…CarriesADeniedObjectPastTheGate`, and it asserted the escape: the
// window between the open and the question was a window because the question
// was asked of the *platform*, and a rename landing in it moved the platform's
// answer — 2000 out of 2000 on Darwin and 4000 out of 4000 on Linux — so the
// gate was consulted about a name the object no longer had while the descriptor
// still held it. End to end that leaked 129 runs in 600 on Linux and 91 in 600
// on Darwin.
//
// The walk closed it, and the flip is stated here rather than left to be
// noticed: the name now comes from the components the open traversed, so there
// is nothing for a later rename to move. The rename still happens, at the same
// instant, and the gate is consulted about the path the open **went through**.
//
// Both directions are asserted, because only one of them is the interesting
// half. That the answer is the denied name is the fix; that the answer is *not*
// the parked name is the escape, named, so that a change reintroducing a
// read-it-back step fails here with the escape's own name in the message rather
// than with a diff of two strings.
func TestARenameInTheCheckWindowDoesNotCarryADeniedObjectPastTheGate(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	for _, sub := range []string{"secret", "pub", "work"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// #703's fixture: the name the caller writes is innocent and the object it
	// reaches is not.
	denied := filepath.Join(dir, "secret", "data")
	if err := os.WriteFile(denied, []byte("classified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(dir, "pub", "data")
	link := filepath.Join(dir, "work", "innocent")
	if err := os.Symlink(denied, link); err != nil {
		t.Fatal(err)
	}
	// The physical spellings, taken here in the test because the test needs an
	// oracle for what the walk will report. Nothing in the walk resolves a path
	// through this call, so the two arriving at the same answer is a claim
	// rather than a tautology.
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	deniedPhysical := filepath.Join(physical, "secret", "data")
	parkedPhysical := filepath.Join(physical, "pub", "data")

	var asked string
	var elsewhere bool
	f, err := Verified(link, os.O_RDONLY, 0, func(r Reached) error {
		// The attacker's move, landing exactly where it has to land: after
		// the open, before the question. No privilege the opener does not
		// have, and the descriptor is not touched.
		if err := os.Rename(denied, parked); err != nil {
			return err
		}
		asked, elsewhere = Elsewhere(r, link)
		return nil
	})
	if err != nil {
		t.Fatalf("Verified failed: %v", err)
	}
	defer func() { _ = f.Close() }()
	// Put it back before any assertion can fail and leave the fixture crooked.
	if err := os.Rename(parked, denied); err != nil {
		t.Fatal(err)
	}

	if !elsewhere {
		t.Fatal("Elsewhere reported the open as having stayed at the name it was asked about; " +
			"a symbolic link into a denied place must still be a second consultation")
	}
	if asked == parkedPhysical {
		t.Fatalf("a gate would be consulted about the parked name %q — the rename moved the "+
			"answer, which is #1050 and is the thing the walk exists to make impossible", asked)
	}
	if asked != deniedPhysical {
		t.Fatalf("a gate would be consulted about %q, want the path the open traversed, %q",
			asked, deniedPhysical)
	}

	// And the descriptor still holds that object, so the name the gate judged
	// and the bytes the caller gets are the same thing.
	body := make([]byte, 32)
	n, err := f.Read(body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body[:n]); got != "classified\n" {
		t.Errorf("the descriptor holds %q, want the object the walk named", got)
	}
}
