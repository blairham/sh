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

	// abandonUsage is an error a builtin reported about **how it was
	// called** — an option it does not have, a name it cannot use as one, a
	// count past the end of the positional parameters, a file `.` could not
	// open, a redirection that would not open on a special builtin.
	//
	// It is abandonError everywhere except at the boundary a special
	// builtin draws around text it is running, where one dialect catches
	// every other fatal error and lets this one out. See
	// Semantics.BuiltinUsageErrorEscapesBorrowedText.
	//
	// The line is the *call* and not the builtin: a readonly reassignment
	// `readonly` or `typeset` refuses is raised inside a builtin too, and it
	// is caught like any other error in the same shell. Measured beside each
	// other, which is what makes this a kind of its own rather than a test
	// on Runner.inBuiltin.
	abandonUsage

	// abandonSubstParse is a command substitution whose body would not
	// parse — `v=$(echo hi; for)`.
	//
	// It is abandonError at every boundary this file and source.go draw,
	// and it is a kind of its own for exactly one of them. Measured
	// 2026-09-16 across the panel: at an interactive prompt all seven
	// columns report it and draw the next prompt, at a `.` and an `eval`
	// zsh and ksh93 catch it and carry on where bash and dash end the
	// script, and in a startup file zsh and dash give up the file and run
	// the session — which is abandonError's answer at all three, arrived at
	// without asking anything new.
	//
	// The boundary it is not abandonError at is giveUpTheCommand, where a
	// redirection's own expansion is given up. The columns split there in a
	// way they do not anywhere else: measured, `cat <<END` with such a
	// substitution in the body costs the *command* in bash 5.3.20 and
	// ksh93 — which carry on at 1 and at 3 — and costs the *script* in zsh
	// 5.9.2 and dash, while the target of a redirection, `cat < "$(echo
	// hi; for)"`, costs the script in bash as well and only ksh93 carries
	// on. Two shapes and two different splits, which is why the boundary
	// asks the here-document body alone —
	// Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell — and
	// the target keeps the stop standing in every dialect.
	abandonSubstParse
)

// pendingFileError reports whether what is unwinding is an error a boundary
// giving up one file may catch, rather than a request to end the shell.
func (r *Runner) pendingFileError() bool {
	return r.ctl == controlExit && r.abandon != abandonRequested
}

// stopTheShell raises a controlExit for any reason but `set -e` firing.
//
// One function rather than an assignment at each site, and that is the whole
// of why it exists: Runner.errexitStopped qualifies the controlExit in hand,
// so a raise that forgot to clear it would inherit the last one's answer and
// a try-always block would skip a cleanup half it should run. There are more
// than a dozen places a shell stops from, and "remember to clear the flag"
// at each of them is the shape of rule that gets a new one wrong (#1238).
//
// The sites that also choose an abandonKind write all three fields together
// instead, for the same reason: whichever way it is spelled, nothing sets
// controlExit without saying which producer it is.
func (r *Runner) stopTheShell() {
	r.ctl, r.errexitStopped = controlExit, false
}

// stopTheShellForExit is `exit` raising the controlExit, which is
// stopTheShell plus the one note only this producer can take: whether a file
// was being read when the word ran. See Runner.ExitRanOutsideAFile.
func (r *Runner) stopTheShellForExit() {
	r.exitRan = true
	r.exitRanOutsideAFile = r.sourceDepth == 0
	r.stopTheShell()
}

// ExitRanOutsideAFile reports whether the shell stopped because `exit` ran,
// and ran somewhere other than in a file this shell was reading — a file `.`
// opened, or a startup file.
//
// For a front end, and for one question: one shell in the panel writes a word
// on its way out of an interactive invocation, and *this* is the property
// that decides it. Measured 2026-09-21 on bash 5.3.20 and 3.2.57, `env -i`
// with a scratch HOME and no terminal on any of the three standard streams,
// each probe behind `-i -c`:
//
//	exit 3                              exit
//	f(){ exit 3; }; f                   exit
//	eval exit 3                         exit
//	. defines.sh; the function it made  exit
//	true                                nothing — no `exit` ran
//	set -e; false                       nothing
//	set -u; echo $nope                  nothing
//	(exit 3)                            nothing — a subshell, not this shell
//	. quits.sh, whose own line exits    nothing
//	an rc file whose own line exits     nothing
//
// So it is not "the shell ended", not "the shell ended through `exit`", and
// not where the *text* came from — the last two rows exit through `exit` and
// say nothing, while a function body read from the same sourced file says the
// word when the call is made outside it. What the shell is reading when the
// word runs is the whole of it.
//
// False for every other way a shell stops, which is what the four quiet rows
// above are: the input running out, `set -e` firing, a fatal error, a
// subshell's own exit.
func (r *Runner) ExitRanOutsideAFile() bool {
	return r.ctl == controlExit && r.exitRanOutsideAFile
}

// ExitRan reports whether the shell is stopping because the `exit` builtin
// ran, wherever it ran — in a file this shell was reading as much as outside
// one.
//
// For a front end, and for one question: a login shell reads a logout file on
// its way out, and *this* is what reaches it. Measured 2026-09-22 against bash
// 5.3.20 with a marker in `~/.bash_logout`, each probe behind `-lc`:
//
//	exit 3                              read
//	. f.sh, whose own line exits        read
//	echo x                              nothing -- the input ran out
//	set -e; false                       nothing
//	set -u; echo $nope                  nothing
//	a line that will not parse          nothing
//
// So it is neither "the shell stopped" — the four quiet rows all stop — nor
// ExitRanOutsideAFile, which the second row is not. It is the builtin having
// run, and nothing else.
func (r *Runner) ExitRan() bool { return r.ctl == controlExit && r.exitRan }

// ResumeAfterExit takes back the stop `exit` raised, so a front end can run
// one more file before the shell is finished, and reports whether there was
// such a stop to take back.
//
// For the logout file above and for nothing else. A shell that has run `exit`
// will not execute another command — that is what the stop is for — so the
// file could not otherwise be read at all; and it is the front end that owns
// startup and shutdown files, which is why this is a seam rather than a read
// interp makes for itself.
//
// The status is left exactly as it stood. A logout file that says nothing
// about it leaves the shell exiting with the number `exit` named, which is
// measured; one that runs `exit` of its own raises the stop again and names a
// new number, which is measured too.
func (r *Runner) ResumeAfterExit() bool {
	if r.ctl != controlExit {
		return false
	}
	r.ctl, r.exitRan, r.exitRanOutsideAFile = controlNone, false, false
	return true
}

// takeFileError consumes a caught error, putting the runner back into ordinary
// flow so the file that reached this one carries on at the next command.
//
// Both fields are cleared together. Leaving the kind set would let a later
// `exit` be read as an error by the next boundary up, which is the one way
// this could turn `exit` into something survivable.
func (r *Runner) takeFileError() {
	r.ctl, r.abandon, r.errexitStopped = controlNone, abandonRequested, false
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
	// The shared box first, because a typed line can *end* at the subshell
	// that filled it. `( v=$(echo hi; for) )` is one statement, so the
	// sequence point in Runner.stmt that takes the box is not reached again
	// until the first command of the **next** line — a line the person typed
	// after the prompt came back, which then silently did not run. Measured
	// at a prompt on bash 5.3.20, zsh 5.9.2 and dash: each reports the
	// failure and runs the line after it. Draining it here makes the box
	// what the rest of this file already is, a thing a boundary owns.
	//
	// The status is the box's and not whatever the subshell reported, for
	// the reason scriptStop.status exists: a failure inside a pipeline
	// element is not the pipeline's own status.
	if status, stopped := r.takeScriptStop(); stopped {
		r.status = status
		r.takeFileError()
		return true
	}
	if !r.pendingFileError() {
		return false
	}
	r.takeFileError()
	return true
}

// giveUpTheHook is the same boundary around a *hook chain*, and it is the
// fourth site of the mechanism above: `.` and `eval` are the first, a startup
// file the second, a typed line the third.
//
// A hook is not text the shell was given. `precmd`, `preexec`, `chpwd` and
// bash's `PROMPT_COMMAND` fire between two things a person typed, so an error
// inside one has no line of its own to cost — which is why the shell ended for
// it. Measured through a pseudo-terminal on 2026-09-10, zsh 5.9.2, one
// keystroke at a time, with a `precmd` that raises each fatal expansion in
// turn — a bad substitution, `${x?word}`, an unset name under `NO_UNSET`, a
// division by zero — every one printed `precmd: …`, drew the prompt and
// answered the next line. The rest of the chain did **not** run: with
// `precmd_functions=(p1 p2 p3)` and `p2` failing, `p3` was never called. So
// what an error costs is the chain, and nothing more. bash 5.3.15 answers a
// failing `PROMPT_COMMAND` the same way, which is why the catch is in
// FireChain and not in one of the two callers.
//
// The half that keeps this from making a session nobody can leave is the one
// GiveUpTheLine has: a request to *stop* is not caught. `exit 7` in a `precmd`
// ends the session with 7 and draws no prompt, and so does errexit firing
// inside one, both measured the same way.
//
// **Only at a prompt**, which is the whole of what the Interactive check is
// for and is measured on the other side: `zsh script.zsh` whose `chpwd` raises
// a bad substitution stops at the `cd` and exits 1, and the line after the
// `cd` does not run. A hook is a boundary because a session is, not because a
// hook is.
func (r *Runner) giveUpTheHook() bool {
	if !r.Interactive {
		return false
	}
	return r.GiveUpTheLine()
}

// exitTrapSkippedByAFatalError reports whether this shell is ending over an
// error it reported, with `set -e` on, in the dialect that runs no EXIT trap
// there.
//
// The discriminator is measured and it is neither the error nor the option
// alone. Measured 2026-09-18 on zsh 5.9.2, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, each row written under
// `trap 'echo TRAP_RAN' EXIT` and run twice — once plain and once with
// `set -e` in front:
//
//	row                          plain              under set -e
//	set -Z                       ends, TRAP_RAN     ends, **no** TRAP_RAN
//	readonly r=1; readonly r=2   ends, TRAP_RAN     ends, **no** TRAP_RAN
//	break                        ends, TRAP_RAN     ends, **no** TRAP_RAN
//	set -u; echo "$nosuch"       ends, TRAP_RAN     ends, **no** TRAP_RAN
//	echo $((1/0))                ends, TRAP_RAN     ends, **no** TRAP_RAN
//	false                        runs on, TRAP_RAN  ends, TRAP_RAN
//	exit 3                       ends, TRAP_RAN     ends, TRAP_RAN
//	echo "${nosuch?word}"        ends, TRAP_RAN     ends, TRAP_RAN
//	shift 5                      runs on, TRAP_RAN  ends, TRAP_RAN
//	unset -Z                     runs on, TRAP_RAN  ends, TRAP_RAN
//	cd /nonexistent              runs on, TRAP_RAN  ends, TRAP_RAN
//	: > /nonexistent/x           runs on, TRAP_RAN  ends, TRAP_RAN
//
// So the rows that lose the trap are exactly the ones the shell *reported and
// gave up over* — abandonError and abandonUsage — and never the ones it was
// asked to make. `false` and `exit 3` keep it, which is what says the option
// is not enough on its own; the same rows keep it with the option off, which
// is what says the error is not either. `${x?word}` keeps it in this column
// because that operator is a request to stop here rather than an error — see
// Semantics.ParamErrorIsAnExitRequest — which is a classification this shell
// already had and did not have to be told again.
//
// bash, dash, ksh93 and BusyBox ash run the trap on every row of that table.
func (r *Runner) exitTrapSkippedByAFatalError() bool {
	if !r.errexit {
		return false
	}
	switch r.abandon {
	case abandonError, abandonUsage:
	default:
		return false
	}
	return r.sem().FatalErrorUnderErrexitSkipsTheExitTrap == Yes
}

// subshellExitTrapSkippedByAGiveUp is the same question at the subshell
// boundary, where it has an answer of its own: the option decides nothing
// there and two columns answer it rather than one.
//
// See [SubshellExitTrapPolicy] for the nine-row table across five shells. The
// short of it: bash, dash and BusyBox ash run the trap however the subshell
// ended; ksh93 and zsh take it away from a subshell the shell itself reported
// an error and gave up on; and of those two only zsh also takes it for a
// special builtin's complaint about how it was called.
//
// `${x?word}` is in the skipping set for both, which is why abandonParamError
// is listed beside abandonError rather than left with the requested stops. A
// command substitution whose body would not parse is there too, measured
// 2026-09-18 on `( trap … EXIT; v=`+"`"+`echo hi; for`+"`"+` )`: zsh and ksh93 lose the
// handler and bash keeps it.
// That is the row where this reaches further than the top-level field: the
// same operator keeps the trap at the top of a script in the same shell,
// because it is a request to stop there — see Semantics.ParamErrorIsAnExitRequest.
//
// Read without asking, as the top-level field is: a vector that has chosen
// nothing runs the trap.
func (r *Runner) subshellExitTrapSkippedByAGiveUp() bool {
	if !r.inSubshell {
		return false
	}
	switch r.abandon {
	case abandonError, abandonParamError, abandonSubstParse:
		return r.sem().SubshellExitTrapAfterAGiveUp >= SubshellExitTrapSkippedByAReportedError
	case abandonUsage:
		return r.sem().SubshellExitTrapAfterAGiveUp == SubshellExitTrapSkippedByABuiltinsUsageToo
	}
	return false
}
