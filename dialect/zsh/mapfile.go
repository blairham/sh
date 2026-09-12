// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"

	"github.com/blairham/sh/interp"
)

// The `zsh/mapfile` module: the filesystem presented as an association.
//
// One parameter and nothing else — measured 2026-09-12, zsh 5.9.2 under
// `zsh -f`, where `zmodload -lF zsh/mapfile` writes exactly `+p:mapfile`.
// A key is a path, the value is that file's bytes, and the three things a
// script can do to an element are the three things it can do to a file:
//
//	${mapfile[p]}          read p whole
//	mapfile[p]=x           write x to p, creating or truncating it
//	unset "mapfile[p]"     unlink p
//
// # Why this module is the sharper test of the gate
//
// `zsh/files` is nine builtins that each name a path, and #1819 was the work
// of finding that not one of them asked anything. This module names a path
// nowhere: the whole of it is a *parameter*, so every route it opens arrives
// through expansion and assignment rather than through a command, and the
// two enumerating forms name no path at all —
//
//	${(k)mapfile}          the names in the working directory
//	${(kv)mapfile}         those names and the contents of every one
//
// — which is a readdir and then a read of everything it found, spelled as a
// parameter flag. A boundary that only watches commands does not see it.
//
// So the discipline `filesgate.go` opens with is the discipline here, and
// this file is small enough to be the whole of it: **nothing below calls the
// `os` package with a script-chosen path except through mapfileRead,
// mapfileWrite, mapfileRemove and mapfileNames**, each of which asks the gate
// before it does anything, and the module calls the `os` package nowhere
// else.
//
// # The context these have and were not handed
//
// A dynamic association's three seams take no [context.Context] — a read is
// an expansion and a write is an assignment, and neither is a command with
// one to pass. The gate takes one. `interp/mathfunc.go` settled the same
// question for the other dialect callback in this position and the answer is
// the shell's own [interp.Runner.ShellContext]: an expansion happens in the
// middle of a command, and that command's context is the only one there is.
//
// # What a refusal looks like
//
// Reading a file's contents is an **open**, so a refused read is reported the
// way a refused redirection is — `open: refused: <path>` — rather than with a
// probe's silence. That is the loud half and it is deliberate: a script that
// asked for a file's contents and got an empty string with nothing said would
// have no way to tell the file apart from an empty one.
//
// What must not differ is the *denied* pair. A path the policy covers reads
// back identically whether or not anything is there, value and wording alike,
// because a refusal that varied would be an oracle for what the policy is
// hiding — a script could walk a denied tree and learn its shape without
// reading a byte. cmd/sh's TestADeniedMapfileReadSaysNothingAboutWhetherThe-
// FileIsThere is the pair, with the operand normalized out of both.
//
// A refused *write* is reported once, by [interp.Runner.AllowModify], in the
// same words. Nothing here says it again.
//
// # Where the key roster and one key disagree, and why that is zsh's shape
//
// [interp.Runner.SetDynamicAssocElement] asks that the one-key reading agree
// with the whole table's, because which of the two answers `${m[k]}` decides
// whether `${m[k]:-d}` takes the default. Here they cannot agree and the
// reason is the module's own design rather than this implementation's: the
// roster is the *working directory*, and a key may be any path at all.
// `${(k)mapfile}` does not name `/etc/hosts` and `${mapfile[/etc/hosts]}`
// reads it — in zsh too. The element producer is the only reading a single
// key ever takes, so the disagreement costs nothing and is written down here
// rather than being discovered.
//
// The same split decides what each asks the gate, and the module hands it to
// us: **the roster carries no values**. Measured — `for k v in
// "${(@kv)mapfile}"` walks a two-file directory and prints `val=[]` for both,
// and `${(v)mapfile}` is a run of empty fields. A file's contents come from a
// subscript and from nowhere else.
//
// So the roster's question is *what is in this directory*, which is
// [interp.Runner.AllowList]'s, and it is the only question the enumerating
// forms ask; an element's is *what is in this file*, which is
// [interp.Runner.AllowReadPath]'s. That is the whole of the gate's job here,
// and reading every file to answer `${(k)mapfile}` would have been both
// slower than zsh and wrong.

// registerMapfileModule installs `$mapfile`, which is the whole module.
func registerMapfileModule(r *interp.Runner) {
	// The roster, which is a directory listing. Reached by `${(k)mapfile}`,
	// `${(v)mapfile}`, `${#mapfile}` and `${(kv)mapfile}`.
	r.SetDynamicAssoc("mapfile", mapfileView)
	// And one key, which is nearly every use of the module and must not
	// build the roster to answer: reading `${mapfile[f]}` through the view
	// would list the working directory and read every file in it to hand
	// back one.
	r.SetDynamicAssocElement("mapfile", mapfileValue)
	// And the two writes, which share a hook: `set` false is the unset, and
	// an unset of a file is an unlink.
	r.SetDynamicAssocWriter("mapfile", writeMapfile)
	// `typeset -H`, which is what zsh marks it: measured `${(t)mapfile}` is
	// `association-hide-hideval-special`. Without it a listing that reached
	// the name would write out the contents of every file in the directory
	// as an assignment somebody could source back.
	r.MarkHidden("mapfile")
	// And `-h` beside it, which is the other half of the same measurement:
	// `local mapfile` inside a function is an ordinary parameter in the shell
	// this models rather than a second view of the filesystem. Both or the
	// `(t)` word is wrong — see Runner.MarkHideInScope.
	r.MarkHideInScope("mapfile")
}

// mapfileValue answers `${mapfile[path]}` — one file, read whole.
//
// Not-found rather than empty-and-found for everything that is not a readable
// file, which is measured rather than chosen: zsh answers `${mapfile[nosuch]}`
// with the empty string at status 0 and does not put the key in `(k)`, and a
// directory reads the same way. Folding a refusal into that same answer is
// what keeps the policy from being an oracle.
func mapfileValue(r *interp.Runner, key string) (string, bool) {
	data, ok := mapfileRead(r, key)
	if !ok {
		return "", false
	}
	return string(data), true
}

// mapfileView answers the roster: the names in the working directory, each
// with an empty value.
//
// Empty is not a gap. It is what zsh answers — `for k v in "${(@kv)mapfile}"`
// over a directory holding `a` and `b` prints `val=[]` twice, and `${(v)mapfile}`
// is a run of empty fields — and it is the reason `${(k)mapfile}` is a listing
// rather than a listing plus a read of everything in it. A view that filled
// the values in would answer a question zsh does not, at the cost of a read
// per file and a gate decision per read, and `${(v)mapfile}` would then differ
// from the shell this models in the direction that leaks.
//
// One key still reads its file, through mapfileValue, which is the only
// reading a subscript takes.
func mapfileView(r *interp.Runner) interp.AssocArray {
	names, ok := mapfileNames(r)
	if !ok {
		return interp.AssocArray{}
	}
	out := make(interp.AssocArray, len(names))
	for _, name := range names {
		out[name] = ""
	}
	return out
}

// writeMapfile is `mapfile[p]=x` and `unset "mapfile[p]"`, which are a write
// and an unlink.
func writeMapfile(r *interp.Runner, key, value string, set bool) {
	if !set {
		mapfileRemove(r, key)
		return
	}
	mapfileWrite(r, key, value)
}

// Every system call this module makes about a path a script named, with the
// gate asked first. Nothing above reaches the `os` package by another route.

// mapfileRead reads one file, having asked whether the script may.
//
// The bytes are the file's exactly — no trailing newline is added or removed,
// measured: a file holding `one\ntwo\n` reads back as `one\ntwo\n`, which is
// what makes `${(f)mapfile[f]}` split into three fields with an empty last
// one rather than two.
func mapfileRead(r *interp.Runner, key string) ([]byte, bool) {
	path := shellPath(r, key)
	if !r.AllowReadPath(r.ShellContext(), path) {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// mapfileWrite replaces a file's contents, having asked whether the script
// may modify that name.
//
// `0666` before the umask, which is what zsh leaves behind: a file created by
// `mapfile[o]=abc` under the default `022` is `0644`, measured. The value is
// written as given — no newline is appended, so `mapfile[o]="abc"` is a
// three-byte file.
//
// A refusal is silent here because AllowModify has already reported it, in
// the same words a refused redirection gets.
func mapfileWrite(r *interp.Runner, key, value string) {
	path := shellPath(r, key)
	if !r.AllowModify(r.ShellContext(), path) {
		return
	}
	// The error is dropped, which is zsh's own answer rather than an
	// omission: `mapfile[/nosuch/x]=y` writes no diagnostic and leaves `$?`
	// at 0, because an assignment's status is the assignment's.
	_ = os.WriteFile(path, []byte(value), 0o666)
}

// mapfileRemove unlinks a file, having asked whether the script may modify
// that name.
//
// An unlink is a modification of the name and not a read of it, so this asks
// the same question a write does. Measured: `unset "mapfile[w]"` removes `w`,
// and `unset mapfile` — the whole parameter — removes nothing at all.
func mapfileRemove(r *interp.Runner, key string) {
	path := shellPath(r, key)
	if !r.AllowModify(r.ShellContext(), path) {
		return
	}
	_ = os.Remove(path)
}

// mapfileNames lists the working directory, having asked whether the script
// may enumerate it.
//
// The shell's directory and not the process's — see shellPath. `cd` moves the
// roster, measured: `${(k)mapfile}` in a two-file subdirectory names those
// two, and names the parent's files after a `cd ..`.
func mapfileNames(r *interp.Runner) ([]string, bool) {
	dir := r.Dir
	if dir == "" {
		dir = "."
	}
	if !r.AllowList(r.ShellContext(), dir) {
		return nil, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, true
}
