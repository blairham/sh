// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The home directory a shell holds, as opposed to the `HOME` parameter a
// script can see.
//
// Two measured rows, and they are the two ends of one mechanism: a shell that
// keeps a home of its own initialized at startup can fill in one the
// environment did not hand it, and can go on having one after the parameter
// has been removed. A shell that only ever reads the parameter can do neither.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f`; `go version -m` says *not a Go
// executable* for it — with the binary copied to a file called `sh` so that
// nothing but the invocation name differs:
//
//	                                        $HOME in the child   `cd`
//	env -u HOME zsh -c …                    /Users/…             goes there, 0
//	env -u HOME sh  -c …                    unset                `HOME not set`, 1
//	zsh -c 'unset HOME; cd'                 unset                nothing, 0
//	sh  -c 'unset HOME; cd'                 unset                nothing, 0
//	env -u HOME sh -c 'HOME=/tmp; unset HOME; cd'  unset         nothing, 0
//	env HOME= sh -c cd                      empty                nothing, 0
//
// The first two rows are the first mechanism and the split is the **mode**
// rather than the word: `--emulate sh` under the name `zsh` answers as the
// third row does. See Semantics.StartupFillsAnAbsentHome.
//
// Rows three to five are the second, and the discriminating pair is rows two
// and five: the same binary, the same absent `HOME`, and the only difference
// is that one of them assigned a value first. Only a shell that has **never**
// had a home says `HOME not set`. See Semantics.CdRemembersAHomeThatWasUnset.

// SeedHomeDirectory fills in a `HOME` the environment did not hand this shell,
// for the one column that does.
//
// The same shape as ensurePWD — a parameter a shell seeds for itself once,
// before anything a script writes can be overwritten by it — and idempotent
// for its reason, since a startup sequence is not a single call.
//
// **The front end calls it, and where in the sequence is the whole of why.**
// An emulation taken from `argv[0]` or from `--emulate` is what decides the
// axis, and the front end applies that *after* the dialect's prelude has run;
// a seed inside RunPart would therefore fire on the prelude, under the
// preset's own answer, and a binary called `sh` would fill in a home its mode
// says it does not. Measured — it did, until this moved. So it sits beside
// Runner.ApplyInheritedParameters in driver, right after the emulation and
// before anything a person wrote.
//
// **The question is put only where there is something to answer it with**, so
// an ordinary shell puts none: the parameter has to be absent *and* this
// Runner has to have a password database to ask. A Runner with no
// Runner.UserHomeDir is a library embedded in a program with no business
// reading a password file, which is the same reading a written `~` gets — see
// Semantics.TildeWithNoHome, whose zsh answer is what it is *because* of this
// row.
func (r *Runner) SeedHomeDirectory() {
	if _, ok := r.getVar("HOME"); ok {
		return
	}
	if r.UserHomeDir == nil {
		return
	}
	// Read as the field rather than through ask, and the difference matters
	// here: ask *refuses* an unanswered axis, and a refusal needs a command
	// to fail. This is a startup seed — there is no line of script in front
	// of it, the status it would set is one nothing has read yet, and the
	// sentence would arrive before the shell had run anything. So an
	// unanswered value is No, which is what both presets say in as many words
	// and what four of the six columns hold.
	if r.sem().StartupFillsAnAbsentHome != Yes {
		return
	}
	if home, ok := r.UserHomeDir(""); ok && home != "" {
		r.setVar("HOME", home)
		// And exported, which is the reference's own answer and not the
		// environment's: measured 2026-10-01 on zsh 5.9.2 under `env -i
		// PATH=/usr/bin:/bin`, `env | grep -c HOME` is 1 and `typeset +x`
		// names HOME, from a shell that was handed none (#5332).
		r.MarkExported("HOME")
		// The shell has a home now, so `unset HOME` after this leaves an
		// empty destination rather than no destination — which is the
		// reference's own answer, since the seeded value is a home it was
		// given exactly as an inherited one is.
		r.homeWasSet = true
	}
}

// noteHomeIsGoing records that a `HOME` with a value is being removed, which
// is what the shell remembers a home *by*.
//
// A flag rather than the value, which is measured: after `HOME=/tmp; unset
// HOME`, `cd` goes nowhere and answers 0 rather than going to `/tmp`. So what
// survives the removal is a home that has been **emptied**, and an empty home
// is somewhere — it is the same silent 0 an inherited `HOME=` already gives.
//
// Read at the removal rather than at the assignment, which is what makes an
// *inherited* `HOME` count without the assignment path having to notice
// anything: `env HOME=/tmp sh -c 'unset HOME; cd'` is 0, and `env -u HOME sh
// -c 'unset HOME; cd'` is `HOME not set` at 1. The two differ only in whether
// there was a value here to remove.
func (r *Runner) noteHomeIsGoing(name string) {
	if name != "HOME" {
		return
	}
	if _, ok := r.getVar(name); ok {
		r.homeWasSet = true
	}
}
