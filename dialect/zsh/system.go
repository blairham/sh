// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
)

// The three of `zsh/system` that are not builtins: `$sysparams`, `$errnos` and
// the math function `systell`.
//
// Measured 2026-09-09 against zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files. `zmodload -lF zsh/system` names nine features —
// six builtins, one math function and two parameters — and the six builtins
// are `sysopen`, `sysread`, `syswrite`, `sysseek`, `syserror` and `zsystem`.
//
// **All nine are implemented, and they arrived in two halves.** This file and
// systemio.go were #1737, which left `zsystem`, `sysseek` and `syserror`
// registered in the feature table and refused *by name* — honest, and not the
// same as done, since `zmodload -F zsh/system b:zsystem` was 1 here and 0 in
// zsh. systemlock.go and systemseek.go are #1749, which closed that gap. The
// rule that made the intermediate state legitimate is zmodload.go's and is
// unchanged: a builtin this shell has not got refuses by name on the line that
// calls it, so it never holds a module shut.
//
// # `$sysparams[pid]` is the key a real startup reads
//
// Counted across the installed plugin tree, `$sysparams` is read fourteen
// times and `$errnos` and `systell` not once. Twelve of the fourteen are
// `pid` — an autosuggestion plugin, a syntax highlighter and a prompt each
// wanting the process they are running in — and the other two are
// `procsubstpid`, which is refused here. See below.
//
// # `procsubstpid` refuses, and answering 0 would have been dangerous
//
// **This refusal was replaced by an empty answer in #1820 and put back by
// #1821, and the reason it came back is the whole of what to know before
// trying again.** Answering it is right and is not sufficient on its own:
// the refusal is fatal to the line, so it was killing `_p9k_worker_start`
// before the theme could arm its `zle -F` handler — but with the key
// answered, the worker starts, reaches `gitstatus_start`, and *that* fails
// on #1532 (`sysread` into a subscripted parameter) after a hard-coded five
// second timeout. Measured: a session went from 0.42s with two diagnostics
// to 8.0s with a `gitstatus failed to initialize` banner, which is worse on
// both counts. #1532 lands first; this follows it.
//
// Three keys, measured: `pid`, `ppid`, and `procsubstpid`, which is the
// process id of the most recent process substitution and is 0 in a shell that
// has started none.
//
// **This shell starts none.** A `<(cmd)` here runs on a goroutine of the
// shell's own process — see interp/procsubst.go — so there is no such process
// and no id for one. Answering the 0 that means "none yet" was the obvious
// thing and is the one answer that must not be given: a real plugin's build
// script writes
//
//	kill -- -$sysparams[procsubstpid]
//
// and `kill -- -0` is a signal to the shell's *own* process group. So the key
// refuses by name, at the expansion that asks, through
// [interp.Runner.SetAbsentElements] — the same mechanism `$terminfo` uses for
// a capability it has no value for, and the same reason: a caller cannot tell
// an honest 0 from an invented one.
//
// `pid` is answered and is this process's, which is true rather than
// approximate — and it is worth saying what it is *not*. In zsh, `$sysparams[pid]`
// differs from `$$` inside a subshell, because a subshell there is a fork and
// `$$` keeps the parent's number; that difference is the reason the key
// exists. Here a subshell is a cloned Runner in one process, so the two agree
// everywhere. Nothing is being rounded off: the key reports the process this
// shell is in, and this shell is in one process.
//
// # `$errnos` is an indexed array and the platform's
//
// `typeset -p errnos` writes `typeset -ar errnos` — an array, where
// `$sysparams` is `typeset -Ar` — indexed by errno number, holding the name.
// Measured on this machine it runs 1 to 107 with no holes, from `EPERM` to
// `ENOTCAPABLE`.
//
// The numbering is the operating system's and not zsh's, so the table is built
// per platform from the constants Go's `syscall` package declares for it —
// facts about the platform's own headers, arrived at without reading any
// shell. See errnotable_darwin.go and errnotable_linux.go, which also record
// the two things that cannot be read off a constant: the one darwin errno Go
// has no name for, and which of two names an alias gets on Linux.
//
// # `systell` is a math function, not a builtin
//
// The listing writes it `+f:systell`, and `f` is zsh's letter for a math
// function — so it is called from arithmetic, `$(( systell(3) ))`, and not as
// a command. Measured: it is the descriptor's current offset in bytes, it
// moves as the descriptor is read, and a descriptor nothing is open at is -1
// rather than an error. That -1 is the shape of the whole function: it does
// not refuse, it reports.

// registerSystemModule installs `$sysparams`, `$errnos` and `systell`.
func registerSystemModule(r *interp.Runner) {
	r.SetDynamicAssoc("sysparams", sysparamsView)
	// Readonly and hidden together, the pair every produced table in this
	// dialect needs: zsh answers `sysparams[pid]=5` with `read-only
	// variable: sysparams`, and readonly without hidden would put the whole
	// table into a `typeset -p` listing as assignments somebody could source
	// back.
	r.MarkReadonly("sysparams")
	r.MarkHidden("sysparams")
	// And the key this shell has no process to report on, refusing by name
	// rather than reading as the 0 that means "none yet". See the note above
	// on why that particular 0 is the one that must not be invented.
	r.SetAbsentElements("sysparams", "no process substitution runs in a process of its own here")
	r.SetDynamicArray("errnos", errnosView)
	r.MarkReadonly("errnos")
	r.MarkHidden("errnos")
	r.RegisterMathFunction("systell", 1, 1, mathSystell)
	// And the module's six builtins: the three that move bytes, the file
	// lock, and the two that close it. See systemio.go, systemlock.go and
	// systemseek.go.
	registerSystemIO(r)
	r.Register("zsystem", zsystemBuiltin)
	r.Register("sysseek", sysseekBuiltin)
	r.Register("syserror", syserrorBuiltin)
}

// sysparamsView is `$sysparams`, produced at every read.
//
// A view rather than a table filled in once, which matters for the same reason
// it matters everywhere else in this dialect and for one more: `os.Getpid` is
// cheap, and a shell that cached it would be handing out a stale number to
// anything that had re-execed itself.
func sysparamsView(*interp.Runner) interp.AssocArray {
	return interp.AssocArray{
		"pid":  strconv.Itoa(os.Getpid()),
		"ppid": strconv.Itoa(os.Getppid()),
	}
}

// errnosView is `$errnos`: the platform's error names, indexed by number.
//
// Index 0 is dropped on the way out because a shell array starts at 1 in this
// dialect and there is no errno 0 to name — the table is written with the
// number as the offset so that a name sits where its errno says, and the
// leading empty is that offset showing.
func errnosView(*interp.Runner) []string {
	if len(errnoNames) == 0 {
		return nil
	}
	return append([]string(nil), errnoNames[1:]...)
}

// mathSystell is `systell(fd)`: where the file open at one of this shell's
// descriptors is positioned.
//
// -1 for a descriptor nothing is open at, which is measured — `$(( systell(99)
// ))` is -1 and not a complaint — and is also the answer for a descriptor
// holding something with no position at all, which is what a pipe and a
// terminal are. See [interp.Runner.DescriptorOffset], which is the seam and
// which is where the difference between those cases stops being visible.
func mathSystell(r *interp.Runner, call interp.MathCall) (interp.MathValue, error) {
	fd, err := call.Value(0)
	if err != nil {
		return interp.MathValue{}, err
	}
	off, ok := r.DescriptorOffset(fd.Int())
	if !ok {
		return interp.MathInt(-1), nil
	}
	return interp.MathInt(int(off)), nil
}
