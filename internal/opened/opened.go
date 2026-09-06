// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package opened answers the one question a name-based gate cannot answer for
// itself: what did the kernel actually give me?
//
// A rule matching names is defeated by a symbolic link, and the reason is not
// that the matching is weak — it is that a name and an object are different
// things, and only the kernel knows which object a name reached. `deny read
// /etc/**` did not stop `cat < link` where the link pointed into /etc, because
// nothing in the decision ever looked past the name the script wrote.
//
// So this asks the kernel, and it asks *after* the open, about the descriptor
// already in hand. That ordering is the whole design, and it is what makes
// the check honest where resolving before the open would not have been:
//
//   - Nothing here resolves a path. There is no lstat loop, no readlink walk,
//     no second traversal of the name. The kernel did the traversal once, as
//     part of the open the caller asked for, and this reads back the answer
//     it arrived at. The gate performs no filesystem reads of its own outside
//     the boundary it is enforcing, which was the objection to resolving
//     first.
//
//   - There is no time-of-check-to-time-of-use window. A descriptor pins an
//     object. Replacing the link afterwards changes which object the *name*
//     reaches and cannot change which object this descriptor holds, so what
//     was checked and what is used are the same thing — which resolve-then-
//     open could never promise, because between its resolution and its open
//     the name is free to mean something else.
//
// # What this is not
//
// It does not make a gate an inode policy. The answer is still a name — the
// kernel's own name for the object rather than the caller's — so two names
// that are both real names for one object are still two answers: a hard link
// and a bind mount are out of scope by construction, and deliberately, since
// closing them means matching on identity rather than on paths and that is
// the OS backend docs/design/sandboxing.md points at. Nor does it reach a
// child process: an allowed exec makes its own system calls and nothing here
// sees them.
//
// # Why this is a package and not two functions in interp
//
// Because there are two enforcers, not one. interp holds the boundary for
// everything a *script* does; internal/boundary holds it for the opens a
// *front end* makes on paths the policy's subject chose — the script operand,
// HISTFILE, the block store, the files an agent asks for over ACP. Both need
// the same three things, and all three are the kind that must not exist twice:
// a syscall wrapper that differs per platform, a table of the links an
// operating system ships, and the held-back truncation, which is a security
// property rather than a convenience. Two copies of those would drift, and
// the copy that drifted would be the one nobody was reading.
//
// So it lives under internal/, which is a visibility rule and not a layer:
// this is not part of the substrate's public surface and never should be. An
// embedder writing a Gate never holds a descriptor — the verification is
// performed for them and their gate is simply consulted a second time — so
// exporting an fcntl wrapper from interp would have widened the public API
// permanently for a need no embedder has. The standard library's own os
// package imports internal/poll for the same reason.
//
// # Where the cost falls
//
// A run with no gate does not enter this at all, and both callers are
// arranged so that such a run opens files by exactly the calls they did
// before. A run with a gate pays one fcntl per open and, in the overwhelmingly
// common case where the name was already the object's own name, one string
// comparison and no second consultation.
package opened

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Path is the kernel's name for the object behind an open descriptor, and
// whether there is one at all.
//
// The false return is not a failure to be feared: it means the object has no
// name in the filesystem — a pipe, a socket, a device the kernel does not
// name that way — and an object with no name cannot be the subject of a rule
// about names, so there is nothing for a verification to find. It is also
// what a platform with no way to ask returns.
//
// Asked through SyscallConn rather than through File.Fd, because Fd is not a
// read: it takes the descriptor out of the runtime's poller and leaves it in
// blocking mode for good. A shell opens fifos — a process substitution is one
// — and quietly changing how one of those behaves in order to inspect it
// would be a gate that broke the thing it was watching.
func Path(f *os.File) (string, bool) {
	conn, err := f.SyscallConn()
	if err != nil {
		return "", false
	}
	var (
		path string
		ok   bool
	)
	if err := conn.Control(func(fd uintptr) { path, ok = pathOfFd(fd) }); err != nil {
		return "", false
	}
	return path, ok
}

// Elsewhere is the kernel's name for what a descriptor holds, when that is a
// different place from the name the caller asked about.
//
// The false return is the ordinary case and covers three situations a caller
// treats identically, because in all three the decision already made is the
// decision about this object: the object has no name, the name the caller
// wrote is the kernel's own name for it, or the two are the operating
// system's own two names for one place.
//
// The requested name is cleaned here rather than at each call site, so that
// `dir//file` and `dir/./file` are not reported as having resolved somewhere
// else — a caller that forgot would have handed its gate a second consultation
// for every path a person typed carelessly.
func Elsewhere(f *os.File, requested string) (string, bool) {
	actual, ok := Path(f)
	if !ok {
		return "", false
	}
	if clean := filepath.Clean(requested); actual == clean || samePlace(clean, actual) {
		return "", false
	}
	return actual, true
}

// Verified opens a file the way a gate needs it opened: with the truncation
// held back until check has passed on the descriptor.
//
// The truncation is the reason this is a function rather than two lines at
// each call site. O_TRUNC is the one flag that destroys before anything can be
// checked: the kernel empties the file as part of the open, so an open that
// is then refused has already done the damage the refusal was for, and
// `> link` pointed at a denied file would leave it empty and report a
// refusal. So the flag is held back, the object is verified, and only then is
// the file emptied — which is the same sequence O_TRUNC is, split at the point
// where a decision can be made.
//
// check is what the caller's gate says about the object the descriptor holds.
// It is called with the file open and before a byte has been written to it or
// removed from it; the error it returns is returned here, and the file is
// closed.
//
// Only regular files are emptied, and that guard is measured rather than
// cautious. On Linux `open("/dev/null", O_WRONLY|O_TRUNC)` succeeds and
// `ftruncate` on the same descriptor answers EINVAL — as it does on a FIFO —
// so splitting the flag off without the guard would break `> /dev/null`,
// which is about the most common thing a script does. Darwin happens to
// accept both, which is exactly why the guard cannot be justified by trying
// it on one machine.
//
// A truncation that does fail is returned as the open's own error, which is
// what it would have been: `> some-running-binary` reports text-file-busy
// either way.
func Verified(path string, flags int, perm fs.FileMode, check func(*os.File) error) (*os.File, error) {
	f, err := os.OpenFile(path, flags&^os.O_TRUNC, perm)
	if err != nil {
		return nil, err
	}
	if err := check(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	if flags&os.O_TRUNC != 0 {
		if err := emptyRegular(f); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return f, nil
}

// emptyRegular is the held-back half of O_TRUNC.
func emptyRegular(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return f.Truncate(0)
}

// samePlace reports whether two absolute paths name one place under the two
// names the operating system itself installs for it.
//
// One direction only, and that is exact rather than lazy: the kernel answers
// with the physical spelling, so the case to recognize is a caller naming the
// logical one. A caller that names the physical one gets it back unchanged
// and never reaches here.
//
// Whole components, so `/tmpfoo` is not under `/tmp`. And the *whole
// remainder* has to match, which is what keeps this from being a hole: a link
// anywhere else along the path changes what follows the prefix, so
// `/tmp/link` reaching `/private/tmp/elsewhere` is a difference this does not
// absorb and the gate is asked about it.
func samePlace(requested, actual string) bool {
	for _, link := range platformLinks {
		if rest, ok := beneath(requested, link[0]); ok && link[1]+rest == actual {
			return true
		}
	}
	return false
}

// beneath reports whether path is prefix itself or lies under it, and returns
// what follows.
func beneath(path, prefix string) (rest string, ok bool) {
	if path == prefix {
		return "", true
	}
	if strings.HasPrefix(path, prefix+"/") {
		return path[len(prefix):], true
	}
	return "", false
}
