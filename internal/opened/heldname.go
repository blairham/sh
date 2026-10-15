// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import "os"

// A held directory can outlive every name it had, and a shell's relative
// paths still have to be resolved from it.
//
// interp keeps its working directory as a descriptor rather than as the
// process's cwd (see interp/helddirectory.go), and asks the kernel what that
// descriptor is called whenever a relative path is about to reach the
// operating system. Once the directory has been deleted there is no answer —
// and if another directory has since been made under the old name, the old
// name leads somewhere else. A process whose cwd is the deleted directory
// resolves a relative path from the deleted directory regardless, so every
// real shell globs it, redirects into it and starts its children in it; a
// name-based answer used the *new* directory instead (#6532).
//
// HeldName is the bridge. It answers with a path string that leads to the
// directory the descriptor holds rather than to whatever a name leads to, so
// that the string-shaped routes — a glob's walk, a redirection's open, a
// child's working directory — reach the held directory without each of them
// learning about descriptors:
//
//   - On Linux it is `/proc/self/fd/N`, which the kernel itself resolves to
//     the object the descriptor holds, deleted or not: every system call that
//     takes a path follows it, and so does the chdir a child makes before it
//     execs (the child has inherited descriptor N at that point; close-on-exec
//     only takes it at the exec).
//
//   - On Darwin `/dev/fd/N` names the descriptor but cannot be walked through
//     — `/dev/fd/N/f` is ENOTDIR and a chdir to it is refused — so the string
//     is recognized here instead, by AtHeld, and the operation is made from a
//     thread whose own working directory has been set to the descriptor
//     (pthread_fchdir). That is the same object a process cwd would be, with
//     the same permission check at every lookup, and a child forked from that
//     thread inherits it, which is what HeldStart relies on.
//
// Everywhere else HeldName answers "" and callers use the names they had.

// HeldName is the path that leads to the directory open on fd, for a caller
// that has no name for it, or "" on a platform with none. On Darwin it
// registers fd so that AtHeld recognizes the answer; ForgetHeld undoes that
// and must be called before fd is closed.
func HeldName(fd int) string { return heldName(fd) }

// ForgetHeld withdraws a HeldName registration. Harmless for an fd that was
// never registered.
func ForgetHeld(fd int) { forgetHeld(fd) }

// AtHeld runs do on path, resolved from the held directory where path is one
// HeldName produced and as written everywhere else.
//
// do is the os function the caller would have called — os.Stat, os.OpenFile
// in a closure, os.ReadFile — and it is called with a name relative to the
// held directory on a thread whose working directory *is* that directory, so
// what comes back is exactly what that function returns for a process
// sitting there: the same FileInfo type (os.SameFile reads it), the same
// errors, the same close-on-exec descriptor.
func AtHeld[T any](path string, do func(name string) (T, error)) (T, error) {
	fd, rest, ok := splitHeld(path)
	if !ok {
		return do(path)
	}
	var (
		v   T
		err error
	)
	if ferr := fromHeldThread(fd, func() { v, err = do(rest) }); ferr != nil {
		var zero T
		return zero, ferr
	}
	return v, err
}

// HeldStart runs start — an exec.Cmd's Start, or whatever wraps it — so that
// the child begins in the held directory where dir is a HeldName, rather than
// changing into a name. dir is the Cmd's Dir and path its Path; HeldStart
// clears the one and makes the other relative for the length of the call
// through setDirPath, and restores both afterwards.
//
// Where dir is not a held name this is start() and nothing else.
func HeldStart(dir, path string, setDirPath func(dir, path string), start func() error) error {
	fd, rest, ok := splitHeld(dir)
	if !ok || rest != "." {
		return start()
	}
	run := path
	if pfd, prest, held := splitHeld(path); held && pfd == fd {
		// The program named from the same held directory: relative now, so
		// the child resolves it from the directory it was started in.
		run = "./" + prest
	}
	var err error
	if ferr := fromHeldThread(fd, func() {
		setDirPath("", run)
		err = start()
		setDirPath(dir, path)
	}); ferr != nil {
		return ferr
	}
	return err
}

// HeldChdir is os.Chdir, moving the *process* into the held directory where
// path is a HeldName. It is for a program about to replace itself with
// another (driver's process replacement) — never for a library caller, for
// whom the process's directory is not its own to move.
func HeldChdir(path string) error {
	if fd, rest, ok := splitHeld(path); ok && rest == "." {
		return heldChdir(fd)
	}
	return os.Chdir(path)
}
