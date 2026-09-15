// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/blairham/sh/interp"
)

// `-s`, the letter `zsh/files` calls paranoid: **no directory component of an
// operand may be a symbolic link, and the walk holds each directory by
// descriptor rather than resolving it by name again.**
//
// From the manual, on `rm`:
//
//	The -s option is a zsh extension to rm functionality. It enables
//	paranoid behaviour, intended to avoid common security problems
//	involving a root-run rm being tricked into removing files other than
//	the ones intended. It will refuse to follow symbolic links, so that
//	(for example) rm /tmp/foo/passwd can't accidentally remove /etc/passwd
//	if /tmp/foo happens to be a link to /etc. It will also check where it
//	is after leaving directories, so that a recursive removal of a deep
//	directory tree can't end up recursively removing /usr as a result of
//	directories being moved up the tree.
//
// Two promises, and both are about the *descent* rather than about the
// operand: which links are followed on the way to a name, and whether the way
// back out is still the way in.
//
// # What the letter is measured to change, which is less than the prose
//
// Measured 2026-09-15 against zsh 5.9.2 with `zmodload zsh/files`, from a
// scratch tree under /private/tmp (the real path, because /tmp is itself a
// symbolic link on this machine and every absolute operand through it is
// refused — which is the letter working, and was nearly recorded as a bug):
//
//	zf_rm -s par/link/passwd      par/link/passwd: not a directory, 1,
//	                              and the file it points at survives
//	zf_rm par/link/passwd         removed, 0
//	zf_rm -s -r par/tree          the tree goes, the symlink in it is
//	                              unlinked rather than descended, and what
//	                              it pointed at survives
//	zf_rm -s par/plain            removed
//	zf_rm -s par/link             the link itself is removed
//	zf_chmod -s 700 par/lf        changes the mode of what `lf` points at
//	zf_chmod -s -R 700 par/tree   the tree, exactly as without the letter
//	zf_chown -s :staff par/lf     0, and the target's group changes
//	zf_rm -s -f par/link/passwd   silent 0, and nothing is removed
//	zf_rm -s -d par/d             operation not permitted, as without it
//
// So the **final** component is treated exactly as it is without the letter —
// a link there is followed by `chmod` and unlinked by `rm` — and the descent
// below an operand is unchanged too, because that walk never followed a link
// in the first place (see fileWalk, which reads each entry with Lstat). The
// whole observable difference is the operand's own directory components.
//
// That is narrower than #1669 expected. It read the manual's second promise —
// a directory moved while the walk runs — as needing every one of the four
// commands' walks rewritten, and the promise is real; it is simply not
// something a test without a racing attacker can see. Both promises are kept
// here, and only one of them shows up in a row.
//
// # How both promises are kept with no dependency
//
// [os.Root] is a directory held open, and every method on one resolves
// relative to that descriptor and refuses to leave it. Descending with
// [os.Root.OpenRoot] one component at a time is the second promise exactly: a
// directory moved elsewhere mid-walk keeps the descriptor it was opened with,
// so there is no "where am I now" left to get wrong.
//
// The first promise needs one thing os.Root does not give, because it will
// follow a link that stays inside the root: each component is [os.Root.Lstat]
// first and refused if it is a link. The window between that check and the
// open is closed by [os.SameFile] — the directory the descent landed in has
// to be the one the check saw.
//
// **Nothing is imported for this.** The `openat` family #1669 named is not in
// `syscall` on darwin and is not reachable by number there either, and
// `golang.org/x/sys` is not a road this module may take: the shipped binary
// has no runtime dependencies at all, which internal/depsurface pins and
// internal/policy/alias.go rests an argument on. os.Root is the standard
// library doing the same system calls.

// filePlace is where one operation lands.
//
// One type for both routes rather than a paranoid twin of each walker, which
// is the shape this file exists to avoid: a second helper is how a fix reaches
// one caller and not the other. A nil root is every call without `-s` and
// means the name is the whole path and the kernel resolves it each time.
type filePlace struct {
	// root is the directory holding name, held open. Nil without `-s`.
	root *os.Root
	// name is the element inside root, or the whole resolved path where
	// there is no root.
	name string
	// full is the path the *gate* is asked about, which is a question about
	// where in the filesystem a script is reaching and is answered the same
	// either way. See filesgate.go.
	full string
	// said is the operand as the script wrote it, which is what every
	// diagnostic names — a complaint about an absolute path the script never
	// typed is a complaint about a different tree.
	said string
}

// filePlaceOf is the ordinary route: no descriptor, the name resolved by the
// kernel at each call.
func filePlaceOf(r *interp.Runner, path string) filePlace {
	at := shellPath(r, path)
	return filePlace{name: at, full: at, said: path}
}

// operandPlace is either of the two, chosen by the letter.
func operandPlace(r *interp.Runner, flags fileFlags, path string) (filePlace, error) {
	if !flags.paranoid {
		return filePlaceOf(r, path), nil
	}
	return openParanoid(r, path)
}

// openParanoid walks the directory components of an operand, holding each by
// descriptor and refusing any that is a symbolic link.
//
// The error is [syscall.ENOTDIR] for a component that is a link, which is the
// errno `openat` with `O_NOFOLLOW` gives and is the sentence the real shell
// writes: `par/link/passwd: not a directory`. It is the errno rather than a
// wording of this package's, so fileReason spells it the way it spells every
// other refusal the kernel makes.
func openParanoid(r *interp.Runner, path string) (filePlace, error) {
	full := shellPath(r, path)
	place := filePlace{name: filepath.Base(full), full: full, said: path}
	dir := filepath.Dir(full)
	root, err := openParanoidDir(dir)
	if err != nil {
		return place, err
	}
	place.root = root
	return place, nil
}

// openParanoidDir opens a directory the paranoid way: from the filesystem
// root, one component at a time.
//
// It starts at "/" rather than at the shell's own directory, and that is the
// letter and not caution: the operand is resolved to an absolute path before
// it gets here, so a relative operand carries the shell's directory in it, and
// the components of *that* are as much a part of the descent as the ones the
// script wrote. Measured — `cd /tmp` is a symbolic link on this machine, and
// zsh refuses every `-s` operand reached through it.
func openParanoidDir(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	for _, component := range strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/") {
		if component == "" || component == "." {
			continue
		}
		next, err := enterParanoid(root, component)
		if err != nil {
			_ = root.Close()
			return nil, err
		}
		_ = root.Close()
		root = next
	}
	return root, nil
}

// enterParanoid descends one component, and is where both halves of the
// promise are made.
//
// Lstat first, because os.Root follows a link that stays inside the root and
// the letter forbids following one at all; os.SameFile afterwards, because the
// check and the open are two calls and something may move between them. A
// descent that landed on a different directory from the one the check saw is
// refused rather than carried on with, which is what "check where it is" means
// when the walk cannot ask the kernel to do it in one step.
func enterParanoid(root *os.Root, component string) (*os.Root, error) {
	info, err := root.Lstat(component)
	if err != nil {
		return nil, err
	}
	// Not a directory **is** the symlink refusal, and stating it once rather
	// than twice is deliberate: Lstat never calls a link a directory, so a
	// `Mode()&fs.ModeSymlink` clause beside this one would be a second
	// condition that can never be the one that fires — a check that reads as
	// load-bearing and is not. It was written that way first, and a mutant
	// removing it changed no answer, which is how it was noticed.
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "openat", Path: component, Err: syscall.ENOTDIR}
	}
	next, err := root.OpenRoot(component)
	if err != nil {
		return nil, err
	}
	// **No row can fail when this is taken out**, and that is the honest
	// position rather than a gap in the tests: the condition it guards needs
	// something renaming a directory between two system calls, which nothing
	// here can produce. It is kept because the manual's second promise is
	// about exactly that — "check where it is after leaving directories" —
	// and it is written down so the next reader does not delete it as dead.
	landed, err := next.Stat(".")
	if err != nil || !os.SameFile(info, landed) {
		_ = next.Close()
		if err == nil {
			err = &fs.PathError{Op: "openat", Path: component, Err: syscall.ENOTDIR}
		}
		return nil, err
	}
	return next, nil
}

// closePlace gives back the descriptor the walk holds, and is a no-op for
// every call without `-s`.
func (p filePlace) closePlace() {
	if p.root != nil {
		_ = p.root.Close()
	}
}

// enter descends into this place, which has to be a directory, and answers a
// place for one entry inside it.
//
// The root is the *caller's* to close, which is why the descent hands back
// both: a walk that closed as it went would be closing the directory it is
// still reading entries out of.
func (p filePlace) enter() (*os.Root, error) {
	if p.root == nil {
		return nil, nil
	}
	return enterParanoid(p.root, p.name)
}

// child is one entry of a directory this place names, under the same
// discipline: the entry's own root is the directory just entered, so the name
// is never resolved from the top again.
func (p filePlace) child(inside *os.Root, entry string) filePlace {
	// Without a descent there is no directory to name the entry inside, so
	// the name is the whole path again — which is what every call without
	// `-s` has always been.
	name := entry
	if inside == nil {
		name = filepath.Join(p.name, entry)
	}
	return filePlace{
		root: inside,
		name: name,
		full: filepath.Join(p.full, entry),
		said: filepath.Join(p.said, entry),
	}
}

// The operations, each one call either way.

func (p filePlace) lstat() (fs.FileInfo, error) {
	if p.root == nil {
		return os.Lstat(p.name)
	}
	return p.root.Lstat(p.name)
}

func (p filePlace) readDir() ([]os.DirEntry, error) {
	if p.root == nil {
		return os.ReadDir(p.name)
	}
	f, err := p.root.Open(p.name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	// os.ReadDir sorts and *os.File.ReadDir does not, and a recursive
	// removal that took its entries in directory order would be removing
	// them in an order that differs from the same tree without the letter.
	// Sorted here so the two routes are one behavior.
	sortDirEntries(entries)
	return entries, nil
}

// removeDir removes an empty directory, and unlinkOnly removes a name that is
// not one.
//
// Two methods rather than one with a bool, because the kernel has two calls
// and `zf_rm -d` is the whole of the difference: the letter asks for the
// *unlink*, which fails on a directory, where a plain removal falls back to
// rmdir and the directory quietly goes.
func (p filePlace) removeDir() error {
	if p.root == nil {
		return os.Remove(p.name)
	}
	return p.root.Remove(p.name)
}

func (p filePlace) unlinkOnly() error {
	if p.root == nil {
		return fileUnlink(p.name)
	}
	// os.Root has no unlink that refuses a directory, so the refusal is made
	// here from what the name is. The errno is the one the call would have
	// given — measured, `zf_rm -d emptydir` is `operation not permitted` —
	// and fileReason spells it the same way for both routes.
	info, err := p.root.Lstat(p.name)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return &fs.PathError{Op: "unlink", Path: p.name, Err: syscall.EPERM}
	}
	return p.root.Remove(p.name)
}

// chmod and chown **follow** the final component, which is measured and is
// where the letter stops: `zf_chmod -s 700 par/lf` changes the mode of what
// `lf` points at, exactly as it does without the letter, and `zf_chown -s`
// the same. So these two go by the resolved path rather than through the
// descriptor — os.Root refuses a link that leaves the root, which would turn
// a line zsh runs into `path escapes from parent`.
//
// The guarantee is not weakened by it. `-s` is about the components *on the
// way to* a name, and those have already been walked and held by the time an
// operation is reached; what the last one points at is the script's own
// business, and the manual says so by giving the example `rm
// /tmp/foo/passwd` — the link it refuses is `foo` and not `passwd`.
func (p filePlace) chmod(mode fs.FileMode) error {
	return os.Chmod(p.follows(), mode)
}

func (p filePlace) chown(uid, gid int) error {
	return os.Chown(p.follows(), uid, gid)
}

// follows is the name an operation that dereferences the last component uses.
func (p filePlace) follows() string {
	if p.root == nil {
		return p.name
	}
	return p.full
}

func (p filePlace) lchown(uid, gid int) error {
	if p.root == nil {
		return os.Lchown(p.name, uid, gid)
	}
	return p.root.Lchown(p.name, uid, gid)
}

// sortDirEntries puts a directory's entries in the order os.ReadDir gives
// them, which is by name.
func sortDirEntries(entries []os.DirEntry) {
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
}
