// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The path `command -p` searches instead of the caller's.
//
// `-p` is the half of `command` a script reaches for when it cannot trust
// PATH: POSIX XCU says the search is performed with a default value for PATH
// "that is guaranteed to find all of the standard utilities", which is what
// makes `command -p sed …` the portable way to reach a tool after PATH has
// been cleared, mangled, or inherited from somewhere unhelpful. It was
// accepted here and did nothing, so the one case the letter exists for — an
// unusable PATH — answered 127 (#2933).
//
// Measured 2026-09-15 with `PATH=` and with `PATH=/nonexistent_zz`, on
// bash 5.3.15, zsh 5.9.2, dash 0.5.12, ksh93 93u+ and BusyBox ash in the
// pinned alpine image: all five find `ls` and `sed` and run them, so this is
// not a dialect question and is asked of nobody.
//
// **Two things about that measurement are worth writing down**, because each
// of them reads as a disagreement and is not one.
//
// The panel's ksh93 answers 127 for `command -p ls` written at the top level
// of a script and 0 for the same words inside `( … )`. Every other column
// answers 0 for both. The first reading of that was "ksh93 has no default
// path", which would have made this an axis and put a 2012 build's quirk into
// the ksh preset; the subshell form is the one POSIX describes and the one
// four other shells agree on, so what is implemented is the behavior and not
// the quirk.
//
// And the *environment* the command runs with is untouched. `command -p env`
// shows the caller's PATH, and `command -p sh -c 'echo $PATH'` prints the
// caller's — unanimous across the panel. So this is a search path and never an
// assignment: nothing here goes near Runner.Vars.

// standardUtilityPath is that default.
//
// A constant rather than a question asked of the system, which is the same
// choice every shell in the panel made — it is `_PATH_STDPATH` from the C
// library's <paths.h> there, compiled in, not read at startup. The value is
// the same string on the two operating systems this ships for: macOS and
// glibc both spell it this way, and the measurements above confirm it from
// the outside — `command -p ls` resolves to `/bin/ls` on this machine, where
// there is no `/usr/bin/ls` to find first, and `command -p sed` resolves to
// `/usr/bin/sed`, which only the leading `/usr/bin` explains.
//
// Asking `getconf PATH` instead would be a process this shell has to find
// before it can find anything — the circle the letter exists to break — and
// `confstr(_CS_PATH)` is not reachable from Go without cgo. Neither buys
// anything the constant does not: what a script wants from `-p` is the
// standard utilities, and these are the four directories they live in.
const standardUtilityPath = "/usr/bin:/bin:/usr/sbin:/sbin"

// commandSearchPath is the PATH a lookup walks: the shell's own, or the
// default one while `command -p` is asking.
//
// One reader rather than a `getVar("PATH")` at each search, because the two
// searches that matter — the one that runs a command and the one that reports
// where it is — must not be able to answer differently. `command -pv ls` and
// `command -p ls` are the same question asked twice.
func (r *Runner) commandSearchPath() string {
	if r.defaultPathSearch {
		return standardUtilityPath
	}
	path, _ := r.getVar("PATH")
	return path
}

// searchingTheDefaultPath puts the default path in force and answers with what
// takes it away again.
//
// Scoped to the lookup and never to what the command then does, which is
// measured rather than assumed: `command -p eval 'ls …'` is 127 in all four
// columns with an unusable PATH, so the letter reaches the word `command`
// named and nothing that word goes on to run. That is why the flag is put on
// around the lookup and the exec rather than around the builtin branch beside
// them — a builtin runs more script, and the script is the caller's.
func (r *Runner) searchingTheDefaultPath(on bool) func() {
	if !on {
		return func() {}
	}
	was := r.defaultPathSearch
	r.defaultPathSearch = true
	return func() { r.defaultPathSearch = was }
}

// rememberingLookups reports whether a resolved path may go into the command
// hash, which a default-path search may not.
//
// The hash is a memo of what the *caller's* PATH resolved, and a `-p` search
// deliberately did not use the caller's PATH — so remembering what it found
// would let a later bare `ls` run a program the script's own PATH cannot
// reach, with nothing in the script saying so.
//
// Measured 2026-09-15 and again 2026-09-18 with `PATH=/nonexistent_zz`, a
// `command -p ls` and a plain `ls` after it, from `-c` and from a script file
// under `env -i`:
//
//	column          command -p ls   the plain ls after it
//	bash 5.3.20     0               **0**
//	zsh 5.9.2       0               127
//	dash 0.5.12     0               127
//	ksh93u+         —               127
//	BusyBox ash     0               127
//
// Four columns to one, and the one is the surprising direction: bash hashes
// what the default-path search resolved, so a later bare `ls` runs a program
// the script's own PATH cannot reach with nothing in the script saying so.
// ksh93's first cell is 127 for the reason the header of this file records —
// that build answers `command -p ls` at the top level of a script with 127
// and the same words inside `( … )` with 0 — and its second cell is 127 from
// the subshell form too, so its row is on the majority side either way.
//
// Semantics.DefaultPathSearchIsRemembered is the axis, answered Yes in the
// bash preset alone. The direction that bites is bash's and ours was the safe
// one, so nothing was silently widened by the answer this had before it was
// asked at all (#2975).
func (r *Runner) rememberingLookups() bool {
	if !r.defaultPathSearch {
		return true
	}
	return r.ask(r.sem().DefaultPathSearchIsRemembered,
		"what a `command -p` search found going into the command hash")
}
