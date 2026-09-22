// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
)

// Four parameters this shell supplies that were simply absent here, and the
// attributes its own parameters carry.
//
// Measured 2026-09-18 with `env -i PATH=/usr/bin:/bin LC_ALL=C` and a scratch
// HOME, over a script file, against bash 5.3.20 (#3098, #3099):
//
//	declare -p BASHPID     declare -i BASHPID="68901"
//	declare -p SRANDOM     declare -i SRANDOM="631212817"
//	declare -p HISTCMD     declare -i HISTCMD="0"
//	declare -p GROUPS      declare -a GROUPS=([0]="20" [1]="12" …)
//
// All four take an assignment at status 0 and **discard** it — `BASHPID=5`
// then `$BASHPID` is the pid again, `GROUPS=5` then `${GROUPS[0]}` is the
// first group id again — which is what the writers below say, and which is
// also what keeps an assignment from shadowing the producer with a stored
// value nothing would ever read back. `unset` takes each of them away for
// good, which is the producer seam's own answer and needs nothing here.
//
// **`$BASHPID` in a subshell is the deviation, and it is recorded rather than
// papered over.** In this shell a subshell is a fork, so `$$` keeps the
// parent's number and `$BASHPID` is the child's; that difference is the whole
// reason the parameter exists. Here a subshell is a cloned Runner in one
// process, so there is no second pid to report. The answer follows
// dialect/zsh's `$sysparams[pid]` exactly — the process group where the
// subshell has one of its own, and empty otherwise — because the failure the
// other reading invites is the same one it invited there: a body that
// believes it has a process of its own will `kill` it, and answering `$$`
// would have it kill the shell (#2046).
func registerShellParameters(r *interp.Runner) {
	// The pid of *this* shell, which is `$$` everywhere but a subshell.
	r.SetDynamic("BASHPID", func(rr *interp.Runner) string { return shellPid(rr) })
	r.SetDynamicWriter("BASHPID", func(*interp.Runner, string) {})
	r.SetDynamicDeclaration("BASHPID", interp.ProducedDeclaration{Integer: true})
	// A 32-bit value from the system entropy source, and **not** `RANDOM`
	// under another name: it is a different width, a different source, and
	// an assignment to it seeds nothing. Measured, two reads on one line are
	// two different numbers and `SRANDOM=42` twice does not repeat a pair
	// the way `RANDOM=42` does.
	r.SetDynamic("SRANDOM", func(*interp.Runner) string {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			// The entropy source is the kernel's and does not fail in
			// practice; a shell that refused a parameter read would be a
			// worse answer than a number, so this reports zero rather than
			// giving up an expansion.
			return "0"
		}
		return strconv.FormatUint(uint64(binary.BigEndian.Uint32(b[:])), 10)
	})
	r.SetDynamicWriter("SRANDOM", func(*interp.Runner, string) {})
	r.SetDynamicDeclaration("SRANDOM", interp.ProducedDeclaration{Integer: true})
	// The history number the command being read would get. Zero where
	// nothing is recording, which is the measured row: a script has history
	// off, and `echo $HISTCMD` three times over is `0` three times.
	r.SetDynamic("HISTCMD", func(rr *interp.Runner) string {
		if !rr.HistoryRecording() {
			return "0"
		}
		return strconv.Itoa(rr.HistoryFirst() + len(rr.HistoryEntries()))
	})
	r.SetDynamicWriter("HISTCMD", func(*interp.Runner, string) {})
	r.SetDynamicDeclaration("HISTCMD", interp.ProducedDeclaration{Integer: true})
	// The user's group ids, in the order the system reports them. A read of
	// the process for the reason `$UID` is one: nothing a script does
	// changes it, and two Runners in one program genuinely have the same
	// answer.
	r.SetDynamicArray("GROUPS", func(*interp.Runner) []string {
		ids, err := os.Getgroups()
		if err != nil {
			return nil
		}
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = strconv.Itoa(id)
		}
		return out
	})
	r.SetDynamicArrayWriter("GROUPS", func(*interp.Runner, []string) {})
	r.SetDynamicDeclaration("GROUPS", interp.ProducedDeclaration{Array: true})
	// The shell's own `$0`, and the one parameter here whose assignment is
	// **kept** rather than discarded: writing it renames the shell.
	//
	// Measured 2026-09-21 against bash 5.3.20 (#TBD):
	//
	//	declare -p BASH_ARGV0            declare -- BASH_ARGV0="/…/bash"
	//	BASH_ARGV0=one; echo "$0"        one      -- `$0` moved with it
	//	f(){ BASH_ARGV0=in; }; f; $0     in       -- and it outlasts the call
	//	( BASH_ARGV0=sub ); echo "$0"    unchanged -- a subshell keeps its own
	//	unset BASH_ARGV0; echo "[$0]"    the name it had; the link is gone
	//
	// No letters in the listing, which is why this one takes the zero value
	// of ProducedDeclaration where its three neighbors take `-i` or `-a`.
	//
	// **It renames `$0` and not the shell.** In a script the diagnostic goes
	// on naming the file after the assignment — measured, `nosuchcmd` on the
	// line after is still `./t.sh: line 3: …` — so this writes the override
	// rather than Runner.Name. See Runner.shellNameForZero.
	r.SetDynamic("BASH_ARGV0", func(rr *interp.Runner) string { return rr.DollarZeroName() })
	r.SetDynamicWriter("BASH_ARGV0", func(rr *interp.Runner, value string) {
		rr.SetDollarZeroName(value)
	})
	r.SetDynamicDeclaration("BASH_ARGV0", interp.ProducedDeclaration{})
}

// shellPid is what `$BASHPID` answers — see the note above registerShell-
// Parameters for why a subshell without a process group of its own answers
// nothing rather than answering `$$`.
func shellPid(r *interp.Runner) string {
	if r != nil && r.InSubshell() {
		if pgid, ok := r.SubshellProcessGroup(); ok {
			return strconv.Itoa(pgid)
		}
		return ""
	}
	return strconv.Itoa(os.Getpid())
}

// The directory stack as a parameter, which is a **view** and not a store.
//
// `pushd`, `popd` and `dirs` are the prelude's, and they keep the pushed
// entries in `__dirstack` — see dialect/bash/prelude.go. What a script reads
// is this shell's own arrangement: the current directory first and the pushed
// entries behind it.
//
// Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, over a script file against bash 5.3.20:
//
//	cd /tmp; pushd /usr; cd /etc
//	  ${DIRSTACK[0]} is /etc            slot zero is $PWD now, not at push time
//	cd /tmp; pushd /usr; pushd /etc     dirs: /etc /usr /tmp
//	  DIRSTACK[1]=/var                  dirs: /etc /var /tmp
//	  DIRSTACK[0]=/zzz                  dirs unchanged, and $PWD unchanged
//	  DIRSTACK=5                        dirs unchanged
//	  DIRSTACK=(/a /b)                  dirs: /etc /b /tmp
//	  DIRSTACK+=(/c)                    dirs unchanged
//	  unset 'DIRSTACK[2]'               dirs unchanged
//
// Three rules out of those rows, and each is a way a store would have got it
// wrong. **Slot zero is not storage** — it is `$PWD`, so a write to it is
// discarded and a `cd` moves it. **A write to slot N replaces entry N-1 where
// there is one** and is dropped where there is not, which is why
// `DIRSTACK=(/a /b)` leaves `/tmp` standing and `+=` changes nothing. And
// **the stack never grows or shrinks through the parameter**: `pushd` and
// `popd` are the only things that change its length.
func registerDirectoryStack(r *interp.Runner) {
	r.SetDynamicArray("DIRSTACK", func(rr *interp.Runner) []string {
		pushed, _ := rr.GetArray(dirStackStorage)
		return append([]string{rr.Dir}, pushed...)
	})
	r.SetDynamicArrayWriter("DIRSTACK", func(rr *interp.Runner, values []string) {
		pushed, ok := rr.GetArray(dirStackStorage)
		if !ok || len(pushed) == 0 {
			// Nothing pushed, so every slot but zero is past the end and
			// zero is $PWD: the whole write is dropped, which is what
			// `DIRSTACK=5` in a shell that has never pushed is measured to
			// do.
			return
		}
		next := append([]string(nil), pushed...)
		for i := 1; i < len(values) && i-1 < len(next); i++ {
			next[i-1] = values[i]
		}
		rr.SetArray(dirStackStorage, next)
	})
	r.SetDynamicDeclaration("DIRSTACK", interp.ProducedDeclaration{Array: true})
}

// dirStackStorage is the name the prelude's `pushd`, `popd` and `dirs` keep
// the pushed entries under. Spelled once, here, because the parameter above
// and the shell text in prelude.go are the two halves of one arrangement and
// a second spelling is how they would come apart.
const dirStackStorage = "__dirstack"

// The attributes this shell puts on its own parameters.
//
// Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, over a script file against bash 5.3.20 (#3099):
//
//	declare -p EUID            declare -ir EUID="501"
//	declare -p UID             declare -ir UID="501"
//	declare -p PPID            declare -ir PPID="63824"
//	declare -p OPTIND          declare -i OPTIND="1"
//	declare -p BASH_VERSINFO   declare -ar BASH_VERSINFO=([0]="5" …)
//	declare -p RANDOM          declare -i RANDOM="20062" — already right
//
// Every one of them listed with no letters at all here, and the counted cost
// was the filtered listings: `declare -i` wrote 1 row where that shell writes
// 9, `declare -r` 3 against 7, `declare -a` 2 against 10.
//
// **The freeze is not cosmetic.** `EUID=0`, `UID=0` and `PPID=1` are each
// refused there — an assignment error, which ends a non-interactive shell —
// and were taken here at status 0, so a script that assigns to one to find
// out whether it is allowed got the opposite answer.
//
// `OPTIND` is the one of the four that is **not** frozen, which is measured
// rather than inferred: `getopts` writes it and a script resets it between
// scans, so freezing it would break the builtin that owns it.
//
// Done here rather than through ProducedDeclaration because none of these is
// produced: they are ordinary stored names, so there is no producer for a
// listing's letters to hang off, and the letter on them is a real attribute
// that an assignment consults. BASH_VERSINFO is the prelude's array and is
// frozen here for the same reason — the prelude has no word for it and the
// mark is this package's fact either way.
func markOwnParameterAttributes(r *interp.Runner) {
	for _, name := range []string{"EUID", "UID", "PPID"} {
		r.MarkInteger(name)
		r.MarkReadonly(name)
	}
	r.MarkInteger("OPTIND")
	// BASH_VERSINFO is frozen by the prelude on the line after it fills the
	// array in, and not here: a mark made now would refuse that assignment.
	// See dialect/bash/prelude.go's identity.
}
