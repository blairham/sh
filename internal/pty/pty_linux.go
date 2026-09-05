// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pty

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// The Linux sequence: open the multiplexer, clear the lock, ask for the pair's
// number, and the other end is /dev/pts/N. `unlockpt` and `ptsname` in the
// manual are libc wrappers over these two ioctls, and this module has no cgo
// to call them with.
func open() (*os.File, *os.File, error) {
	c, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := c.Fd()
	var unlock int32 // zero means unlocked
	if err := ioctlInt32(fd, syscall.TIOCSPTLCK, &unlock); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	var n uint32
	if err := ioctlUint32(fd, syscall.TIOCGPTN, &n); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	t, err := os.OpenFile("/dev/pts/"+strconv.FormatUint(uint64(n), 10), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, t, nil
}

func ioctlInt32(fd, req uintptr, v *int32) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(v)))
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlUint32(fd, req uintptr, v *uint32) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(v)))
	if errno != 0 {
		return errno
	}
	return nil
}
