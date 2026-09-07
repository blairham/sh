// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
)

// Where a path physically is, resolved one gated component at a time.
//
// `cd -P` and `pwd -P` report the directory rather than the name it was
// reached by, and doing that means following every symlink on the way. The
// standard library will do the whole walk in one call, and that is exactly
// what makes it wrong here: filepath.EvalSymlinks lstats and reads each
// component through the os package, so a policy denying a subtree still let
// `cd -P hidden/link` traverse it, learn where the link pointed, and go there,
// with nothing in the event stream to show it had happened. The existence
// check that followed was gated, but by then the answer was already in hand —
// a boundary drawn after the fact is not a boundary.
//
// So the walk is written out here over r.lstat and r.readLink. Every component
// is an ActionStat the gate can refuse and a record the sink receives, which
// is the whole difference; the resolved path this returns is the same one the
// library call returned.
//
// It also keeps the rule that interp may not consult the process. EvalSymlinks
// resolves a relative path against the *process's* directory, which a Runner
// does not own — two Runners in one program have two directories and neither
// is that one. This resolves against r.Dir, and declines a path it cannot make
// absolute rather than borrowing one.
//
// A failure is not reported and not distinguished. Both callers do what they
// did for an unresolvable path before: leave it as written, and let the gated
// stat that follows say what the operating system says about it. That is why a
// symlink cycle still reads as ELOOP in each dialect's own wording — the
// kernel supplies the reason, and the panel is unanimous that a cycle is the
// same failure under `-P` as under `-L`, where the chdir hits it anyway.

// maxSymlinkHops bounds the walk. A hand-rolled resolver has to bound it or a
// cycle spins forever — `ln -s a b; ln -s b a; cd -P a` is two links and no
// bottom. The value sits above every SYMLOOP_MAX in the panel's reach (32 on
// macOS, 40 on Linux), so the kernel refuses a chain this deep before we do
// and the limit never decides an answer; it only guarantees the loop ends.
const maxSymlinkHops = 64

// physicalPath resolves path to where it physically is, through the gate.
//
// The error is the first one a component's lstat or readlink gave, or ELOOP
// when the walk ran past its bound. Callers treat any of them as "could not
// resolve" — the distinction belongs to the stat that follows, not here.
func (r *Runner) physicalPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.workDir(), path)
	}
	if !filepath.IsAbs(path) {
		// A Runner with no Dir means "stay relative", and there is no
		// directory to resolve against that this package is allowed to ask
		// for. Unresolvable, which the callers already know what to do with.
		return "", &fs.PathError{Op: "lstat", Path: path, Err: syscall.ENOENT}
	}
	vol := filepath.VolumeName(path)
	pending := splitPathComponents(path[len(vol):])
	resolved := vol
	hops := 0
	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]
		switch comp {
		case ".":
			continue
		case "..":
			// The parent of where we have got to, which is already fully
			// resolved — so this is the physical parent rather than the one
			// the name suggests, and stripping the last element is exactly
			// that. At the root it is the root.
			if i := strings.LastIndexByte(resolved, filepath.Separator); i >= len(vol) {
				resolved = resolved[:i]
			} else {
				resolved = vol
			}
			continue
		}
		candidate := resolved + string(filepath.Separator) + comp
		info, err := r.lstat(candidate)
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			resolved = candidate
			continue
		}
		hops++
		if hops > maxSymlinkHops {
			return "", &fs.PathError{Op: "lstat", Path: path, Err: syscall.ELOOP}
		}
		target, err := r.readLink(candidate)
		if err != nil {
			return "", err
		}
		// An absolute target restarts from the root; a relative one is read
		// from where its link sits, which is where we already are. Either
		// way what is left of the original path still has to follow it.
		tvol := filepath.VolumeName(target)
		if filepath.IsAbs(target) {
			resolved = tvol
			target = target[len(tvol):]
		}
		pending = append(splitPathComponents(target), pending...)
	}
	if resolved == vol {
		return vol + string(filepath.Separator), nil
	}
	return resolved, nil
}

// splitPathComponents breaks a path into its non-empty components, dropping
// the separators. Repeated and trailing separators contribute nothing, which
// is what makes `//a//b/` and `/a/b` the same walk.
//
// The slice is freshly allocated, which the walk relies on: it prepends this
// to what is still pending, and appending into a shared array would overwrite
// the tail it has yet to read.
func splitPathComponents(path string) []string {
	return strings.FieldsFunc(path, func(c rune) bool {
		return c == '/' || c == filepath.Separator
	})
}

// physicalPrefix resolves as much of path as exists and leaves the rest
// alone, which is what a *modifier* wants where `cd -P` wants an error.
//
// `${x:A}` and `${x:P}` never fail: a path that is not there comes back with
// its existing prefix resolved and the rest appended exactly as written, so
// `/tmp/no/such` is `/private/tmp/no/such` where `/tmp` is a link and `no` is
// not there. A dangling symlink is left as its own name for the same reason —
// the walk stops at the component it cannot follow.
//
// Separate from physicalPath rather than a flag on it because the two want
// opposite things from the same failure, and a bool parameter at the call
// site would say which only to whoever looked the function up.
func (r *Runner) physicalPrefix(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.workDir(), path)
	}
	if !filepath.IsAbs(path) {
		return path
	}
	vol := filepath.VolumeName(path)
	pending := splitPathComponents(path[len(vol):])
	resolved := vol
	hops := 0
	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]
		switch comp {
		case ".":
			continue
		case "..":
			// The parent of what is already resolved, which is the physical
			// parent rather than the one the name suggests. This is the whole
			// difference between `:P` and `:A`: the other one cancels `..`
			// lexically before the walk begins.
			if i := strings.LastIndexByte(resolved, filepath.Separator); i >= len(vol) {
				resolved = resolved[:i]
			} else {
				resolved = vol
			}
			continue
		}
		candidate := resolved + string(filepath.Separator) + comp
		info, err := r.lstat(candidate)
		if err != nil {
			// As far as it goes. What is left is appended as written,
			// including this component, because none of it can be resolved
			// once its parent cannot be.
			return joinRemainder(candidate, pending)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			resolved = candidate
			continue
		}
		// A link whose target is not there is left as its own name, which is
		// measured: `${x:A}` on a dangling link answers the link, not what it
		// points at. stat follows the whole chain, so this is also the answer
		// for a chain that ends in one.
		if _, serr := r.stat(candidate); serr != nil {
			return joinRemainder(candidate, pending)
		}
		hops++
		if hops > maxSymlinkHops {
			return joinRemainder(candidate, pending)
		}
		target, err := r.readLink(candidate)
		if err != nil {
			return joinRemainder(candidate, pending)
		}
		tvol := filepath.VolumeName(target)
		if filepath.IsAbs(target) {
			resolved = tvol
			target = target[len(tvol):]
		}
		pending = append(splitPathComponents(target), pending...)
	}
	if resolved == vol {
		return vol + string(filepath.Separator)
	}
	return resolved
}

// joinRemainder puts an unresolvable component and everything still pending
// back on the end of what was resolved.
func joinRemainder(resolved string, pending []string) string {
	if len(pending) == 0 {
		return resolved
	}
	return resolved + string(filepath.Separator) + strings.Join(pending, string(filepath.Separator))
}
