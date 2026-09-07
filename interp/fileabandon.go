// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A fatal error and a request to stop unwind the shell identically, and they
// are caught in different places.
//
// `exit 3` in a file the shell is reading ends the shell, wherever that file
// came from. An *error* the shell reported ends only the text it happened in,
// in some shells and at some boundaries — measured, a file read by `.` and an
// `eval` argument are both such a boundary in ksh93 and zsh and neither is in
// bash or dash, and a startup file is one in every shell that reads a startup
// file at all.
//
// controlExit carries both, so a boundary that catches one and not the other
// needs to know which it is holding. That is this file: one field beside
// r.ctl, set where a fatal error is raised, read and cleared where a boundary
// gives up a file.

// abandonKind says what raised the controlExit currently unwinding.
type abandonKind uint8

const (
	// abandonRequested is the zero value on purpose: a controlExit nobody
	// annotated is a request to stop, which is the answer that ends the
	// shell and so the safe one for a site that has not thought about it.
	// `exit`, errexit firing and `exec` failing all land here.
	abandonRequested abandonKind = iota

	// abandonError is an error the shell reported and then gave up over —
	// an unset parameter under `set -u`, a readonly assignment a dialect
	// calls fatal, a division by zero, a bad substitution.
	abandonError

	// abandonParamError is `${x?word}` and `${x:?word}` firing, which is
	// abandonError everywhere except that one dialect documents the
	// operator as exiting the shell and behaves that way at a boundary
	// where its own unset-parameter error is caught. See
	// Semantics.ParamErrorIsAnExitRequest.
	abandonParamError
)

// pendingFileError reports whether what is unwinding is an error a boundary
// giving up one file may catch, rather than a request to end the shell.
func (r *Runner) pendingFileError() bool {
	return r.ctl == controlExit && r.abandon != abandonRequested
}

// takeFileError consumes a caught error, putting the runner back into ordinary
// flow so the file that reached this one carries on at the next command.
//
// Both fields are cleared together. Leaving the kind set would let a later
// `exit` be read as an error by the next boundary up, which is the one way
// this could turn `exit` into something survivable.
func (r *Runner) takeFileError() {
	r.ctl, r.abandon = controlNone, abandonRequested
}

// GiveUpTheFile ends the *file* a fatal error happened in rather than the
// shell, reporting whether there was such an error to end.
//
// It is what a front end that reads whole files of its own — a shell's startup
// files — needs in order to behave the way every shell in the panel does with
// one: measured, a `~/.zshenv` or a `$BASH_ENV` whose third line is `echo
// X${NOPE}` under `set -u` stops at that line, the startup files after it are
// still read, and the script or command string the shell was started for still
// runs. Without this the same error ended the session, so one bad line in one
// startup file silently cost a person the whole rest of their invocation.
//
// It deliberately does not catch a request to stop. `exit 3` in a startup file
// exits 3 and the files after it are not read, which is measured in every
// shell that reads more than one, and Exited still answers yes for it.
func (r *Runner) GiveUpTheFile() bool {
	if !r.pendingFileError() {
		return false
	}
	if r.abandon == abandonParamError &&
		r.ask(r.sem().ParamErrorIsAnExitRequest, "`${x?word}` ending the shell rather than the file it is in") {
		// The one operand a dialect reads as a request to stop rather than
		// as an error. Measured at this boundary as well as at a `.`: the
		// same `${NOPE?msg}` at the top of a startup file stops that file
		// and lets the script run in bash, and ends the shell before the
		// script in zsh.
		return false
	}
	r.takeFileError()
	return true
}

// GiveUpTheLine is the same boundary at an interactive prompt: the unit is the
// line a person typed, and an error in it costs that line rather than the
// session.
//
// The third site of one mechanism — `.` and `eval` are the first, a startup
// file the second — and it is a method of its own rather than a flag on
// GiveUpTheFile because **the axis is answered differently here**, which is
// the only thing that separates the two.
//
// `${x?word}` is that axis. A dialect that documents the operand as exiting
// the shell does exit for it in a *script*: measured, `echo X${NOPE?gone}`
// followed by `echo after` prints only the diagnostic in zsh and in dash, and
// prints `after` in bash and ksh93. At a **prompt** all four print the
// diagnostic and draw the next prompt — measured through a pseudo-terminal,
// one keystroke at a time, with a marker the typed line cannot contain — so
// here there is nothing to ask: every error costs the line.
//
// What it still does not catch is a request to *stop*, which is the half that
// keeps this from turning a shell into one nobody can leave. Measured at a
// prompt in all four: `exit`, `exit 7`, `eval 'exit 7'` and errexit firing
// (`set -e` then `false`) each end the session.
func (r *Runner) GiveUpTheLine() bool {
	if !r.pendingFileError() {
		return false
	}
	r.takeFileError()
	return true
}
