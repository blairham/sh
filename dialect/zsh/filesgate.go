// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/blairham/sh/interp"
)

// Every system call `zsh/files` makes about a path the script named, with the
// gate asked first.
//
// The module is nine builtins whose whole purpose is modifying the
// filesystem, and until #1819 not one of them asked anything: each called the
// `os` package directly, so a policy of `default deny` still let `zf_rm`
// delete what it was pointed at, `zf_mkdir` create outside the boundary, and
// `zf_ln` hard-link a denied file into an allowed directory — after which its
// contents read back normally, because a hard link is a second name rather
// than a link a resolving walk could notice.
//
// That was not an oversight by the module's author so much as the shape
// #1808 describes: a dialect holds no Boundary, so the guard that reads every
// package which opens files never read this one, and `AllowOpen` was the only
// seam a dialect had — an open, which none of these nine performs.
//
// # The wrappers are the whole discipline
//
// Nothing below calls the `os` package with a script-chosen path except
// through one of these, and the module calls the `os` package nowhere else.
// That is what makes the property checkable by reading one file rather than
// by auditing nine command implementations, and it is why even the probes are
// here: `zf_rm nosuch` and `zf_rmdir notadir` answer from an `Lstat`, and an
// ungated one is an oracle for exactly what the policy hides.

// fileMayModify asks whether the script may change what is at this path.
//
// The path is the operand as the script wrote it, resolved against the
// shell's own directory rather than the process's — see shellPath, which is
// the difference between the file the script meant and whatever that name
// happens to be beside the program that embedded this shell.
func fileMayModify(r *interp.Runner, ctx context.Context, path string) bool {
	return r.AllowModify(ctx, shellPath(r, path))
}

// fileMayModifyTarget is fileMayModify for an operation that follows a
// symbolic link to do its work, which `chmod` and `chown` do unless `-h`
// said otherwise.
//
// Checking the operand alone would be a hole rather than a check: a link
// inside an allowed directory pointing at a denied file is a name the policy
// permits and an object it does not, so `zf_chmod 777 ./link` would change
// the mode of the thing the rule was written to protect. So the chain is
// walked and **every hop is asked about**, the final object included.
//
// A run with no gate is unaffected beyond the readlink calls: AllowModify
// answers yes for everything when nothing is watching, so this neither
// refuses nor reorders anything a shell without a policy used to do.
//
// The walk is bounded because a symbolic link may point at itself. The bound
// is the platform's own answer to that — the kernel gives up too — and a
// chain longer than this is left to the system call, which reports the loop
// in the words the script expects.
func fileMayModifyTarget(r *interp.Runner, ctx context.Context, path string) bool {
	at := shellPath(r, path)
	for hop := 0; hop < fileLinkHops; hop++ {
		if !r.AllowModify(ctx, at) {
			return false
		}
		info, err := os.Lstat(at)
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			// Not a link, or gone: either way there is no next hop, and the
			// error is the system call's to report in its own words.
			return true
		}
		dest, err := os.Readlink(at)
		if err != nil {
			return true
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(at), dest)
		}
		at = dest
	}
	return true
}

// fileLinkHops bounds the chain fileMayModifyTarget walks. Darwin follows 32
// and Linux 40, so refusing to walk further than the shorter of the two hands
// the rest to the kernel, which reports ELOOP the way every shell does.
const fileLinkHops = 32

// fileMayRead asks whether the script may reach the contents at a path.
//
// `zf_ln` is the caller and the reason this is a read rather than a write: a
// hard link does not copy anything, but it puts the object inside whatever
// directory the new name is in, and every later read of that name is then
// allowed on its own merits. The contents cross the boundary when the link is
// made, so that is where the question belongs (#1819).
func fileMayRead(r *interp.Runner, ctx context.Context, path string) bool {
	return r.AllowReadPath(ctx, shellPath(r, path))
}

// fileLstat is os.Lstat through the gate, with the probe semantics the rest
// of the shell's probes have: a path the policy hides is answered exactly as
// a path that is not there.
//
// Every one of the nine starts with one of these — `rm` to learn whether the
// operand is a directory, `ln` and `mv` to learn whether the source exists,
// `rmdir` to refuse a name that is not a directory — and each answer is a
// fact about a file the script may not otherwise touch.
func fileLstat(r *interp.Runner, ctx context.Context, path string) (fs.FileInfo, error) {
	at := shellPath(r, path)
	if !r.AllowProbe(ctx, at) {
		return nil, fileNotThere("lstat", at)
	}
	return os.Lstat(at)
}

// fileStat is os.Stat through the gate, for the one caller that asks about
// what a link points at: `fileIsDir` deciding whether a destination is a
// directory to put things in.
func fileStat(r *interp.Runner, ctx context.Context, path string) (fs.FileInfo, error) {
	at := shellPath(r, path)
	if !r.AllowProbe(ctx, at) {
		return nil, fileNotThere("stat", at)
	}
	return os.Stat(at)
}

// fileReadDir is os.ReadDir through the gate. `zf_rm -r` is the caller, and
// enumerating a directory is the thing ActionReadDir exists to cover: a
// recursive removal that could list a denied tree has learned its shape even
// where every unlink in it is refused.
func fileReadDir(r *interp.Runner, ctx context.Context, path string) ([]os.DirEntry, error) {
	at := shellPath(r, path)
	if !r.AllowList(ctx, at) {
		return nil, fileNotThere("open", at)
	}
	return os.ReadDir(at)
}

// fileNotThere is what a refused probe answers with: the kernel's own errno
// for a path that is not there, not fs.ErrNotExist, so that a caller printing
// the reason prints the same sentence for both. A refusal that could be told
// apart from a missing file is an oracle for what the policy hides — the rule
// interp/fsgate.go states and every caller inherits.
func fileNotThere(op, path string) error {
	return &fs.PathError{Op: op, Path: path, Err: syscall.ENOENT}
}
