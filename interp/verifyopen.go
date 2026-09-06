// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The one question a name-based gate cannot answer for itself: what did the
// kernel actually give me?
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
//     part of the open the script asked for, and this reads back the answer
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
// It does not make the gate an inode policy. The answer is still a name — the
// kernel's own name for the object rather than the script's — so two names
// that are both real names for one object are still two answers: a hard link
// and a bind mount are out of scope by construction, and deliberately, since
// closing them means matching on identity rather than on paths and that is
// the OS backend the design document points at. Nor does it reach a child
// process: an allowed exec makes its own system calls and nothing here sees
// them.
//
// # Where the cost falls
//
// A run with no gate does not enter this at all, and the code it guards is
// arranged so that such a run opens files by exactly the calls it did before.
// A run with a gate pays one fcntl per open and, in the overwhelmingly common
// case where the name was already the object's own name, one string
// comparison and no second consultation.

// openedPath is the kernel's name for the object behind an open descriptor,
// and whether there is one at all.
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
func openedPath(f *os.File) (string, bool) {
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

// verifyOpened reports whether the object a just-opened descriptor holds is
// one the gate permits, having been asked about the name that reached it.
//
// The action it consults with carries the same ID as the one the caller
// already asked about, because it is the same access: a consumer joining the
// two records sees one open whose name resolved elsewhere, not two opens.
//
// A refusal is recorded here, with the resolved path on it, and reported by
// the caller, without it. That split is deliberate. The audit stream belongs
// to whoever wrote the policy and has to say what was actually reached, or
// the record cannot be acted on; the diagnostic goes to the script, which is
// the party the policy is about, and a refusal naming where a link pointed
// would hand it the one fact the rule exists to withhold. So every caller
// below reports the name as written, in the wording an ordinary refusal
// already uses — a script cannot tell the two apart, which is the same
// property a denied stat has for the same reason.
func (r *Runner) verifyOpened(ctx context.Context, a Action, f *os.File) bool {
	if r.Gate == nil {
		return true
	}
	actual, ok := openedPath(f)
	if !ok {
		// The object has no name in the filesystem, so no rule about names
		// can be speaking about it and there is nothing to find.
		return true
	}
	if requested := filepath.Clean(a.Path); actual == requested || samePlace(requested, actual) {
		// The name the script wrote is the kernel's own name for what it
		// reached, or the two are the operating system's own two names for
		// one place. Either way the decision already made is the decision
		// about this object.
		return true
	}
	a.Path = actual
	if r.Gate.Allow(ctx, a) == Deny {
		r.emit(ctx, Event{Kind: EventDenied, Action: a})
		return false
	}
	return true
}

// reportRefusal tells the script an action was refused, naming the path it
// wrote.
//
// Shared by allowed() and by the verification above so that the two produce
// the same sentence — the point being that they must, since a script that
// could tell "the name is denied" from "the name reached a denied object" has
// been told where the name went.
func (r *Runner) reportRefusal(a Action) {
	r.diagf("%s: refused: %s\n", a.Kind, a.Path)
}

// openGated opens a file the way a redirect needs it opened, and confirms
// what it reached before letting a byte through.
//
// The truncation is the reason this is a function rather than two lines at
// the call site. O_TRUNC is the one flag that destroys before anything can be
// checked: the kernel empties the file as part of the open, so an open that
// is then refused has already done the damage the refusal was for, and
// `> link` pointed at a denied file would leave it empty and report a
// refusal. So under a gate the flag is held back, the object is verified, and
// only then is the file emptied — which is the same sequence O_TRUNC is,
// split at the point where a decision can be made.
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
//
// A run with no gate takes the first line and nothing else, so its opens are
// the calls they always were, flags included.
func (r *Runner) openGated(ctx context.Context, a Action, path string, flags int) (*os.File, error) {
	if r.Gate == nil {
		return os.OpenFile(path, flags, 0o666)
	}
	f, err := os.OpenFile(path, flags&^os.O_TRUNC, 0o666)
	if err != nil {
		return nil, err
	}
	if !r.verifyOpened(ctx, a, f) {
		_ = f.Close()
		return nil, errRefused
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

// readFileGated is os.ReadFile with the same verification, for `.`.
//
// Split from openGated rather than layered on it because the two answer to
// different callers: a redirect hands the descriptor to a command and needs
// the flags, and `.` wants the bytes and closes the file itself.
func (r *Runner) readFileGated(ctx context.Context, a Action, path string) ([]byte, error) {
	if r.Gate == nil {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if !r.verifyOpened(ctx, a, f) {
		return nil, errRefused
	}
	return io.ReadAll(f)
}

// samePlace reports whether two absolute paths name one place under the two
// names the operating system itself installs for it.
//
// One direction only, and that is exact rather than lazy: the kernel answers
// with the physical spelling, so the case to recognize is a script naming the
// logical one. A script that names the physical one gets it back unchanged
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
