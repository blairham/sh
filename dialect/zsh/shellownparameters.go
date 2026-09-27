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
	r.MarkShellOwnParameter("IFS")
	for _, pair := range builtInTies {
		r.MarkShellOwnParameter(pair[0])
		r.MarkShellOwnParameter(pair[1])
	}
	markTheDirectoryStack(r)
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
