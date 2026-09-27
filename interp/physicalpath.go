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

// uncleanedJoin puts operand under base **without** canceling a `..` against
// the component before it, which is the one thing filepath.Join would do that
// a physical resolution must not have done for it.
//
// The whole difference is one component. `filepath.Join("/t", "sub/fake/..")`
// is `/t/sub`, and by the time a walk sees that the `..` it was meant to
// resolve is gone — where the five reference shells all arrive at `/t`, the
// physical parent of whatever `fake` points at. So the walk is handed the
// path as written and takes the `..` off what it has resolved instead.
//
// An absolute operand is already the whole path and a Runner with no
// directory has nothing to join against; both come back untouched, which is
// what physicalPath then declines rather than guessing at.
func uncleanedJoin(base, operand string) string {
	if base == "" || filepath.IsAbs(operand) {
		return operand
	}
	return base + string(filepath.Separator) + operand
}

// hasDotDotComponent reports whether path holds a `..` as a component of its
// own, which is what one session switch is keyed on — see
// Runner.CdResolvesDotDot.
//
// A component and not a substring: `..` decides, `...` and `a..b` do not.
// Measured on the shell that has the switch, `cd sub/./fake` keeps the
// logical name with it on, so a `.` is not this question either.
func hasDotDotComponent(path string) bool {
	for _, comp := range splitPathComponents(path) {
		if comp == ".." {
			return true
		}
	}
	return false
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

// operandCrossesASymlink walks operand's own components from base and reports
// whether any of them is a symbolic link. It is what `cd -s` refuses — see
// Semantics.CdHasSymlinkFreeOption.
//
// The walk starts at base rather than at the root for a relative operand,
// which is the measurement and not an economy: `cd link` and then `cd -s deep`
// moves in zsh, though the directory it arrives in is reached through a link.
// Only what the operand itself names is examined. An absolute operand has no
// base and is walked from the root, which is what refuses `cd -s /tmp/x` on a
// machine where `/tmp` is a link.
//
// `.` and `..` are components like any other and are lstatted **where they
// stand**, which is why the path is built by concatenation rather than by
// filepath.Join: `..` out of a linked directory is the physical parent and is
// not itself a link, and cleaning the path first would answer a different
// question. Measured, `cd -s ../cdtest/real` moves in a `/tmp/cdtest` whose
// `/tmp` is a link — a lexical clean makes that operand `/tmp`, finds the
// link, and refuses a move the shell makes.
//
// A component that cannot be lstatted ends the walk with no refusal. What to
// say about a path that is not there is the ordinary failure's to say, and
// saying it here would answer `cd -s nosuchdir` with `not a directory`, which
// no shell does.
//
// Through the gate, one component at a time, for physicalPath's reason: this
// is a walk over the filesystem that a script chose the path for, so a policy
// has to see each step of it.
func (r *Runner) operandCrossesASymlink(base, operand string) bool {
	at := base
	if filepath.IsAbs(operand) {
		at = filepath.VolumeName(operand) + string(filepath.Separator)
	}
	for _, comp := range splitPathComponents(operand) {
		if strings.HasSuffix(at, string(filepath.Separator)) {
			at += comp
		} else {
			at += string(filepath.Separator) + comp
		}
		info, err := r.lstat(at)
		if err != nil {
			return false
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

// canceledComponentRefused reports why a `..` in operand may not be taken out
// of the path without looking: the component it would cancel is not there, or
// is not a directory.
//
// The operand is walked **as written**, because the thing being asked about is
// exactly what cleaning would remove, and the stack it pops from starts at
// base — so a `..` that runs past the operand goes on to consume the
// directory the shell is already in, which is a component like any other and
// is where the three readings part.
//
// withinOperand keeps the looking to the operand's own components and cancels
// one belonging to base unseen, which is ksh93's reading. See
// [Semantics.CdCancelsADotDot] for the grids and for where the question is
// put.
//
// A **stat of the component** rather than a walk through it, which is measured
// rather than convenient: a directory at mode 000 cancels perfectly well in
// bash and zsh, so neither traverses what it is canceling, and stat needs
// nothing of the component itself. It follows links, which is the other half —
// `sub/fake/..` where `fake` points at a directory is accepted by all six, and
// a *dangling* link is refused by the four that look, which is one reading and
// not two.
//
// A `..` with nothing left to cancel is asked nothing: at the root it is the
// root in every column, and in a relative path with no base it names no
// component this could ask about.
//
// Through the gate, because a script chose the path and a policy that hides
// part of the filesystem must be able to answer for a question about it — the
// same reason the walk above is gated a component at a time.
func (r *Runner) canceledComponentRefused(base, operand string, withinOperand bool) error {
	path := uncleanedJoin(base, operand)
	vol := filepath.VolumeName(path)
	rest := path[len(vol):]
	lead := ""
	if strings.HasPrefix(rest, "/") || (filepath.Separator != '/' && strings.HasPrefix(rest, string(filepath.Separator))) {
		lead = "/"
	}
	// How much of the stack belongs to base, so that a `..` popping below
	// this mark is one that has reached past the operand.
	fromBase := 0
	if !filepath.IsAbs(operand) {
		fromBase = len(splitPathComponents(base))
	}
	var built []string
	for _, comp := range splitPathComponents(rest) {
		switch comp {
		case ".":
			continue
		case "..":
			if len(built) == 0 {
				continue
			}
			if withinOperand && len(built) <= fromBase {
				built = built[:len(built)-1]
				continue
			}
			canceledPath := vol + lead + strings.Join(built, "/")
			info, err := r.stat(canceledPath)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return &fs.PathError{Op: "chdir", Path: canceledPath, Err: syscall.ENOTDIR}
			}
			built = built[:len(built)-1]
		default:
			built = append(built, comp)
		}
	}
	return nil
}

// physicalPathCancelingIntoTheDirectoryHeld resolves operand physically, with
// one exception: a `..` that reaches past the operand and into the directory
// the shell is **logically** in cancels a component of that directory unseen,
// and only what is left of the path is resolved.
//
// One column reads `-P` that way and the other four resolve the whole path
// with every `..` in place — see [Semantics.CdCancelsADotDot], which is the
// same axis and the same reading, `CdDotDotLooksWithinTheOperand`, that the
// `-L` route is keyed on. Asked only where the operand holds a `..`, so an
// ordinary `cd -P` puts no question.
//
// **The two kinds of `..` interleave, which is why this is a walk and not a
// two-part split.** Cancel the logical ones first and hand the rest to the
// physical resolver and `cd -P deep/../..` comes out wrong: the first `..`
// belongs to the operand, resolves physically, and leaves the walk somewhere
// the second `..` must be read from. Every `..` is therefore decided as it is
// reached, by whether the component it pops is one the operand put there.
//
// Measured 2026-09-27 against ksh93u+ 2012-08-01 — with dash 0.5.12, bash
// 5.3.20, zsh 5.9.2 and BusyBox ash 1.37.0 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// as the unanimous controls — in a tree `t` holding `real/deep`, `sub`, and
// `sub/fake` pointing at `../real`; `go version -m` says *not a Go executable*
// for each of the four native ones. `$PWD` is the logical name the shell
// arrived under, and the same figures come back whether it was reached
// absolutely or a component at a time:
//
//	from              cd -P …          ksh93    the unanimous four
//	t                 sub/fake/..      t        t
//	t                 sub/fake/deep/.. t/real   t/real
//	t                 sub/fake/../..   above t  above t
//	t/sub             fake/..          t        t
//	t/sub/fake        ..               t/sub    t
//	t/sub/fake        ./..             t/sub    t
//	t/sub/fake        ../real          refused  t/real
//	t/sub/fake        ../..            t        above t
//	t/sub/fake        .././..          t        above t
//	t/sub/fake        deep/..          t/real   t/real
//	t/sub/fake        deep/../..       t        t
//	t/sub/fake        deep/../../..    above t  above t
//	t/sub/fake        ../fake/..       t        refused
//	t/sub/fake/deep   ..               t/real   t/real
//
// Three of those rows are the split and two of them carry the rule. **`../fake/..`
// is the sharpest**: the first `..` cancels the link's own name logically,
// putting the walk in `t/sub` where a `fake` really is, and the second
// resolves it physically — so one operand produces both readings, and the
// four columns that never take the first step refuse the row outright. And **`deep/../..` is what rules out "a leading `..`"**: the
// first `..` pops a component the operand put there and resolves, the second
// reaches `$PWD` and cancels, and the answer is decided per component rather
// than by what the operand starts with. `t/sub/fake/deep` with a bare `..` is
// the row that looks like agreement and is not evidence: the two readings
// coincide one level down.
//
// A failure is handed back as physicalPath's is, and the caller does with it
// what it does for any path it could not resolve.
func (r *Runner) physicalPathCancelingIntoTheDirectoryHeld(base, operand string) (string, error) {
	if base == "" || filepath.IsAbs(operand) {
		// Nothing of the shell's own directory is in the path, so there is
		// no component for a `..` to reach into and the two readings are
		// the same one.
		return r.physicalPath(operand)
	}
	vol := filepath.VolumeName(base)
	built := splitPathComponents(base[len(vol):])
	// How much of the stack the shell's own directory put there. A `..` that
	// would pop below this mark is one the operand ran past.
	fromBase := len(built)
	joined := func() string {
		return vol + string(filepath.Separator) + strings.Join(built, string(filepath.Separator))
	}
	for _, comp := range splitPathComponents(operand) {
		switch comp {
		case ".":
			continue
		case "..":
			if len(built) == 0 {
				// The root's parent is the root, in every column.
				continue
			}
			if len(built) <= fromBase {
				// A component of the directory the shell is logically in:
				// canceled without being looked at, which is the whole of
				// this reading.
				built = built[:len(built)-1]
				fromBase--
				continue
			}
			// The operand's own, so it is the physical parent — which means
			// resolving what has been built before taking the parent off,
			// since the component being popped may be a link.
			resolved, err := r.physicalPath(joined())
			if err != nil {
				return "", err
			}
			rest := resolved[len(vol):]
			parent := splitPathComponents(rest)
			if len(parent) > 0 {
				parent = parent[:len(parent)-1]
			}
			built, fromBase = parent, 0
		default:
			built = append(built, comp)
		}
	}
	if resolved, err := r.physicalPath(joined()); err == nil {
		return resolved, nil
	} else {
		// **The path that is blamed is the canceled one**, which is what
		// makes this reading visible in a failure as well as in an arrival:
		// `cd -P ../real` from inside a link names the directory the
		// cancellation built and not the operand, because the operand is not
		// where this shell looked. An operand that failed on its own
		// components above returns nothing here and is blamed as written,
		// which is the same shell's answer for `cd -P nosuch/..`.
		return joined(), err
	}
}
