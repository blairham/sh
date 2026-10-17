// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package testenv

import (
	"os"
	"syscall"
	"testing"
)

// TestAnInheritedDescriptorIsClosedAndTheProcesssOwnAreNot pins both halves of
// the rule closeInheritedDescriptors rests on: a descriptor without
// close-on-exec is the host's and goes, and one with it is this process's and
// stays. The first half is what #6944 needed; the second is what keeps the
// guard from closing the runtime's poller or a file a test already holds.
//
// The inherited descriptor is made here rather than waited for — a
// descriptor whose flag was cleared is indistinguishable from one that came
// through exec, which is the whole of what the guard can see.
func TestAnInheritedDescriptorIsClosedAndTheProcesssOwnAreNot(t *testing.T) {
	inherited, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("opening the stand-in for an inherited descriptor: %v", err)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(inherited), uintptr(syscall.F_SETFD), 0); errno != 0 {
		_ = syscall.Close(inherited)
		t.Fatalf("clearing close-on-exec on %d: %v", inherited, errno)
	}
	own, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("opening a descriptor of this process's own: %v", err)
	}
	defer func() { _ = own.Close() }()

	if !isOpen(inherited) {
		t.Fatalf("the stand-in %d is not open before the guard runs, so the check below would pass on nothing", inherited)
	}
	closeInheritedDescriptors()

	if isOpen(inherited) {
		_ = syscall.Close(inherited)
		t.Errorf("descriptor %d has no close-on-exec, so it came from the host, and it is still open", inherited)
	}
	if !isOpen(int(own.Fd())) {
		t.Errorf("descriptor %d is close-on-exec, so it is this process's own, and the guard closed it", own.Fd())
	}
}

func isOpen(fd int) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFD), 0)
	return errno == 0
}
