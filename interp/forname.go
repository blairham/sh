// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ForNameRunForm is what a `for` or `select` does when it is reached and the
// word standing where its variable belongs is not a name.
//
// A form rather than a flag because there are three answers among the two
// shells that get this far, and the third differs from the second in its
// status alone — which is a difference a bool could not carry. Only the
// dialects with [syntax.Dialect.ForNameCheckedWhenTheLoopRuns] ever ask: the
// other four refuse the word while parsing and never build a clause to run.
//
// Measured 2026-09-06 and re-measured 2026-09-07, `env -i PATH=/usr/bin:/bin`
// with a scratch HOME, over a script file holding `n=x`,
// `for $n in a b; do echo body; done` and `echo "reached-after st=$?"`:
//
//	shell        what a run prints                       script status
//	bash 5.3.15  the complaint, then reached-after st=1   0
//	bash-as-sh   the complaint, and stops                 2
//	bash 3.2.57  the complaint, then reached-after st=1   0
//	ksh93u+      the complaint, and stops                 1
//
// **The bash-as-`sh` row is POSIX mode and not the build**: bash 3.2 and 5.3
// agree under their own names, and `set -o posix; for 1x in a b` in bash 5.3
// stops at 2 exactly as the same binary invoked as `sh` does. So it is a mode
// the shell can enter and leave, which is why it is a value of this axis and
// is swapped by [Runner.SetPosixMode] rather than being a second preset.
//
// **`select` answers the same as `for` in both shells.** #1110 recorded ksh93
// as fatal for `for` and not for `select`; re-measured on 93u+ 2012-08-01
// with stdin closed, `select $n in a b` and `select 1x in a b` both end the
// script at 1 with the line after the loop unreached, exactly as the `for`
// spellings do. bash-as-`sh` stops at 2 for both as well. So the loop keyword
// is not an axis and there are three answers here rather than four.
type ForNameRunForm int

const (
	// ForNameRunUnspecified is no answer, and is refused like any other. It
	// is what a core run reaches, and a core run cannot get here at all
	// without the grammar flag — so reaching it means a dialect turned the
	// flag on and left this unanswered.
	ForNameRunUnspecified ForNameRunForm = iota

	// ForNameFailsTheLoop reports the complaint, gives the *loop* the status
	// in [Diagnostics.ForNameStatus], and lets the script carry on. bash
	// under its own name, 3.2 and 5.3 alike.
	ForNameFailsTheLoop

	// ForNameEndsTheScript reports the complaint and stops, at
	// [Diagnostics.ForNameStatus]. ksh93 — **unless the clause carries a
	// redirection**, and that exception is measured rather than tolerated.
	//
	// `for 1x in a b; do :; done; echo after` ends the script at 1 there and
	// `for 1x in a b; do :; done > mf; echo after` reports the same sentence,
	// prints `after` and exits 0. Any redirection will do — `2>&1` behaves as
	// `> mf` does — and the `select` spelling behaves the same way. It is
	// ksh93's alone: bash-as-`sh` stops at 2 with a redirection and without
	// one, and bash under its own name carries on either way.
	ForNameEndsTheScript

	// ForNameEndsTheScriptAsASyntaxError reports the same complaint and stops
	// at the dialect's *syntax-error* status rather than the refusal's own —
	// 2 for bash, which is what POSIX mode answers. The wording does not
	// change with it, which is measured: bash invoked as `sh` writes
	// ``line 2: `$n': not a valid identifier`` word for word as bash does.
	//
	// No redirection exception here, and that was measured rather than
	// assumed by symmetry with the value above: bash-as-`sh` stops at 2 for
	// `for 1x in a b; do :; done > mf; echo after` too.
	ForNameEndsTheScriptAsASyntaxError
)

func (f ForNameRunForm) String() string {
	switch f {
	case ForNameFailsTheLoop:
		return "ForNameFailsTheLoop"
	case ForNameEndsTheScript:
		return "ForNameEndsTheScript"
	case ForNameEndsTheScriptAsASyntaxError:
		return "ForNameEndsTheScriptAsASyntaxError"
	}
	return "ForNameRunUnspecified"
}

// refuseForName raises the complaint a loop's unusable variable earns when the
// loop is reached, and reports whether the loop should be abandoned.
//
// The wording is the same one the parse-time refusal uses, from
// [Diagnostics.ForName], because it is the same sentence: the shells that
// check the name here and the shells that check it while parsing word it
// identically, and the *stage* is the whole of the difference. Which is why
// there is one field and not two that could drift.
//
// The line the complaint names is the clause's, and nothing here sets it: the
// command dispatcher already did, from this clause's own position, before
// either loop runner was entered. Setting it again was the first version of
// this and it is **removed rather than kept**, because a mutant that replaced
// it with nothing answered every column identically — which is the evidence
// that it decided nothing, and the same shape forNameIsUsable's dead guard
// had.
//
// redirected says the clause carries a redirection, which one dialect's answer
// turns on — the caller knows, and asking it here would mean holding the
// clause rather than the two facts about it.
func (r *Runner) refuseForName(word string, redirected bool) {
	form := r.sem().ForNameWhenTheLoopRuns
	if form == ForNameRunUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a loop variable that is not a name, reached at run time")))
		r.status = 2
		r.unspecified = true
		return
	}
	r.diagf("%s\n", Wording(r.diag().ForName, "expected a name after `for`", word, r.line))
	switch form {
	case ForNameEndsTheScriptAsASyntaxError:
		r.endTheScriptAt(r.diag().SyntaxStatus())
	case ForNameEndsTheScript:
		if redirected {
			// A redirection on the clause makes it not fatal in the one
			// dialect that answers this way. See the constant.
			r.status = r.forNameStatus()
			return
		}
		r.endTheScriptAt(r.forNameStatus())
	default:
		r.status = r.forNameStatus()
	}
}

// endTheScriptAt is fatalQuiet with the status given rather than taken from
// FatalErrorStatusIsOne.
//
// It has to be given, because this refusal is the one place the two numbers
// part: bash's fatal errors are 1 and its POSIX-mode answer here is 2, and
// ksh93's fatal errors are 1 and so is this — so a shared axis would have to
// hold two values for bash at once.
func (r *Runner) endTheScriptAt(status int) {
	r.status = status
	r.ctl, r.abandon = controlExit, abandonError
}

// forNameStatus is the status the refusal itself carries, which both shells
// that report it as a command's failure put at 1 — and which is
// Diagnostics.ForNameStatus so that the run-time route and the parse-time
// route read one number.
func (r *Runner) forNameStatus() int {
	if st := r.diag().ForNameStatus; st != 0 {
		return st
	}
	return 1
}
