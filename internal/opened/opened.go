// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package opened answers the one question a name-based gate cannot answer for
// itself: what path did this open actually go through?
//
// A rule matching names is defeated by a symbolic link, and the reason is not
// that the matching is weak — it is that a name and an object are different
// things, and the name the script wrote may not be the name the access went
// by. `deny read /etc/**` did not stop `cat < link` where the link pointed
// into /etc, because nothing in the decision ever looked past the name the
// script wrote.
//
// Two arrangements were tried and rejected before the one that is here, and
// both rejections are the design:
//
//   - **Resolve the name, then open it.** A time-of-check-to-time-of-use race
//     in its classic form: between the resolution and the open the name is
//     free to mean something else, so the check and the use are about two
//     different objects.
//
//   - **Open, then read the answer back off the descriptor.** This was the
//     arrangement for two releases, with F_GETPATH on Darwin and
//     /proc/self/fd on Linux, on the premise that the kernel answers with
//     whichever name the descriptor was opened by. A descriptor does pin an
//     object — that half held, and replacing a link afterwards cannot change
//     which object is in hand. But it does not pin a *name*, and a rule
//     matches names. #1029 found the first hole and #1050 measured the
//     second: on Darwin the vnode carries one name for an object however many
//     the filesystem holds and any lookup re-stamps it, and on **both**
//     platforms a rename moves the answer for a descriptor that has not
//     moved. Measured, an ordinary allowed open carried a denied object past
//     the gate at about a third of attempts. The two tests named at the
//     bottom of this comment keep those measurements running.
//
// So the path is walked here, and the open is the last step of the walk. Each
// component is opened O_NOFOLLOW, each symbolic link is read explicitly, a
// descriptor is kept on every directory passed through so that `..` is a pop
// rather than a lookup, and the final openat is made on a directory
// descriptor held since that directory was checked. The name reported is
// assembled from the components traversed rather than looked up anywhere, so:
//
//   - Nothing can make it report a path the walk did not take. A rename or a
//     second hard link changes what a *name* leads to and cannot change a
//     sequence of components already walked.
//
//   - The object opened is the object at the path reported, because the last
//     openat is relative to a descriptor that no rename can move.
//
// resolve.go has the walk, the three details that are the actual work — which
// flags a directory is opened with, the two errnos a symbolic link answers
// with, the platform's own link budget — and the one thing a userspace walk
// cannot follow.
//
// # What this is not
//
// It does not make a gate an inode policy. The answer is still a name — the
// path the open went through rather than the one the caller wrote — so two
// names that are both real names for one object are still two answers: a hard
// link and a bind mount are out of scope by construction, and deliberately,
// since closing them means matching on identity rather than on paths, and a
// rule is a glob over a namespace where an identity has no patterns. That is
// the OS backend docs/design/sandboxing.md points at.
//
// That scope is now held by construction on both platforms, which is what
// #1114 changed. It was held by luck before, and TestTwoNamesForOneObject
// AndWhetherTheAnswerHolds and TestARenameMovesTheAnswer
// ForADescriptorThatHasNotMoved are why: they assert that the *platform* answer still moves, in the
// direction each platform moves it. Nothing in the check depends on that any
// more — Path has one caller left, the fail-closed fallback in Open — but a
// kernel that stopped behaving this way is the moment to revisit the design
// page rather than the moment to discover the claim had quietly changed.
//
// Nor does it reach a child process: an allowed exec makes its own system
// calls and nothing here sees them.
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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Path is the kernel's name for the object behind an open descriptor, and
// whether there is one at all.
//
// *A* name, not *the* name, and *now*, not for as long as the descriptor
// lives. The distinction is a platform's rather than this function's, and it
// is the reason the verification does not use this: the walk in resolve.go
// does. What is left for Path is Open's fallback, where the question being
// asked is not "what is this called" but "does this have a name at all", and
// for that the answer moving does not matter.
//
// It has two parts.
//
// An object with several names has one answer on Linux — the name the
// descriptor was opened through, which /proc holds per descriptor — and on
// Darwin any of them, because F_GETPATH reads a single name off the vnode and
// any lookup re-stamps it. So on Darwin this can answer differently for a
// descriptor that has not moved, and a caller comparing the answer to a name
// must not read a difference as "the name went elsewhere" for an object it
// knows to have more than one.
//
// An object with *one* name is not safe either, and that part is neither
// Darwin's nor about links: a rename moves the answer on both platforms,
// because Linux's dentry is moved by it and Darwin's vnode name is re-stamped
// by it. A descriptor pins an object, never a name.
//
// Measured every way in the two tests named above.
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

// Open opens a path and reports what it opened, by resolving the path itself
// rather than by asking afterwards.
//
// The walk in resolve.go is the whole of it on Darwin and Linux. Elsewhere
// there is no walk and no way to ask, so this is the ordinary open with no name
// attached — which is what the gate has always had on those platforms, and is
// the limitation docs/design/sandboxing.md records rather than a new one.
//
// The fallback is the interesting part and it fails closed. A walk that has
// followed a symbolic link and then failed may have met the one thing a
// userspace resolution cannot follow: a magic link, of the sort /proc/<pid>/fd
// holds, whose target is not a path at all — `pipe:[12345]`. The kernel
// resolves those and the walk cannot, so `cat < /dev/fd/3` would stop working
// under a policy while working without one, which is a policy breaking a
// mechanism rather than refusing an access.
//
// So the ordinary open is tried, and then the platform is *asked* whether the
// object it got has a name. If it has none, this is genuinely the nameless case
// — no rule about names can be speaking about it, which is what Elsewhere has
// always done with it. If it has one, the walk and the platform disagree about
// a path that does have a name, and rather than pick a winner this returns the
// walk's error: a disagreement here is either a bug in the walk or a filesystem
// doing something neither of us understands, and neither is a thing to open a
// file on.
func Open(path string, flags int, perm fs.FileMode) (Reached, error) {
	return openAsking(path, flags, perm, nil)
}

// openAsking is Open with the question the walk puts before a creation.
//
// ask is consulted only where the open would make a file that is not there,
// and only on the walked platforms; it is nil for a caller that is not gating,
// which is every caller of Open itself. See walk.createChecked for why the
// question belongs inside the walk and not around it.
func openAsking(path string, flags int, perm fs.FileMode, ask func(string) error) (Reached, error) {
	if !walkSupported {
		f, err := os.OpenFile(path, flags, perm)
		if err != nil {
			return Reached{}, err
		}
		return Reached{File: f}, nil
	}
	r, err := walkOpen(path, flags, perm, ask)
	if err == nil || !errors.Is(err, errFollowedALink) {
		return r, err
	}
	// The fallback does not create. It is here for the one thing a userspace
	// walk cannot follow — a magic link, whose target is not a path — and a
	// magic link is always already there, so O_CREAT has nothing to do on this
	// route but the one thing this file now holds back everywhere else: make a
	// file that no check was asked about. Dropping it turns the only case it
	// could reach, a name that is simply absent, back into the walk's own
	// ENOENT, which is the answer the caller would have had anyway.
	if ask != nil {
		flags &^= os.O_CREATE
	}
	f, plainErr := os.OpenFile(path, flags, perm)
	if plainErr != nil {
		// The walk and the kernel agree that this does not open. The walk's
		// error is the one to report, because it is the one built from the
		// caller's own path.
		return Reached{}, err
	}
	return nameless(f, err)
}

// nameless is the fallback's decision, and it is a function of its own because
// it is the one place in this package that can fail open and so the one place
// that has to be testable on its own. Its own test hands it a descriptor on an
// ordinary file and a descriptor on a pipe and requires the two answers.
//
// A descriptor the platform has no name for is the nameless case: no rule about
// names is speaking about it, which is what Elsewhere has always done with it.
// A descriptor the platform *does* name is a disagreement about a path that has
// a name, and the walk's error stands — the descriptor is closed rather than
// returned, because a caller handed one would have an unchecked open.
func nameless(f *os.File, walkErr error) (Reached, error) {
	if _, ok := Path(f); ok {
		_ = f.Close()
		return Reached{}, walkErr
	}
	return Reached{File: f}, nil
}

// Elsewhere is where an open reached, when that is a different place from the
// name the caller asked about.
//
// The false return is the ordinary case and covers three situations a caller
// treats identically, because in all three the decision already made is the
// decision about this object: the object has no name, the name the caller
// wrote is the path the open went through, or the two are the operating
// system's own two names for one place.
//
// The requested name is cleaned here rather than at each call site, so that
// `dir//file` and `dir/./file` are not reported as having resolved somewhere
// else — a caller that forgot would have handed its gate a second consultation
// for every path a person typed carelessly.
func Elsewhere(r Reached, requested string) (string, bool) {
	if r.Name == "" {
		return "", false
	}
	if clean := filepath.Clean(requested); r.Name == clean || samePlace(clean, r.Name) {
		return "", false
	}
	return r.Name, true
}

// Verified opens a file the way a gate needs it opened: with both of the
// flags that act *during* an open held back until check has passed.
//
// There are two of them and they are held back differently, because only one
// of them can be. O_TRUNC empties a file that is already there, so the
// descriptor exists before the damage has to happen and the flag can simply
// be applied later — that is the rest of this comment. O_CREAT makes a file
// that is not there, so there is no descriptor to check first and no later to
// defer it to; the decision has to be made inside the walk, on the parent
// descriptor the creation will be relative to, which is what
// walk.createChecked does and argues for. Asked says it happened, and is why
// a creation is not consulted about twice.
//
// Both were the same omission and only one of them was found first: a refused
// `> link` reported a refusal and left an empty file at the end of the link,
// for two releases, because the flag nobody had held back was the one that
// makes rather than the one that destroys.
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
// check is what the caller's gate says about the object the descriptor holds,
// and it is handed the path the open went through as well as the descriptor.
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
func Verified(path string, flags int, perm fs.FileMode, check func(Reached) error) (*os.File, error) {
	r, err := openAsking(path, flags&^os.O_TRUNC, perm, func(name string) error {
		// The same check, asked about a path that has no descriptor yet
		// because the file it names is about to be made. Both callers answer
		// from the name — that is what a rule matches — and the one that
		// needed a descriptor was the arrangement this package replaced.
		return check(Reached{Name: name})
	})
	if err != nil {
		return nil, err
	}
	if !r.Asked {
		if err := check(r); err != nil {
			_ = r.File.Close()
			return nil, err
		}
	}
	if flags&os.O_TRUNC != 0 {
		if err := emptyRegular(r.File); err != nil {
			_ = r.File.Close()
			return nil, err
		}
	}
	return r.File, nil
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
