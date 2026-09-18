// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// abandonTheCommand is the one door to controlAbandon: the command being run
// is given up, the rest of the *input line* goes with it, and the shell picks
// up at the next line.
//
// One door rather than the ten assignments it replaces, because every one of
// them had the same bug and it could only be fixed in one place: they wrote
// `r.abandonLine = r.line`, the line the **failing command** was written on,
// and the statement loop skips the statements that share it. Those two lines
// are the same number only when the failure is at the top level, which is
// where every one of the ten was measured. Inside a function body they are
// different, and bash gives up the line that **called** the function rather
// than the line inside it (#3503).
//
// The status is the caller's to set. Some of these refusals carry the
// dialect's fatal status and some carry a 1 of their own, and which is which
// is measured per site; this sets the control flow alone.
func (r *Runner) abandonTheCommand() {
	r.ctl, r.abandonLine = controlAbandon, r.giveUpLine()
}

// giveUpLine is how much of the input a give-up takes with it: the last line
// of the top-level statement the shell is currently running.
//
// Not r.line, which is the line of whatever failed. Measured 2026-09-17,
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, script files, one shape per row, with
// `unset 'a[b c]'` as the failure and `; echo "same=$?"` behind the *caller*:
//
//	                                        bash 5.3  bash 3.2  here before
//	unset …; echo same                      no same   no same   no same
//	f(){ unset …; }; f; echo same           no same   no same   same=1
//	f(){ unset …; }; echo pre; f; echo same no same   no same   same=1
//	if true; then f; fi; echo same          no same   no same   same=1
//	g(){ unset …; }; f(){ g; }; f; echo s.  no same   no same   same=1
//	( f; echo insub ); echo same            same=1    same=1    same=1
//
// The same six rows with a reassignment to a readonly name and with
// `echo "$((1/0))"` in place of the `unset` answer identically, so this is
// the give-up mechanism and not one site's mistake.
//
// The complaint's own location is **not** this line: bash writes `line 3` for
// a failure three lines inside a function body while giving up the line 6 the
// call was on, so r.line stays what the diagnostic reports and only the
// give-up moves.
//
// A subshell is the row that does not move, and it needs nothing here: the
// parentheses run in a copy of this runner, so the give-up ends the copy and
// the parent never sees it. A function body is the case this exists for,
// because it runs in *this* runner with no statement loop of its own.
//
// Borrowed text — an `eval` or a `.` — has a loop of its own and is a file as
// far as giving up a line goes, which was already measured and already right:
// see the loop in interp/source.go, which sets this field for its own
// statements and puts the caller's back afterwards.
func (r *Runner) giveUpLine() int {
	if r.inputLine != 0 {
		return r.inputLine
	}
	// Nothing is running at input level — a trap body, or a caller driving
	// Runner.stmt directly — so the failing line is the best answer there is,
	// which is what this did everywhere before.
	return r.line
}

// inputLineOf is the last line of one top-level statement, which is the line
// a give-up inside it gives up.
//
// The statement's own `;` counts, and that is not a detail: a statement
// continued with a backslash ends on an earlier line than the `;` that
// separates it from the next one, and the two of them are still one input
// line. Measured — `unset 'a[b c]' \` then `  ; echo "same=$?"` prints no
// `same` in bash 5.3.20 and 3.2.57, where taking only the words' last line
// would have run it. The same holds at the other end for a multi-line
// compound: `if true; then` … `fi; echo "same=$?"` gives the `echo` up too.
func (r *Runner) inputLineOf(st *syntax.Stmt) int {
	line := r.lineOf(st.End())
	if st.Semi.IsValid() {
		if l := r.lineOf(st.Semi); l > line {
			line = l
		}
	}
	return line
}

// giveUpForABadSubscript is abandonTheCommand plus the one rule that a
// give-up over an unevaluable **subscript** carries and no other give-up
// does: a `-c` string is given up whole rather than resumed at its next
// command.
//
// #3494 measured that rule and wrote it at the two builtin-operand sites —
// `unset 'a[b c]'` and the store a `read` or `printf -v` operand walks into.
// It belongs to every subscript, and the sites that expand one never got it,
// so `unset 'a[b c]'` and `a[b c]=v` — the same expression, one builtin apart
// — disagreed under `-c` (#3502).
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C`, each program run
// as a script file and as one `-c` string, `a=(1 2 3)` first and
// `echo "next=$?"; echo end` behind the failure on its own lines:
//
//	                        bash 5.3 file  bash 5.3 -c   bash 3.2 -c
//	a[b c]=v                next=1 end 0   nothing, 1    nothing, 1
//	a[b c]+=v               next=1 end 0   nothing, 1    nothing, 1
//	a=([b c]=v)             next=1 end 0   nothing, 1    n/a
//	${a[b c]}               next=1 end 0   nothing, 1    nothing, 1
//	${#a[b c]}              next=1 end 0   nothing, 1    nothing, 1
//	$(( a[b c]++ ))         next=1 end 0   nothing, 1    nothing, 1
//	a[1/0]=v                next=1 end 0   nothing, 1    nothing, 1
//
// **And it is a rule about subscripts and not about failed expansions**,
// which is the half #3502 had the other way round. The same arithmetic that
// ends a `-c` string inside brackets runs the next command outside them, in
// both builds:
//
//	                        bash 5.3 file  bash 5.3 -c
//	echo "$((1/0))"         next=1 end 0   next=1 end 0
//	echo "$((b c))"         next=1 end 0   next=1 end 0
//	echo "${v:b c:2}"       next=1 end 0   next=1 end 0
//	readonly r=1; r=2       next=1 end 0   next=1 end 0
//
// So this is a door of its own rather than a clause in failedExpansion: the
// bracketed spellings come through here and the bare ones do not. The rest of
// the panel cannot be asked — dash, ksh93, zsh and BusyBox ash end the script
// at all of these by both routes, which is what makes the rule the abandoning
// column's own rather than an axis.
//
// The status is the dialect's fatal one, 1 in bash, and not the 127 a failed
// expansion under `-c` carries there: measured, `bash -c 'a=(1 2 3)
// a[b c]=v'` exits 1 where `bash -c 'set -u; echo $NOPE'` exits 127 — see
// Diagnostics.ExpansionFailureStatusFromCommandString for the other half of
// that split.
func (r *Runner) giveUpForABadSubscript() {
	if r.Route == RouteCommandString {
		r.fatalQuiet()
		return
	}
	r.setFatalStatus()
	// controlAbandon rather than controlExit: it unwinds past loops,
	// functions, groups and subshells alike and is consumed at the top-level
	// statement loop, which is precisely the shape measured.
	r.abandonTheCommand()
}
