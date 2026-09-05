// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package pty

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

// The BSD sequence: open the multiplexer, grant and unlock the pair, then ask
// it for the name of the other end. Documented as `posix_openpt`, `grantpt`,
// `unlockpt` and `ptsname` in the manual; those are libc wrappers over these
// three ioctls, and this module has no cgo to call them with.
func open() (*os.File, *os.File, error) {
	c, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := c.Fd()
	if err := ioctlNone(fd, syscall.TIOCPTYGRANT); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	if err := ioctlNone(fd, syscall.TIOCPTYUNLK); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	// Long enough for any device path this can name; the answer is
	// NUL-terminated.
	var name [128]byte
	if err := ioctlName(fd, syscall.TIOCPTYGNAME, &name); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	if i := bytes.IndexByte(name[:], 0); i >= 0 {
		name[i] = 0
		t, err := os.OpenFile(string(name[:i]), os.O_RDWR|syscall.O_NOCTTY, 0)
		if err != nil {
			_ = c.Close()
			return nil, nil, err
		}
		return c, t, nil
	}
	_ = c.Close()
	return nil, nil, ErrUnsupported
}

func ioctlNone(fd, req uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, 0); errno != 0 {
		return errno
	}
	return nil
}

func ioctlName(fd, req uintptr, name *[128]byte) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(name)))
	if errno != 0 {
		return errno
	}
	return nil
}
