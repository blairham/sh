// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

// An in-package test, unlike the rest of this directory's: the scan is not
// exported and must not be — a program embedding a Runner has no business
// being handed a way to publish its own descriptors — and what is being
// checked is the discriminator itself rather than anything a caller can see.
package driver

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A descriptor the process was handed and one it opened for itself are told
// apart by close-on-exec and by nothing else. The Go runtime opens everything
// close-on-exec — that is the reason the outbound table has to be rebuilt by
// hand at all — so a descriptor that *would* survive an exec is one this
// program did not open, and it is the only kind a script may be shown.
//
// dup(2) is what makes the case testable without a second process: it never
// sets the flag, so a duplicate of a file this test opened is
// indistinguishable from a descriptor a caller left open, which is exactly
// the property being relied on.
func TestOnlyADescriptorThatWouldSurviveAnExecIsPublished(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held")
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	own, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = own.Close() }()
	ownFd := int(own.Fd())
	if !closeOnExec(ownFd) {
		t.Fatalf("a file this program opened should be close-on-exec, fd %d is not", ownFd)
	}

	handed, err := syscall.Dup(ownFd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Close(handed) }()
	if closeOnExec(handed) {
		t.Fatalf("a plain dup should not be close-on-exec, fd %d is", handed)
	}

	files := scanInheritedFiles()
	defer func() {
		for _, f := range files {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	// Entry i is descriptor 3+i, so a descriptor keeps its number rather than
	// being packed down to the next free slot.
	if i := handed - firstExtraFd; i >= len(files) || files[i] == nil {
		t.Fatalf("descriptor %d was not published; %d entries", handed, len(files))
	}
	if i := ownFd - firstExtraFd; i < len(files) && files[i] != nil {
		t.Errorf("descriptor %d is this program's own and should not have been published", ownFd)
	}

	copied := files[handed-firstExtraFd]
	// A copy, not the descriptor itself: the original belongs to whoever
	// opened it, and a table this shell closes entries from must not be able
	// to take it away from them.
	if int(copied.Fd()) == handed {
		t.Errorf("the published file is descriptor %d itself rather than a copy", handed)
	}
	// The copy is close-on-exec where the original was not. A child is given
	// the table by number through os/exec, so nothing has to leak through an
	// exec to reach one.
	if !closeOnExec(int(copied.Fd())) {
		t.Errorf("the copy of descriptor %d should be close-on-exec", handed)
	}
	// And the original is left open, which is what keeps `exec cmd` — the
	// path that replaces this process rather than forking — handing the
	// descriptor to its replacement by number.
	if closeOnExec(handed) {
		t.Errorf("descriptor %d was closed or marked by the scan", handed)
	}

	buf := make([]byte, 5)
	if _, err := copied.ReadAt(buf, 0); err != nil {
		t.Fatalf("reading through the published copy: %v", err)
	}
	if string(buf) != "hello" {
		t.Errorf("the copy read %q, want %q", buf, "hello")
	}
}

// The listing is the fast path and it has to say when it did not answer, so
// the caller tries the next name rather than concluding that nothing is open.
// A directory that is not there is the plain case; an empty one is the case
// worth pinning, because a listing always contains at least the descriptor
// used to read it.
func TestAFdListingThatDidNotAnswerIsNotAnEmptyTable(t *testing.T) {
	if _, ok := readFdDir(filepath.Join(t.TempDir(), "absent")); ok {
		t.Error("a directory that is not there should not have answered")
	}
	if _, ok := readFdDir(t.TempDir()); ok {
		t.Error("an empty directory should not have answered")
	}
	fds, ok := readFdDir("/dev/fd")
	if !ok {
		// Neither /proc/self/fd nor /dev/fd on this platform: the bounded
		// probe is the fallback, and it is what the scan would use.
		t.Skip("no descriptor directory on this platform")
	}
	if len(fds) == 0 {
		t.Error("the listing answered with nothing, which cannot include its own descriptor")
	}
}
