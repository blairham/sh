// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The walk, which is the answer to a question the platform cannot answer: what
// path did this open actually go through?
//
// Asking the kernel afterwards was the previous answer and it does not hold.
// A descriptor pins an object and never a name: on Darwin the vnode carries one
// name for an object however many the filesystem has and any lookup re-stamps
// it, and on both platforms a rename moves the name out from under a descriptor
// that has not moved. So an answer read back after the fact is a fact about the
// filesystem *now*, not about the open — which is exactly the wrong thing for a
// rule that has already been applied.
//
// The walk does not ask. It resolves the path itself, one component at a time,
// keeping a descriptor on every directory it passes through and reading every
// symbolic link explicitly, so that:
//
//   - The name it reports is the sequence of components it actually traversed.
//     Nothing can make it report a path it did not walk, because the path is
//     assembled from the walk rather than looked up.
//
//   - The final open is an openat on a directory descriptor that has been held
//     since that directory was checked. A rename of any component after the
//     walk passed through it changes where the *name* leads and cannot change
//     which directory the descriptor holds, so the object that is opened is the
//     object at the path that was reported. That is the property the previous
//     arrangement could not have.
//
//   - No component is followed by accident. Every component is opened
//     O_NOFOLLOW, so a symbolic link is a thing the walk decides about rather
//     than a thing the kernel silently absorbed.
//
// What it costs is one openat per component instead of one open for the path,
// on the gated path only — a run with no gate does not enter here at all. What
// it does not cost is any public surface: the answer is still a name, so a rule
// still matches names, a Gate is still asked the same question and Resolved is
// still a path an operator can grep for.

// Reached is an open descriptor together with the name the open went through.
//
// Name is absolute and is the path the walk traversed, which for the ordinary
// case is the caller's own path with any symbolic link and any `.` or `..`
// resolved away. It is empty when the walk could not establish a name at all,
// which is the nameless case Elsewhere already treated as "no rule about names
// can be speaking about this": a descriptor on an object the filesystem does
// not name that way. See walkOpen for the one situation that produces it and
// for why it fails closed rather than guessing.
type Reached struct {
	File *os.File
	Name string
	// Asked reports that the caller's check has already been made on Name,
	// before the file at it was created.
	//
	// It is the half of the held-back O_CREAT the caller has to know about.
	// A creation cannot be checked after the fact the way an ordinary open
	// can — the file exists by then, which is the damage — so the walk asks
	// on the way past, and this says it did. A caller that asked again would
	// consult its gate twice for one open and write two audit records for
	// it, which is the duplication Elsewhere already goes out of its way to
	// avoid for a path somebody spelled carelessly.
	Asked bool
}

// walkOpen opens path by resolving it a component at a time, and reports the
// path it resolved.
//
// The flags and mode are the caller's, with two additions the caller does not
// see: O_NOFOLLOW on every component, so that following a symbolic link is a
// decision made here, and O_CLOEXEC, which the standard library's own open
// sets and a descriptor leaked into a child process would be a hole of its own.
// A caller's own O_NOFOLLOW is honored — it means "refuse a symbolic link", and
// the walk refuses it with the errno the kernel would have used.
//
// Errors are *fs.PathError with Op "open" and the caller's path, which is what
// os.OpenFile would have produced, because a script is shown these and a
// diagnostic that changed shape under a policy would tell it a policy was
// there.
func walkOpen(path string, flags int, perm fs.FileMode, ask func(string) error) (Reached, error) {
	w := &walk{}
	defer w.close()
	fail := func(err error) (Reached, error) {
		pathErr := &fs.PathError{Op: "open", Path: path, Err: err}
		if w.links > 0 {
			return Reached{}, followedALink{pathErr}
		}
		return Reached{}, pathErr
	}
	if path == "" {
		// The empty name is not the current directory, and every shell says so.
		return fail(syscall.ENOENT)
	}
	if len(path) >= pathMax {
		// The kernel measures the string it is handed, and the walk hands it
		// one component at a time — so without this a path too long to open is
		// opened under a policy and refused without one. The caller's own
		// string is what is measured, for the reason pathMax gives.
		return fail(syscall.ENAMETOOLONG)
	}
	full := path
	if !filepath.IsAbs(full) {
		// Resolved against the process's directory, which is what the kernel
		// would have done with this very path a line later. interp's rule
		// against os.Getwd is about a *shell* having an opinion on where it is
		// — two Runners in one program must not fight over one answer — and
		// this is not an opinion: it is reproducing the resolution the open is
		// about to perform, for the sole purpose of naming it. A Runner that
		// has a directory has already joined it on.
		wd, err := os.Getwd()
		if err != nil {
			return fail(err)
		}
		full = wd + "/" + full
	}

	rootfd, err := syscall.Open("/", traverseFlags()|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fail(err)
	}
	w.dirs = []int{rootfd}

	// A trailing slash is not decoration: `cat < file/` is ENOTDIR on both
	// platforms, and it survives a symbolic link, so it is carried rather than
	// cleaned away.
	wantDir := strings.HasSuffix(full, "/")
	pending := components(full)

	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]
		last := len(pending) == 0

		if comp == "." || comp == ".." {
			if comp == ".." {
				w.up()
			}
			if last {
				break
			}
			continue
		}

		flagsHere := traverseFlags()
		if last {
			flagsHere = flags
			if wantDir {
				// A trailing slash requires the target to be a directory, and
				// O_DIRECTORY is how that is asked for. O_CREAT comes off with
				// it, because the two together are EINVAL on both platforms
				// while the kernels' own answer for `> dir/new/` is a refusal
				// without a creation — ENOENT on Darwin and EISDIR on Linux,
				// which is the one corner where the two disagree with each
				// other. Dropping the flag refuses without creating anything
				// and matches Darwin exactly; see the differential test, which
				// states that rather than hiding it.
				flagsHere = (flagsHere &^ syscall.O_CREAT) | syscall.O_DIRECTORY
			}
		}
		var (
			fd      int
			openErr error
			asked   bool
		)
		if last && ask != nil && flagsHere&syscall.O_CREAT != 0 {
			fd, asked, openErr = w.createChecked(comp, flagsHere, perm, ask)
		} else {
			fd, openErr = openat(w.top(), comp, flagsHere|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(perm.Perm()))
		}
		if openErr == nil {
			if !last {
				w.push(fd, comp)
				continue
			}
			r := w.reached(fd, comp, path)
			r.Asked = asked
			return r, nil
		}
		if refused, ok := refusalOf(openErr); ok {
			// The check refused the creation and nothing was created. Its
			// own error is returned as it was given rather than wrapped in
			// the *fs.PathError a failed open produces, because the caller
			// matches on that error to tell a refusal from an errno — and
			// because there is no open here to have failed.
			return Reached{}, refused
		}
		if last && flags&syscall.O_NOFOLLOW != 0 {
			// The caller said not to follow one. The kernel's own answer for
			// that is ELOOP and it is the right answer to pass on.
			return fail(openErr)
		}
		target, isLink, err := w.readlink(comp, openErr)
		if err != nil {
			return fail(err)
		}
		if !isLink {
			// Not a symbolic link, so the open failed for its own reason and
			// that reason is the answer.
			return fail(openErr)
		}
		if last && strings.HasSuffix(target, "/") {
			// Only from the *last* component. A trailing slash on an
			// intermediate link's target says that link is a directory, which
			// it has to be anyway for the walk to continue through it, and
			// carrying it to the end would make `cat < midlink/file` demand
			// that `file` be a directory too. The kernel opens that, measured
			// on both platforms, and taking the slash from every link was the
			// bug this condition replaces.
			wantDir = true
		}
		spliced, err := w.follow(target, pending)
		if err != nil {
			return fail(err)
		}
		pending = spliced
		if len(pending) == 0 {
			// A link whose target is the root, or is `.` — the target is the
			// directory the walk now stands in.
			break
		}
	}

	// Nothing left to open by name: the target is the directory the walk ended
	// in, which is `/` itself, or a path ending in `.`, `..` or a slash.
	fd, err := openat(w.top(), ".", flags|syscall.O_DIRECTORY|syscall.O_CLOEXEC, uint32(perm.Perm()))
	if err != nil {
		return fail(err)
	}
	return Reached{File: os.NewFile(uintptr(fd), path), Name: w.name()}, nil
}

// maxLinkBytes is the buffer a symbolic link's target is read into: PATH_MAX
// on Linux and four times MAXPATHLEN on Darwin, which is the larger of the two
// and is what makes this one number rather than a platform constant. A target
// longer than the platform allows cannot be stored in the first place, so the
// only thing this size has to do is not truncate — and a truncated target would
// name somewhere else entirely, which readlink below refuses rather than walks.
const maxLinkBytes = 4096

// walk is the state of one resolution: a descriptor per directory passed
// through, the component that opened each, and how many symbolic links have
// been spent.
//
// The descriptors are the point. `..` is answered by *popping* rather than by
// asking the kernel for the parent, so the directory the walk goes back to is
// the one it came from and not whatever that name means now — and a mount
// boundary, where the kernel's `..` and the lexical one can differ, cannot
// separate the name from the descriptor because the descriptor was never
// re-derived from the name.
type walk struct {
	dirs  []int
	names []string
	links int
}

func (w *walk) top() int { return w.dirs[len(w.dirs)-1] }

func (w *walk) push(fd int, name string) {
	w.dirs = append(w.dirs, fd)
	w.names = append(w.names, name)
}

// up is `..`. At the root it stays at the root, which is what the kernel does.
func (w *walk) up() {
	if len(w.dirs) == 1 {
		return
	}
	_ = syscall.Close(w.dirs[len(w.dirs)-1])
	w.dirs = w.dirs[:len(w.dirs)-1]
	w.names = w.names[:len(w.names)-1]
}

// toRoot is what an absolute symbolic-link target does: the walk starts again
// from `/`, which is what makes the reported name the *link's* destination
// rather than the caller's path with a piece substituted.
//
// Written as a slice truncation rather than as a loop over up(), because a loop
// whose termination depends on up() popping is a loop that spins if up() ever
// stops popping — which is not hypothetical: it is exactly what a mutant that
// made up() a no-op did, and the mutant was caught by a five-minute timeout
// rather than by an assertion. The behavior is identical and there is nothing
// left to spin.
func (w *walk) toRoot() {
	for _, fd := range w.dirs[1:] {
		_ = syscall.Close(fd)
	}
	w.dirs = w.dirs[:1]
	w.names = w.names[:0]
}

// name is the path the walk has traversed so far.
func (w *walk) name() string {
	if len(w.names) == 0 {
		return "/"
	}
	return "/" + strings.Join(w.names, "/")
}

// wouldReach is the path a final component resolves to, whether or not it is
// there yet.
//
// Split from reached because a creation has to be named *before* it happens:
// the check that decides it is asked about this name while the file is still
// not on the filesystem, and the same assembly has to produce the same answer
// a moment later when the descriptor exists. One function, so the name a
// creation was allowed under and the name it is reported under cannot drift.
func (w *walk) wouldReach(comp string) string {
	name := w.name()
	if name == "/" {
		return name + comp
	}
	return name + "/" + comp
}

// reached is the answer for a final component that opened.
func (w *walk) reached(fd int, comp, requested string) Reached {
	name := w.wouldReach(comp)
	// os.NewFile rather than a hand-built File, and deliberately without
	// putting the descriptor into non-blocking mode first. os.OpenFile does
	// that for the kinds its platform can poll and arranges for Fd() to undo
	// it, which os.NewFile does not — so setting it here would hand a child
	// process a non-blocking descriptor, and a shell's whole job is handing
	// descriptors to children. Left blocking, a child inherits exactly what it
	// inherits today.
	return Reached{File: os.NewFile(uintptr(fd), requested), Name: name}
}

// createChecked opens a final component that the caller's flags allow to be
// created, asking before the creation rather than after it.
//
// This is O_CREAT's half of what Verified does for O_TRUNC, and the argument
// is the one already written there: a flag that acts *as part of* the open has
// done its work before any check on the descriptor can run, so an open that is
// then refused has already done the thing the refusal was for. O_TRUNC empties
// a file; O_CREAT makes one. Both were the damage, and only one of them was
// held back — `> link` pointing outside a policy's reach reported a refusal
// and left an empty file there, which is the boundary saying no and meaning
// mostly.
//
// The two cannot be held back the same way, and that asymmetry is the whole
// shape of this function. Truncation can be deferred because the descriptor is
// in hand and ftruncate is a second call; creation cannot, because there is no
// descriptor until it happens. So the question is asked in the one place where
// asking it is not a race: here, between the walk and the openat, with a
// descriptor on the parent directory that has been held since that directory
// was checked. Resolving the name, asking, and then opening would be the
// classic time-of-check-to-time-of-use gap this package opens by rejecting —
// the parent could mean somewhere else by the time the creation happened, and
// the check and the creation would be about two different directories. Here
// the openat is relative to that pinned descriptor, so the file is made at the
// path that was allowed or it is not made at all.
//
// The sequence is three steps and each one is load-bearing:
//
//   - Open what is already there, with O_CREAT off. If that succeeds the file
//     existed, nothing was created, and the ordinary check on the descriptor
//     is the one that decides it — so an open of an existing file is exactly
//     the call it always was, and costs no extra consultation.
//
//   - If the answer is ENOENT, this open would create. Ask, naming the path
//     the walk assembled rather than the one the caller wrote, which is the
//     point of the walk. A refusal returns before the openat.
//
//   - Create with O_EXCL, so that a file which appeared in between is not
//     silently opened as though this had made it.
//
// Any other errno is handed back untouched: a symbolic link answers ELOOP or
// ENOTDIR here and the caller's loop still has to follow it, and a real
// failure is the answer it always was. In both cases nothing has been created,
// which is the only property this function owes its caller.
func (w *walk) createChecked(comp string, flags int, perm fs.FileMode, ask func(string) error) (fd int, asked bool, err error) {
	// The file as it already is. O_EXCL comes off with O_CREAT because the
	// two are one flag — "fail if it exists" is a condition on the creating —
	// and a probe that kept it would refuse the very case it is probing for.
	probe := (flags &^ (syscall.O_CREAT | syscall.O_EXCL)) | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
	fd, err = openat(w.top(), comp, probe, 0)
	if err == nil {
		if flags&syscall.O_EXCL != 0 {
			// The caller asked for the creation to be the whole point. It
			// exists, so this is EEXIST, which is what the kernel would have
			// answered the open the caller actually wrote.
			_ = syscall.Close(fd)
			return -1, false, syscall.EEXIST
		}
		return fd, false, nil
	}
	if err != syscall.ENOENT {
		return -1, false, err
	}
	if refusal := ask(w.wouldReach(comp)); refusal != nil {
		return -1, false, refused{refusal}
	}
	fd, err = openat(w.top(), comp, flags|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(perm.Perm()))
	if err == syscall.EEXIST && flags&syscall.O_EXCL == 0 {
		// Something else created it between the probe and this call. The
		// creation that was allowed did not happen, so what is here now is a
		// file that was already there — the case the descriptor check decides,
		// reached exactly as it would have been without the race. asked stays
		// false so that check is made.
		fd, err = openat(w.top(), comp, probe, 0)
		return fd, false, err
	}
	return fd, err == nil, err
}

// refused carries a check's own refusal out through the walk, which otherwise
// deals only in errnos.
//
// A wrapper rather than a sentinel because the caller's error is the thing
// that has to survive: interp and the front end both match on their own
// refusal value, and a walk that replaced it with one of its own would leave
// them reporting a refusal as an unexplained failed open.
type refused struct{ error }

func (r refused) Unwrap() error { return r.error }

// refusalOf reports whether an error is a check's refusal, and unwraps it.
func refusalOf(err error) (error, bool) {
	var r refused
	if errors.As(err, &r) {
		return r.error, true
	}
	return nil, false
}

// readlink reports whether a component that would not open is a symbolic link,
// and what it points at.
//
// It is only asked when the open failed the two ways O_NOFOLLOW fails on a
// link, and those two were measured rather than read off POSIX: ELOOP without
// O_DIRECTORY and **ENOTDIR with it**, on Darwin 25.5.0 and Linux 6.8 alike. A
// resolver that looked only for ELOOP would treat every symlinked directory
// component as "not a directory" and refuse paths the kernel resolves — which
// is most of a Mac, where /tmp and /var are links.
//
// EINVAL from readlinkat is how a thing that is not a link says so, also
// measured on both. Anything else is a real error and is returned.
func (w *walk) readlink(comp string, openErr error) (target string, isLink bool, err error) {
	if openErr != syscall.ELOOP && openErr != syscall.ENOTDIR {
		return "", false, nil
	}
	buf := make([]byte, maxLinkBytes)
	n, rerr := readlinkat(w.top(), comp, buf)
	switch {
	case rerr == syscall.EINVAL:
		return "", false, nil
	case rerr != nil:
		return "", false, rerr
	case n <= 0 || n >= len(buf):
		// A link with no target is not a link the walk can follow, and a
		// truncated one would name somewhere else entirely. The test is `>=`
		// rather than `>` because readlinkat fills the buffer and does not say
		// whether it ran out: a target exactly as long as the buffer is
		// indistinguishable from one that was cut off. The buffer is PATH_MAX,
		// which a real target cannot reach because that length counts the
		// terminator, so this refuses nothing a filesystem can hold — which
		// also means no test can reach this line, and a mutant that weakens
		// it to `>` survives. That is stated here rather than left as an
		// unexplained gap in the score: it is a guard against a platform
		// whose readlinkat does not behave, not a branch with a case.
		return "", false, syscall.ELOOP
	}
	return string(buf[:n]), true, nil
}

// follow splices a symbolic link's target into what is left to resolve.
//
// An absolute target restarts the walk at the root, which is what makes the
// reported name the *link's* destination rather than the caller's path with a
// piece substituted. The budget is the platform's own, so a chain this walk
// refuses is a chain the kernel would have refused, with the same errno.
func (w *walk) follow(target string, pending []string) ([]string, error) {
	w.links++
	if w.links > maxSymlinks {
		return nil, syscall.ELOOP
	}
	if strings.HasPrefix(target, "/") {
		w.toRoot()
	}
	return append(components(target), pending...), nil
}

func (w *walk) close() {
	// The root descriptor included: every one of these was opened here and
	// none of them is handed out.
	for _, fd := range w.dirs {
		_ = syscall.Close(fd)
	}
	w.dirs = nil
}

// components splits a path the way the kernel walks it, which is not the way
// filepath.Clean tidies it.
//
// `.` and `..` are kept. Cleaning them away first would be wrong rather than
// merely different: filepath.Clean turns `link/..` into `.`, and the kernel
// resolves `link` first and then takes the parent of wherever it landed. Those
// are two different directories whenever the link points outside its own
// parent, and a resolver that used the lexical answer would report a path it
// had not walked — the one thing this file exists to make impossible.
func components(path string) []string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// errFollowedALink marks a walk that followed at least one symbolic link and
// then failed, which is the only shape in which Open will try the ordinary open
// instead. It is wrapped into the *fs.PathError rather than replacing it, so a
// caller that only reports the error shows the same sentence as before.
var errFollowedALink = errors.New("opened: the walk followed a symbolic link and then failed")

// followedALink wraps a walk failure so Open can tell the two cases apart.
type followedALink struct{ error }

func (f followedALink) Unwrap() []error { return []error{f.error, errFollowedALink} }
