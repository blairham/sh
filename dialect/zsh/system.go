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
// `procsubstpid`, which is answered empty here. See below.
//
// # `procsubstpid` is empty, because there is no process and 0 is a lie
//
// Three keys, measured: `pid`, `ppid`, and `procsubstpid`, which is the
// process id of the most recent process substitution and is 0 in a real zsh
// that has started none.
//
// **This shell starts none, ever.** A `<(cmd)` here runs on a goroutine of
// the shell's own process — see interp/procsubst.go — so there is no such
// process and no id for one, and that is a property of the shell rather
// than of the moment it is asked. Four answers were possible and three of
// them are worse:
//
//   - **0**, which is what real zsh says for "none yet", is the one answer
//     that must not be given. A real plugin's build script writes
//     `kill -- -$sysparams[procsubstpid]`, and `kill -- -0` is a signal to
//     the shell's *own* process group. The plausible answer is the
//     catastrophic one.
//   - **This process's pid** is the same catastrophe spelled differently.
//   - **Refusing by name** was what this did, through
//     [interp.Runner.SetAbsentElements], and it is honest but it is not
//     free: a refused expansion is *fatal to the line and status 1*, so it
//     does not merely fail to answer the caller, it takes down whatever the
//     caller was in the middle of. Measured on this machine's own startup,
//     the prompt theme's worker reads this key one line after opening its
//     response descriptor, so the refusal ended `_p9k_worker_start` before
//     it could arm the `zle -F` handler on the next line — and the theme's
//     `always` block then tried to *remove* that handler, which is where
//     the second diagnostic of every session came from:
//
//         _p9k_worker_start:32: sysparams[procsubstpid]: no process substitution …
//         _p9k_worker_stop:zle:4: No handler installed for fd 11
//
//     Two messages, one cause, and the async worker dead in a shell that
//     could have run it.
//
// So the key **exists and is empty**, which is the only one of the four
// that is both true and safe. It is true because there is no pid; it is
// safe because the guard every caller in the wild puts in front of the
// dangerous line — p10k's is `[[ -n $_p9k__worker_pid ]] && kill -- -$_p9k__worker_pid`
// — reads an empty value as "nothing to signal" and skips it, where 0 sails
// straight through. And it carries *more* than 0 does: a caller can tell
// "no process" from "process 0", which real zsh's spelling cannot.
//
// This is a deliberate deviation and the only one in this file. Real zsh
// answers `0` here and this shell answers ``; `${+sysparams[procsubstpid]}`
// is 1 in both. It is recorded rather than hidden because the alternative
// was a shell that prints two errors per prompt and runs no async segments.
//
// # It landed twice, and the first attempt is worth knowing about
//
// #1820 made this change alone and was reverted a few hours later (#1821).
// The reasoning above was right and was not sufficient: answering the key
// let the worker *start*, and what it then reached was two further walls,
// each of which cost more than the diagnostics did. Measured on the same
// machine and the same rc:
//
//	before #1820   0.42s   the two _p9k_worker diagnostics
//	after  #1820   8.0s    gitstatus failed to initialize
//
// The 8s was a hard-coded five-second `gitstatus` handshake timeout —
// `gitstatus.plugin.zsh:520` — failing because `sysread` refused a
// subscripted destination (#1828), plus the quadratic span search a
// substitution did over the prompt (#1847). Both are fixed, and with them
// in, the same session is **0.53s and prints nothing at all**.
//
// So the lesson is not about this key. It is that a refusal one line into
// a startup path hides everything behind it, and the cost of answering it
// is whatever those things turn out to be — which cannot be known until
// it is answered once.
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
		// Present and empty. See the note above: there is no process, 0 is
		// the answer that gets a caller's own process group killed, and a
		// refusal takes down the line that asked.
		"procsubstpid": "",
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
