// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// What this shell's own stored parameters say about themselves.
//
// `${(t)UID}` was `scalar` here where the reference says `integer-special`,
// and the same was true of every name the shell stores for itself rather than
// producing: `$EUID`, `$GID`, `$EGID`, `$IFS`, and both halves of all eight
// built-in ties. The produced ones — `$RANDOM`, `$LINENO`, `$SECONDS` — were
// right throughout, because a producer is somewhere to hang the facts and a
// stored name had nowhere (#4488).
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, run `-f` from a script file, against
// `go build ./cmd/zsh` from this tree. `go version -m` says *not a Go
// executable* for the reference and `github.com/blairham/sh/cmd/zsh` for
// ours, so these are two programs:
//
//	                  reference                   here, before
//	${(t)UID}         integer-special             scalar
//	${(t)EUID}        integer-special             scalar
//	${(t)GID}         integer-special             scalar
//	${(t)EGID}        integer-special             scalar
//	${(t)IFS}         scalar-special              scalar
//	${(t)PATH}        scalar-tied-export-special  scalar-tied-export
//	${(t)path}        array-tied-special          array-tied
//	typeset -p UID    typeset -i10 UID=501        typeset UID=501
//
// and the other seven pairs answer as `PATH`/`path` do, `MAILPATH` carrying
// the export attribute beside `PATH` and the six others not.
//
// **`SetSpecial` does not do this itself**, although it stores five of the
// names, and that is the row that says so: `$HOST` is stored through the same
// hook and `${(t)HOST}` is a bare `scalar` in the reference — measured the
// same day, in the same run. A hook that marked what it stored would have
// taught this shell a fact the reference contradicts. So the list is written
// out, one name at a time, and a name added to `SetSpecial` tomorrow is not
// silently marked by arriving.
func markTheShellsOwnParameters(r *interp.Runner) {
	for _, name := range identityParameters {
		r.MarkShellOwnParameter(name)
		// The integer letter and the base a listing writes beside it. Stated
		// as a *declaration* rather than put in the attribute table, because
		// the attribute table also decides what an assignment to the name
		// means and the reference's answer to that is a system call this
		// shell does not make — see Runner.SetDynamicDeclaration, which has
		// the measurement, and the `$GID` comment in zsh.go for why the
		// write is left alone.
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Integer: true, Base: 10})
	}
	guardIdentityAssignments(r)
	r.MarkShellOwnParameter("IFS")
	for _, pair := range builtInTies {
		r.MarkShellOwnParameter(pair[0])
		r.MarkShellOwnParameter(pair[1])
	}
	for _, name := range storedShellOwnParameters {
		r.MarkShellOwnParameter(name)
	}
	for _, name := range shellOwnWhileSet {
		r.MarkShellOwnParameterWhileSet(name)
	}
	markTheProcessDepthAndIndex(r)
	markTheDirectoryStack(r)
}

// storedShellOwnParameters are the names this shell stores an ordinary value
// into and still calls its own.
//
// #4488 marked the names `SetSpecial` and `Tie` make; these are the rest, and
// nothing distinguishes them from a script's variable except that the shell
// put them there. Swept 2026-09-27 — `${(t)NAME}` for each of the 123 names a
// fresh `zsh -f` lists under `typeset +`, one script, both shells in the same
// run, `-f` from a script file under `env -i PATH=/usr/bin:/bin` with a
// scratch `HOME`, against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`. `go version -m` says *not a Go executable*
// for the reference and `github.com/blairham/sh/cmd/zsh` for ours, so the
// two columns are two programs:
//
//	              reference                 here, before
//	HOME          scalar-export-special     scalar-export
//	HISTSIZE      integer-special           integer
//	NULLCMD       scalar-special            scalar
//	READNULLCMD   scalar-special            scalar
//	WORDCHARS     scalar-special            scalar
//	PS1           scalar-special            scalar
//	PS2           scalar-special            scalar
//	PS4           scalar-special            scalar
//	histchars     scalar-special            scalar
//
// **The control is the 72 rows of that sweep that already agreed**, which is
// what makes the list a finding rather than "this shell never says special":
// `IFS`, `RANDOM`, `SECONDS`, `LINENO`, `path` and `PATH` all matched, and so
// did `HOST scalar` — the row that says the mark cannot be moved into
// `SetSpecial`, since `$HOST` is stored through that hook and carries no
// `special` in the reference.
//
// `histchars` is in the list and `HISTCHARS` is not, for the reason `PS1` is
// in it and `PROMPT` is not: each of those pairs is one parameter under two
// names, the lower-case half holds the value and the upper-case half is
// produced over it — and a producer is already the statement, so the produced
// half has been answering `special` all along while the half that holds the
// value had nowhere to hang the fact. That asymmetry is what the sweep found:
// `HISTCHARS`, `PROMPT`, `PROMPT2`, `PROMPT3` and `PROMPT4` agreed in the
// same run their stores disagreed in.
var storedShellOwnParameters = [...]string{
	"HISTSIZE",
	"HOME",
	"NULLCMD",
	"PS1",
	"PS2",
	"PS4",
	"READNULLCMD",
	"WORDCHARS",
	"histchars",
}

// shellOwnWhileSet are the names this shell calls its own once something has
// set them, and leaves absent until then — see
// interp.Runner.MarkShellOwnParameterWhileSet for the measurement (#5575).
// The terminal and locale names a script or the environment supplies, and
// the right-hand prompts, whose `PROMPT`-style aliases already answered.
var shellOwnWhileSet = [...]string{
	"LANG",
	"LC_ALL",
	"LC_COLLATE",
	"LC_CTYPE",
	"LC_MESSAGES",
	"LC_NUMERIC",
	"LC_TIME",
	"RPROMPT",
	"RPROMPT2",
	"RPS1",
	"RPS2",
	"TERM",
	"TERMINFO",
	"TERMINFO_DIRS",
}

// markTheProcessDepthAndIndex is the three names whose *kind* was wrong as
// well as their specialness, and for two of them the attribute is not
// decoration.
//
// Measured in the same sweep and the same run as the list above:
//
//	              reference                  here, before
//	OPTIND        integer-special            scalar
//	PPID          integer-readonly-special   scalar
//	SHLVL         integer-export-special     scalar-export
//
// and, in the reference, `typeset -p` writes `typeset -i10 OPTIND=1` and
// `export -i10 SHLVL=1`, `SHLVL=1+2` stores 3, `SHLVL=abc` stores 0, and
// `PPID=7` is `read-only variable: PPID` at status 1 where this shell took it
// at 0. So the integer letter and the freeze are the attribute tables and not
// a listing's spelling — an assignment consults both — which is the split
// [interp.Runner.SetIntegerParameter] states and the opposite of the choice
// the four identity names above are given.
//
// The freeze is on `PPID` alone. `OPTIND` is the name `getopts` writes and a
// script resets between scans, and `SHLVL` is what a nested shell increments,
// so freezing either would refuse a write the shell itself makes — the same
// row dialect/bash/shellparameters.go records for its own four.
func markTheProcessDepthAndIndex(r *interp.Runner) {
	for _, name := range [...]string{"OPTIND", "PPID", "SHLVL"} {
		r.MarkShellOwnParameter(name)
		// Base ten written down rather than left off, which is what
		// separates one of this shell's own integers from one a script
		// declared: the reference lists all three with `-i10` where its own
		// `typeset -i x=5` carries no base at all.
		r.SetIntegerParameter(name, 10)
	}
	r.MarkReadonly("PPID")
	// And the row a `-p` listing writes for it, which is none.
	//
	// `$PPID` answers the four listing forms exactly as `$ARGC` and `$LINENO`
	// do, and those two have carried [interp.ProducedDeclaration.Silent]
	// since they were implemented — which is why they already agreed and this
	// one did not. Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script
	// file, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, all three names
	// in one run:
	//
	//	                   PPID        ARGC        LINENO
	//	typeset -p NAME    nothing, 0  nothing, 0  nothing, 0
	//	readonly -p        no row      no row      no row
	//	readonly           one row     one row     one row
	//	typeset -r         one row     one row     one row
	//
	// against `typeset -i10 -r PPID=17841` from the named form here, and a
	// row on `readonly -p` besides. The freeze and the integer letter stay:
	// this is the *listing* being told the name writes nothing, not the
	// attributes being taken off, and `readonly`'s own two forms still write
	// their row.
	//
	// **Silent rather than a rule about frozen specials**, which was the
	// shape #4864 was filed with and which the same run disproves:
	// `$keymaps`, `$widgets` and `$zsh_scheduled_events` carry the attribute
	// word `builtins` and `funcstack` carry, byte for byte, and the reference
	// writes the first three and omits the second two. So the answer is not a
	// function of the attributes and there is nothing to derive it from — it
	// is a fact about each name, which is what this seam is for.
	r.SetDynamicDeclaration("PPID", interp.ProducedDeclaration{
		Integer: true, Base: 10, Silent: true,
	})
}

// markTheDirectoryStack is `$dirstack`, whose *behavior* agreed with the
// reference on every row and which said nothing at all about itself.
//
// Measured 2026-09-26 on zsh 5.9.2 under `-f`, in a shell that has pushed
// nothing:
//
//	                       reference                    here, before
//	${(t)dirstack}         array-hide-hideval-special   empty — no such name
//	after a push           array-hide-hideval-special   array
//	set's row              the bare name                dirstack=( /path )
//	typeset -p dirstack    typeset -a dirstack          typeset -a dirstack=…
//
// The three flags are one fact between them: `special` is the shell
// maintaining the parameter, `hideval` is what keeps the values out of a
// listing, and `hide` is what makes a `local dirstack` inside a function an
// ordinary parameter rather than a second view of the stack (#4615).
//
// The **empty array** is what answers the first row, and it is a fact about
// the reference rather than a convenience: real zsh's stack parameter is an
// array from startup, where this shell's arrives with the first push. The
// prelude reads the stack as `${dirstack[@]+"${dirstack[@]}"}` precisely
// because nothing declared it, and that idiom holds either way — see
// dialect/zsh/prelude.go, which says so and names real zsh's answer.
//
// **The issue's fourth row was wrong and is corrected here.** #4615 recorded
// `typeset -p dirstack` as writing "nothing at all" in the reference. It does
// not: measured 2026-09-26 on zsh 5.9.2 under `-f`, both in a fresh shell and
// after a push, it writes `typeset -a dirstack` — the declaration with no
// `=`, which is exactly what `hideval` makes of a name that has a value. So
// the row needs no answer of its own and ProducedDeclaration.Silent, which
// was the obvious way to write "nothing at all", would have been a third
// spelling nobody measured.
func markTheDirectoryStack(r *interp.Runner) {
	r.SetArray("dirstack", nil)
	r.MarkHidden("dirstack")
	r.MarkHideInScope("dirstack")
	r.MarkShellOwnParameter("dirstack")
}

// identityParameters are the four names that say who the process is. They
// carry the integer attribute in the shell being modeled and are *not*
// readonly there, which is where this dialect parts from bash's four.
var identityParameters = [...]string{"UID", "EUID", "GID", "EGID"}
