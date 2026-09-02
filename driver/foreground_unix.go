// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import (
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// foreground hands the terminal to a process group.
//
// This is what makes ^C and ^Z reach the command. The kernel sends them to the
// terminal's *foreground* group, so a command running in a group of its own
// that was never given the terminal cannot be interrupted or stopped — which
// is not a smaller version of job control but a worse one: before this, ^Z on
// a foreground command reached only the shell, the command carried on, and the
// wait had nothing to report.
//
// A pgid of 0 means the shell itself, which is how the terminal comes back.
func foreground(pgid int) error {
	if pgid == 0 {
		pgid = syscall.Getpgrp()
	}
	tty, err := controllingTerminal()
	if err != nil {
		return err
	}
	defer func() { _ = tty.Close() }()

	// SIGTTOU has to be ignored around this, and the reason is a trap worth
	// stating: a process that is *not* in the foreground group and calls
	// tcsetpgrp is sent SIGTTOU, whose default action stops it. So the very
	// call that takes the terminal back would stop the shell that made it.
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)

	return tcsetpgrp(int(tty.Fd()), pgid)
}

// controllingTerminal opens the terminal this shell is attached to.
//
// /dev/tty rather than stdin: a shell whose input is redirected still has a
// controlling terminal, and it is the one job control is about.
func controllingTerminal() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func tcsetpgrp(fd, pgid int) error {
	p := pgid
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd),
		uintptr(syscall.TIOCSPGRP), uintptr(unsafe.Pointer(&p)))
	if errno != 0 {
		return errno
	}
	return nil
}
