// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import "syscall"

// signalGroup sends to a process group rather than to one process.
//
// The negative pid is how the kernel is told to mean the group, and the group
// is the point: a job is a pipeline as often as a command, so resuming only
// its first process would leave the rest stopped with nothing able to name
// them.
//
// In driver rather than interp for the reason kill's fatal path is: a signal
// that leaves this process is the binary's to send. A Runner embedded in
// another program must not be able to signal that program's children because
// a line of script said so.
//
// The test that holds this only fails on one of the two platforms, which is
// worth knowing rather than assuming. Killing a group's leader leaves its
// other members running, and asking whether anything is left means signaling
// the group and reading the error — but a *zombie* is still a member, and a
// group holding only zombies answers EPERM on a BSD and success on Linux. So
// the same test discriminates on Linux and cannot on macOS, where the
// leader-only bug and the correct code give the same answer.
func signalGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}
