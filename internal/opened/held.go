// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import "os"

// Hold opens a directory in order to keep hold of **the directory**, rather
// than of the name it currently answers to.
//
// It is the same open the walk makes on a directory it passes through, and it
// is here under a name of its own because the caller wants the opposite of
// what the walk wants. The walk keeps a descriptor so that a rename cannot
// move the place it is about to look; this keeps one so that a rename *can*
// be seen, by asking Path afterwards what the directory is called now. Path's
// comment records that a rename moves its answer for a descriptor that has not
// moved, on both platforms, and lists it among the reasons a gate must not
// trust it. That property is a hazard for a rule about names and it is the
// whole mechanism here.
//
// The traverse flags rather than a readable open, for the reason
// walkat_darwin.go gives: a directory that is traversable and not readable —
// 0111, an ordinary way to publish one file out of a private tree — answers
// EACCES to O_RDONLY|O_DIRECTORY and is walked straight through by the kernel.
// A shell can be sitting in one, and a hold that refused it would take the
// answer away exactly where the shell still has one.
func Hold(path string) (*os.File, error) {
	return os.OpenFile(path, traverseFlags(), 0)
}

// Within opens a directory named relative to a directory already held, so that
// the place the name starts from is the one in hand and not the one its own
// name currently leads to.
//
// This is the half Hold cannot do with a string. Once a directory's name has
// stopped leading to it, `filepath.Join` has nothing to join to: the only way
// left to say "the directory beside this one" is to say it to the kernel with
// the descriptor as the starting point. A directory whose last link has gone
// still answers here, which is the case no name can reach at all.
//
// The descriptor is taken through SyscallConn rather than through File.Fd for
// Path's reason: Fd is not a read — it takes the descriptor out of the
// runtime's poller and leaves it blocking for good.
//
// On a platform with no openat this returns the walk's own "unsupported",
// exactly as the walk does, so a caller falls back to whatever it does with a
// name and nothing here guesses a system call number.
func Within(dir *os.File, name string) (*os.File, error) {
	conn, err := dir.SyscallConn()
	if err != nil {
		return nil, err
	}
	var (
		fd      int
		openErr error
	)
	if err := conn.Control(func(dirfd uintptr) {
		fd, openErr = openat(int(dirfd), name, traverseFlags(), 0)
	}); err != nil {
		return nil, err
	}
	if openErr != nil {
		return nil, openErr
	}
	return os.NewFile(uintptr(fd), name), nil
}
