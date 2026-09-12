// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// The table a replacement is given reaches down to zero, and it has to: a
// replacement is this process, so the numbers it finds are the numbers this
// process is holding, and nothing renumbers 0, 1 and 2 on the way across the
// way os/exec does for a child.
func TestAReplacementsTableHoldsTheNamedStreamsAtTheirOwnNumbers(t *testing.T) {
	in, out, errs := openScratch(t), openScratch(t), openScratch(t)
	parked := openScratch(t)

	r := newTestRunner(t, &Runner{Stdin: in, Stdout: out, Stderr: errs})
	r.setFd(4, parked)

	files := r.replacementFiles()
	if len(files) != 5 {
		t.Fatalf("table has %d entries, want one per descriptor through 4", len(files))
	}
	for fd, want := range map[int]*os.File{0: in, 1: out, 2: errs, 4: parked} {
		if files[fd] != want {
			t.Errorf("descriptor %d = %v, want the file behind it", fd, files[fd])
		}
	}
	if files[3] != nil {
		t.Errorf("descriptor 3 = %v, want a hole — nothing was opened there", files[3])
	}
}

// Only a real file can cross, and a stream that is not one leaves a nil rather
// than a number. A nil means the same thing everywhere in this table — the
// descriptor must not be open there — so a caller's buffer behind standard
// output leaves the replacement with no standard output, rather than with the
// process's own stream that the script never named.
func TestAStreamThatIsNotAFileDoesNotCrossToAReplacement(t *testing.T) {
	r := newTestRunner(t, &Runner{Stdin: bytes.NewReader(nil), Stdout: &bytes.Buffer{}, Stderr: io.Discard})
	files := r.replacementFiles()
	if len(files) != firstExtraFd {
		t.Fatalf("table has %d entries, want the three named streams and nothing above them", len(files))
	}
	for fd, f := range files {
		if f != nil {
			t.Errorf("descriptor %d = %v, want nothing placeable for a stream that is not a file", fd, f)
		}
	}
}

// A stream closed with `>&-` is a nil for the same reason and to the same
// effect, which is what makes the two readings of a nil one reading: every
// shell in the panel leaves such a descriptor closed in the command that
// `exec` runs next.
func TestAClosedNamedStreamLeavesAReplacementNothingToPlace(t *testing.T) {
	r := newTestRunner(t, &Runner{Stdin: closedFd{}, Stdout: closedFd{}, Stderr: openScratch(t)})
	files := r.replacementFiles()
	if files[0] != nil || files[1] != nil {
		t.Errorf("closed streams placed %v and %v, want nothing on either number", files[0], files[1])
	}
	if files[2] == nil {
		t.Error("the stream that was still open placed nothing")
	}
}

// The mark that keeps a coprocess's near ends out of the table a command
// inherits stops at the table: it guards descriptors 3 and up, and a
// coprocess put on *standard output* with `exec >&${C[1]}` is a redirected
// standard stream like any other. Measured on the panel, where the command a
// replacement runs writes into the coprocess through descriptor 1.
func TestACoprocessOnANamedStreamStillCrossesToAReplacement(t *testing.T) {
	f := openScratch(t)
	r := newTestRunner(t, &Runner{Stdout: shellOwnedFd{f}})
	r.setFd(3, shellOwnedFd{openScratch(t)})

	files := r.replacementFiles()
	if files[1] != f {
		t.Errorf("descriptor 1 = %v, want the file behind the coprocess's feed", files[1])
	}
	// And the mark still holds at 3, where the table proper begins. What
	// changed with #1917 is how that is *said*: the number is reached and
	// left nil, which is this file's own rule that a nil is a number which
	// must not be open there. It used to be said by not reaching the number
	// at all, and a number nothing reaches is not closed — it is whatever
	// this process happens to have on it, which is the hole that issue was.
	if len(files) <= firstExtraFd {
		t.Fatalf("the table stops at %d, so descriptor %d is never closed in the replacement",
			len(files)-1, firstExtraFd)
	}
	if files[firstExtraFd] != nil {
		t.Errorf("descriptor %d = %v, want nothing: the coprocess's own end does not cross",
			firstExtraFd, files[firstExtraFd])
	}
}

// openScratch is a file of this test's own, closed when it ends.
func openScratch(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
