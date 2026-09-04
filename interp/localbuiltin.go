// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// localOutsideAFunction answers `local x=2` written where there is no
// function to be local to, which the panel answers three ways and a fourth
// does not have the question.
//
// bash says so and carries on, and does not set the variable. dash says so
// and stops the script. zsh takes it and sets a global, which reads as the
// most forgiving answer and is the one that hides a misplaced `local` in a
// script written for another shell. ksh93 has no `local` at all, so the word
// is a command that was not found and this is never reached.
//
// Returns the status and whether the builtin is finished.
func (r *Runner) localOutsideAFunction() (int, bool) {
	if !r.ask(r.sem().LocalOutsideAFunctionIsAnError, "`local` outside a function being refused") {
		if r.unspecified {
			return r.status, true
		}
		return 0, false
	}
	if r.unspecified {
		return r.status, true
	}
	r.diagf("%s\n", Wording(r.diag().LocalOutsideAFunction,
		"local: can only be used in a function"))
	if r.ask(r.sem().LocalOutsideAFunctionIsFatal, "that refusal ending the script") {
		if r.unspecified {
			return r.status, true
		}
		r.fatalQuiet()
		return r.status, true
	}
	if r.unspecified {
		return r.status, true
	}
	return 1, true
}
