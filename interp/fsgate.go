// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"syscall"
)

// Every stat, link read and directory read in the interpreter comes through
// here, so the whole category passes the gate and reaches the event stream. It
// matters because a probe is an oracle: `[ -f /etc/shadow ]` learns something
// real about the filesystem, `echo /**` enumerates it, and a PATH search stats
// a candidate in every directory PATH names — none of which a policy could
// see, let alone refuse, while these went to the os package directly.
//
// That claim has to be re-checked against the code rather than read, and it
// was false once: `cd -P` and `pwd -P` handed a script-chosen path to
// filepath.EvalSymlinks, which lstats and reads every component of it through
// the os package. A policy hiding a subtree could not stop `cd -P` walking
// into it and reporting what it found, and nothing recorded the walk. The
// resolution loop in physicalpath.go replaced that call, so the sentence above
// is a statement about the code again.
//
// A refusal is quiet, unlike a refused exec or open. Those stop a command
// that cannot honestly run without them, and say so; a probe's caller only
// wants an answer, and "no such file" is one. The gate hiding a path and the
// kernel not having it are indistinguishable on purpose — the deny semantics
// are documented on ActionStat and ActionReadDir, and every caller inherits
// them by treating the error exactly as it treats a missing path.
//
// The nil-nil fast path is load-bearing: glob descent and PATH lookup are
// hot, and a Runner with no Gate and no Sink must pay two nil checks and
// nothing else.

// stat is os.Stat through the gate.
func (r *Runner) stat(path string) (os.FileInfo, error) {
	if r.Gate == nil && r.Events == nil {
		return os.Stat(path)
	}
	if r.probeDenied(r.act(Action{Kind: ActionStat, Path: path})) {
		// The errno the kernel gives for a path that is not there, not
		// fs.ErrNotExist: a caller that prints the reason must print the
		// same sentence for both, or the refusal identifies itself.
		return nil, &fs.PathError{Op: "stat", Path: path, Err: syscall.ENOENT}
	}
	return os.Stat(path)
}

// lstat is os.Lstat through the gate: the same question about the link
// itself, so it is the same action to the policy.
func (r *Runner) lstat(path string) (os.FileInfo, error) {
	if r.Gate == nil && r.Events == nil {
		return os.Lstat(path)
	}
	if r.probeDenied(r.act(Action{Kind: ActionStat, Path: path})) {
		return nil, &fs.PathError{Op: "lstat", Path: path, Err: syscall.ENOENT}
	}
	return os.Lstat(path)
}

// readLink is os.Readlink through the gate. Reading where a link points is
// the same question about the link that lstat asks — what is this component,
// and what is behind it — so it is the same action to the policy, and a path
// the policy hides answers the way a path that is not a link answers.
func (r *Runner) readLink(path string) (string, error) {
	if r.Gate == nil && r.Events == nil {
		return os.Readlink(path)
	}
	if r.probeDenied(r.act(Action{Kind: ActionStat, Path: path})) {
		return "", &fs.PathError{Op: "readlink", Path: path, Err: syscall.ENOENT}
	}
	return os.Readlink(path)
}

// readDir is os.ReadDir through the gate. A denied directory reads as empty,
// which is what the error below means to every caller: nothing to descend
// into, nothing to match.
//
// Unlike its neighbors it opens the directory itself rather than handing the
// path to os.ReadDir, and the reason is the one verifyopen.go sets out: a
// name is not an object, so the listing is taken from a descriptor whose
// object the gate has agreed to. `echo link/*` enumerating a denied directory
// through a link is the bug that closes, and it is worth closing here because
// a listing is the loudest oracle the filesystem has.
//
// A refusal at that second point is the quiet one the first is, for the same
// reason and with the same error: to a glob a directory the policy hides is a
// directory that is not there, and it must not be able to tell the two apart
// by whether a link was involved.
func (r *Runner) readDir(path string) ([]os.DirEntry, error) {
	if r.Gate == nil && r.Events == nil {
		return os.ReadDir(path)
	}
	action := r.act(Action{Kind: ActionReadDir, Path: path})
	if r.probeDenied(action) {
		return nil, &fs.PathError{Op: "readdirent", Path: path, Err: syscall.ENOENT}
	}
	if r.Gate == nil {
		// Watching without gating: there is nothing a verification could
		// refuse, so the standard library's one-call form stands.
		return os.ReadDir(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if !r.verifyOpened(r.ctx, action, f) {
		return nil, &fs.PathError{Op: "readdirent", Path: path, Err: syscall.ENOENT}
	}
	entries, err := f.ReadDir(-1)
	// Sorted, because os.ReadDir sorts and every caller here was written
	// against that: a glob's expansion is in name order and would otherwise
	// come out in whatever order the directory happens to be stored in.
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, err
}

// probeDenied consults the gate about a probe and emits the record either
// way. It does not report and does not touch the status — the refusal is the
// caller's to fold into whatever a missing path already means there.
func (r *Runner) probeDenied(a Action) bool {
	if r.Gate != nil && r.Gate.Allow(r.ctx, a) == Deny {
		r.emit(r.ctx, Event{Kind: EventDenied, Action: a})
		return true
	}
	r.emit(r.ctx, Event{Kind: EventAccess, Action: a})
	return false
}

// openQuietlyDenied consults the gate about an open whose refusal the caller
// reports through a diagnostic path of its own, rather than through the
// generic refusal allowed() prints. `.` is the caller: a file it may not read
// is reported the way a file it cannot read is, fatality axis included.
func (r *Runner) openQuietlyDenied(a Action) bool {
	if r.Gate != nil && r.Gate.Allow(r.ctx, a) == Deny {
		r.emit(r.ctx, Event{Kind: EventDenied, Action: a})
		return true
	}
	return false
}

// errRefused is the reason a gate's refusal reads as, where it is routed
// through a diagnostic that prints one.
var errRefused = errors.New("refused")
