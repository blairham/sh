// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"strconv"
	"sync"
	"syscall"
)

// A substitution's pipe, and the descriptor number its path is made of.
//
// `<(cmd)` and `>(cmd)` expand to a path, and what the path has to be is the
// only thing a shell here has any choice about. Every shell in the panel that
// has the construct hands the command `/dev/fd/N` — measured 2026-09-15, bash
// 5.3, ksh93u+ and zsh 5.9.2 all do, on all three invocation routes — and this
// one handed over a named pipe in a directory of its own under `$TMPDIR`
// instead (#2893). Both are pipes and both read correctly, so what differed
// was the name: a path that leaks the temporary directory into anything that
// echoes its arguments, does not repeat between runs, exists in the filesystem
// for something to have to remove, and answers a second `open` differently.
//
// # Why /dev/fd works here and did not before
//
// The rejection this replaces was real and is worth stating exactly, because
// the reasoning was right about the mechanism it had in mind. A `/dev/fd/N`
// path is only openable by a process that holds N, so the descriptor has to
// reach the command — and everything Go opens is close-on-exec, so the way to
// make it reach anything looked like clearing that flag. Clearing it hands the
// descriptor to **every** command the shell runs afterwards, and for `>(cmd)`
// that is fatal rather than untidy: a later command holding the writing end
// open means the body never reads end-of-file, so
//
//	echo x | tee >(tr a-z A-Z); sleep 0.4
//
// produced nothing at all, because `sleep` was holding the pipe. A C shell
// clears the flag around one fork and sets it again; Go's os/exec takes the
// fork lock itself, so there is no window a caller can hold.
//
// The flag never had to be cleared. A descriptor reaches a child **by number**
// through the table childFiles rebuilds — that is what `exec 3>out3; cmd`
// already runs on, and it places a file on a chosen number in that child and
// in no other. So the end the command opens is parked on a number here, stays
// close-on-exec, and is put into that table for the commands of the one shell
// that named it. Nothing else inherits it, which is stronger than the C shells
// manage: a substitution's own body, and every command the body runs, is a
// clone whose procSubs list is empty, so `tee >(cat)` cannot hand the writing
// end to the `cat` that is reading the other side of it.
//
// # And it is a pipe rather than a FIFO
//
// Everything a named pipe needed and an anonymous one does not goes with it.
// A FIFO's two ends have to *meet*: opening one blocks until the other is
// opened, which for a path the command may never open at all is a wait that
// may never end, and the pipe itself exists only while somebody holds it —
// so a shell that opened, wrote and closed inside the window a reader was
// still in `open(2)` ran a whole pipe's life cycle beside a reader attached
// to none of it (#2733), and a last-writer close could be delivered to
// nobody (#1079). An `os.Pipe` has both ends from the moment it is made and
// cannot be torn down while either is held, so there is no rendezvous, no
// bounded poll for a peer, no placeholder end to keep the pipe alive and no
// close to repeat.
//
// What replaces all of it is one descriptor: the end the command opens, held
// by the shell from the moment the word expands until removeProcSubs, which
// is the same lifetime the FIFO's name had. It keeps the pipe alive while the
// command may still open the path, and closing it is what delivers
// end-of-file to a `>(cmd)`'s body and EPIPE to a `<(cmd)`'s.

// procSubEnds is the two halves of one substitution's pipe.
//
// shell is the end this shell reads or writes through — the body's output for
// `<(cmd)`, the body's input for `>(cmd)`. child is the other end, parked on a
// descriptor number, which is the whole of what path names.
type procSubEnds struct {
	shell *os.File
	child *os.File
	path  string
}

// firstProcSubFd is where the search for a number to park a substitution's end
// on begins.
//
// Above the nine a script names by hand, which is not tidiness: the number is
// where the table childFiles builds will place the descriptor in the command,
// and a script's own `exec 3>out` is an entry in that same table. Two things
// on one number is one of them lost, and which one would depend on map
// iteration order.
const firstProcSubFd = 10

// devFdDir and procFdDir are the two directories a substitution's path can be
// named after. Both are the process's own descriptor table under another
// name, and where both exist the first is a symlink to the second.
const (
	devFdDir  = "/dev/fd"
	procFdDir = "/proc/self/fd"
)

// procFdDirExists is whether /proc/self/fd is there on this machine.
//
// Asked once: it is a property of the kernel the process is running on, and a
// shell that stat'd it per substitution would be asking the same question
// thousands of times to get the same answer. A read of the filesystem rather
// than a build tag, because a Linux kernel without /proc mounted is a real
// configuration and GOOS cannot see it.
var procFdDirExists = sync.OnceValue(func() bool {
	info, err := os.Stat(procFdDir)
	return err == nil && info.IsDir()
})

// procSubFdDir is the directory this shell names its substitutions after.
//
// Semantics.SubstitutionPathPrefersProcSelfFd carries the measurement and the
// reason the directory is looked for rather than assumed. /dev/fd is the
// fallback every preset takes where /proc is not, which is every preset on
// this machine's own platform.
func (r *Runner) procSubFdDir() string {
	if r.sem().SubstitutionPathPrefersProcSelfFd == Yes && procFdDirExists() {
		return procFdDir
	}
	return devFdDir
}

// newProcSubPipe makes a substitution's pipe and parks the end the command
// will open.
//
// childWrites says which way round the ends go, and it is the only difference
// between the two spellings: `>(cmd)` hands the command the writing end and
// keeps the reading one, `<(cmd)` the other way about. dir is the directory
// the path is named after — see Runner.procSubFdDir.
func newProcSubPipe(childWrites bool, dir string) (procSubEnds, error) {
	rd, wr, err := os.Pipe()
	if err != nil {
		return procSubEnds{}, err
	}
	shell, child := wr, rd
	if childWrites {
		shell, child = rd, wr
	}
	parked, err := parkDescriptor(child, dir)
	// The original is closed either way: on success the parked duplicate is
	// the one the path names, and on failure there is nothing to hand over.
	_ = child.Close()
	if err != nil {
		_ = shell.Close()
		return procSubEnds{}, err
	}
	return procSubEnds{shell: shell, child: parked, path: parked.Name()}, nil
}

// parkDescriptor duplicates a file onto the lowest free number at or above
// firstProcSubFd, and names it the way the path will.
//
// F_DUPFD_CLOEXEC rather than a dup and a flag, because the pair is not atomic
// and a fork on another goroutine between them is a descriptor leaked into a
// child — the same reason driver's own descriptor moves use it. Close-on-exec
// stays *on*: what puts this in a command is the table childFiles builds, by
// number, and a descriptor that also leaked through the kernel behind that
// table's back would be open in every command the shell runs.
//
// The name is the path the command is going to be handed — `/dev/fd/N`, or
// `/proc/self/fd/N` in the dialect and on the platform that prefer it — and
// *os.File carries its name for whoever asks, which is what lets
// holdsDescriptorOnto recognize a descriptor the script has since taken onto
// the same pipe. Both spellings are the same number in the same table, so the
// choice reaches nothing but the string.
//
// # And the blocking mode is put back
//
// Go opens a pipe non-blocking and runs it behind its own poller, which is a
// fact about *this* process and travels further than it looks: on darwin
// `/dev/fd/N` is a duplicate rather than a fresh open, so it shares the open
// file description and its flags. A command handed a path whose description
// is non-blocking meets EAGAIN on an empty pipe and reports it as an error —
// `cat <(…)` answered `Resource temporarily unavailable` and `read < <(…)`
// read nothing at status 0, which is the quieter half of the same fault.
//
// Cleared on the duplicate, which is the end nothing in this process reads or
// writes through: the shell's own end is the pipe's *other* description and
// keeps the mode Go gave it, so the poller is untouched.
func parkDescriptor(f *os.File, dir string) (*os.File, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}
	var parked int
	var parkErr error
	if cerr := conn.Control(func(fd uintptr) {
		parked, parkErr = fcntlInt(int(fd), syscall.F_DUPFD_CLOEXEC, firstProcSubFd)
	}); cerr != nil {
		return nil, cerr
	}
	if parkErr != nil {
		return nil, parkErr
	}
	if err := syscall.SetNonblock(parked, false); err != nil {
		_ = syscall.Close(parked)
		return nil, err
	}
	return os.NewFile(uintptr(parked), dir+"/"+strconv.Itoa(parked)), nil
}

// fcntlInt is the one fcntl this package needs, with the errno turned into an
// error.
func fcntlInt(fd, cmd, arg int) (int, error) {
	n, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}
