// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package tty

import (
	"os"
	"syscall"
	"unsafe"
)

// modeState is what a terminal's line discipline is, on a platform that has
// one. See mode.go for what this package does with it.
type modeState = syscall.Termios

// **These take the descriptor with Fd and not with SyscallConn, and that is
// load-bearing rather than the older spelling left alone.**
//
// IsTerminal in this package deliberately uses SyscallConn, because it holds a
// reference for the length of the call and a Close racing it cannot free the
// number. Changing *the mode* the same way is a different matter: `Fd` also
// detaches the file from the runtime's poller and leaves it blocking, and
// `repl`'s pseudo-terminal conduit depends on that — the terminal it hands a
// child is put in raw output here, and with the file still under the poller a
// close raced the pump and dropped the last block. Measured by making the
// change: repl's TestClosingWaitsForWhatIsStillInTheConduit fails with 261120
// of 262144 bytes delivered, reproducibly, three runs in three.
//
// So the asking is done the safe way and the changing is done the way the
// callers were built on. A future change here has to keep that or move the
// conduit off its assumption in the same breath.

// setMode reads the current discipline, keeps it, and writes back a changed
// copy. raw is the editor's mode; otherwise only the waiting is turned off.
func setMode(f *os.File, raw bool) (*Mode, error) {
	if f == nil {
		return nil, ErrUnsupported
	}
	fd := f.Fd()
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return nil, err
	}
	saved := t
	// One byte is enough to return from a read, and no timer: a caller blocks
	// until something is typed rather than spinning. Both modes want this —
	// it is the half `read -k` is asking for.
	t.Lflag &^= syscall.ICANON
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if raw {
		t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
			syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
		t.Oflag &^= syscall.OPOST
		t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ISIG | syscall.IEXTEN
		t.Cflag &^= syscall.CSIZE | syscall.PARENB
		t.Cflag |= syscall.CS8
	}
	if err := ioctl(fd, tcSets, &t); err != nil {
		return nil, err
	}
	return &Mode{f: f, saved: saved}, nil
}

// putMode writes a saved discipline back.
func putMode(f *os.File, saved modeState) error {
	return ioctl(f.Fd(), tcSets, &saved)
}

// postProcessesOutput answers TranslatesNewlines.
func postProcessesOutput(f *os.File) bool {
	if f == nil {
		return false
	}
	var t syscall.Termios
	if err := ioctl(f.Fd(), tcGets, &t); err != nil {
		return false
	}
	return t.Oflag&syscall.OPOST != 0
}

// clearOutputPostProcessing answers RawOutput.
func clearOutputPostProcessing(f *os.File) error {
	if f == nil {
		return ErrUnsupported
	}
	fd := f.Fd()
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return err
	}
	t.Oflag &^= syscall.OPOST
	return ioctl(fd, tcSets, &t)
}

func ioctl(fd uintptr, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, req,
		uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
