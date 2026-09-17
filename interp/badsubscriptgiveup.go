// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// badSubscriptGivesUp writes a complaint about a subscript a builtin could not
// evaluate and then gives up exactly as much as the dialect gives up.
//
// One door for the two sites that ask, because they are the same failure seen
// from two builtins and the three outcomes have to mean the same thing at
// both: see BadSubscriptPolicy for what each one is and where it was measured.
// Before this, `unset` asked a bool that read "fatal" and the store asked
// nothing at all and was fatal unconditionally — so bash ended a script it
// carries on with, and ksh93 ended one it does not stop at all (#3485).
//
// The status is 1 wherever the shell is still running to see it, which is
// unanimous: the complaint leaves a failed builtin behind, and the next
// command reads 1 whether it got there by the builtin returning or by the
// abandoned command's line ending. The two give-up branches take the dialect's
// own fatal status instead, which is the same 1 everywhere it can be observed
// — only dash and BusyBox ash answer that axis differently and neither reaches
// here.
func (r *Runner) badSubscriptGivesUp(p BadSubscriptPolicy, what, sentence string) int {
	if p == BadSubscriptUnspecified {
		// The refusal alone, and not the sentence under it: a shell that was
		// never told what to do here has not decided to complain about the
		// subscript, it has failed to answer a question. Status 2 and the
		// unspecified flag are what every other refused axis leaves, and the
		// caller reads the flag to keep from reporting twice.
		r.diagf("%s\n", r.unanswered(what))
		r.status, r.unspecified = 2, true
		return 2
	}
	r.diagf("%s\n", sentence)
	switch p {
	case BadSubscriptEndsTheScript:
		r.fatalQuiet()
		return r.status
	case BadSubscriptAbandonsTheCommand:
		if r.Route == RouteCommandString {
			// A `-c` string is given up whole rather than resumed at its next
			// command — measured, and the reason is on the constant.
			r.fatalQuiet()
			return r.status
		}
		r.setFatalStatus()
		// controlAbandon rather than controlExit: it unwinds past loops,
		// functions, groups and subshells alike and is consumed at the
		// top-level statement loop, which is precisely the shape measured.
		// abandonLine is what takes the rest of the line with it, so
		// `unset 'q[b c]'; echo x` prints no x and the next *line* runs.
		r.ctl, r.abandonLine = controlAbandon, r.line
		return r.status
	}
	return 1
}
