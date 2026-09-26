// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Where the two jobs-at-exit sentences are located on a script (#4545).
//
// The name was already right — it is the script's path on the script route
// and the shell's own at a prompt — so what is asked here is the number, and
// the number is three rules rather than one. See
// Diagnostics.JobsAtExitLocatedInAScript for the measured rows.
//
// The last two rows are the pair the arithmetic rests on, and they are the
// ones a single rule gets wrong: with an `exit` in front of it the warning
// moves down a line **only** when that `exit` wrote a sentence first. So it is
// the sentence that costs the line, not the `exit`.
func TestWhereTheJobsAtExitLinesAreLocatedInAScript(t *testing.T) {
	for _, tc := range []struct {
		name         string
		located      bool
		route        Route
		stmtLine     int
		exitRan      bool
		told         bool
		wantSentence string
		wantHangup   string
	}{
		{
			// Running off the end of a two-line script: both at 3.
			name: "off the end of a script", located: true, route: RouteScriptFile,
			stmtLine: 2, wantSentence: "s.sh:3", wantHangup: "s.sh:3",
		},
		{
			// An `exit` on line 3 that wrote a sentence: 3 and then 4.
			name: "an exit that wrote a sentence", located: true, route: RouteScriptFile,
			stmtLine: 3, exitRan: true, told: true,
			wantSentence: "s.sh:3", wantHangup: "s.sh:4",
		},
		{
			// An `exit` on line 4 with the check switched off: the warning
			// lands on the `exit`'s own line.
			name: "an exit that wrote nothing", located: true, route: RouteScriptFile,
			stmtLine: 4, exitRan: true,
			wantSentence: "s.sh:4", wantHangup: "s.sh:4",
		},
		{
			// A prompt: the name alone, which is what this shell wrote on
			// both routes before the change.
			name: "a session", located: true, route: RouteStandardInput,
			stmtLine: 9, wantSentence: "s.sh", wantHangup: "s.sh",
		},
		{
			// CONTROL. A dialect that does not locate these keeps the plain
			// name on the script route too, so the flag is what moves it and
			// not the route on its own.
			name: "a dialect that does not locate them", located: false, route: RouteScriptFile,
			stmtLine: 2, wantSentence: "s.sh", wantHangup: "s.sh",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dg := Diagnostics{JobsAtExitLocatedInAScript: tc.located}
			r := newTestRunner(t, &Runner{Diagnostics: &dg, Name: "s.sh"})
			r.Route = tc.route
			r.lastStmtLine = tc.stmtLine
			r.exitRan = tc.exitRan
			r.toldOfJobsAtExit = tc.told
			if got := r.jobsAtExitName(tc.exitRan); got != tc.wantSentence {
				t.Errorf("the sentence is named %q, want %q", got, tc.wantSentence)
			}
			if got := r.jobsHungUpName(); got != tc.wantHangup {
				t.Errorf("the hangup warning is named %q, want %q", got, tc.wantHangup)
			}
		})
	}
}

// And the line the sentence is located from follows a **background**
// statement.
//
// `Runner.line` is the command dispatcher's, and a `&` statement never reaches
// it here: the clone runs the command and records the line on its own copy. So
// a script whose whole body is `sleep 3 &` left that field at nought and the
// sentence this is the entire point of came out located at line 1, where the
// reference writes 3. This is the guard on the field that fixed it, and the
// `true` in front is the control: without the background statement advancing
// it, the answer would be that line rather than the `&`'s.
func TestTheJobsAtExitLineFollowsABackgroundStatement(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	f, err := syntax.Parse("true\n: &\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(t.Context(), f); err != nil {
		t.Fatal(err)
	}
	if got := r.lastStmtLine; got != 2 {
		t.Errorf("the last statement is line %d, want 2 — the `&` line, not the `true` in front of it", got)
	}
}
