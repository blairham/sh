// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `command -v` given several names is two questions with two answers, and
// this engine had one answer to both — the one two columns of five give.
//
// How many names are read at all is Semantics.CommandReportsEveryOperand, and
// what the status is when the answers are mixed is
// Semantics.CommandCountsAMissingOperand. They are separate because the
// output is identical either way: the same one line of `echo` comes back
// whether the status is 0 or 1.
func TestCommandVOverSeveralNames(t *testing.T) {
	for _, c := range []struct {
		name          string
		every, counts Answer
		src, want     string
		status        int
	}{
		{
			"every operand is answered",
			Yes, No,
			`command -v echo shift`, "echo\nshift\n", 0,
		},
		{
			"and the first alone where the dialect stops there",
			No, No,
			`command -v echo shift`, "echo\n", 0,
		},
		{
			"a name it missed decides the status where the dialect counts it",
			Yes, Yes,
			`command -v echo nosuchthing_zz`, "echo\n", 1,
		},
		{
			"and a name it found decides it where the dialect does not",
			Yes, No,
			`command -v echo nosuchthing_zz`, "echo\n", 0,
		},
		{
			"the order of the two does not matter either way",
			Yes, Yes,
			`command -v nosuchthing_zz echo`, "echo\n", 1,
		},
		{
			"every name missing is a failure however it is counted",
			Yes, No,
			`command -v nosuch1_zz nosuch2_zz`, "", 1,
		},
		{
			"every name found is a success however it is counted",
			Yes, Yes,
			`command -v echo shift`, "echo\nshift\n", 0,
		},
		// `-V` is the same two questions with the sentence in place of the
		// name, which is what says this is about the operands and not about
		// the letter.
		{
			"the sentence form asks the same two questions",
			Yes, No,
			`command -V echo shift`, "echo is a shell builtin\nshift is a special shell builtin\n", 0,
		},
		{
			"and stops at the first where the dialect does",
			No, No,
			`command -V echo shift`, "echo is a shell builtin\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			setup := func(r *Runner) {
				sem := *r.Semantics
				sem.CommandReportsEveryOperand = c.every
				sem.CommandCountsAMissingOperand = c.counts
				r.Semantics = &sem
			}
			out, st := run(t, c.src, setup)
			if out != c.want || st != c.status {
				t.Errorf("%s =\n%q at %d\nwant\n%q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// A `command` reached through an expansion loses the power to run what it
// names in one dialect, and reports instead — see
// Semantics.ExpandedCommandOnlyReports.
//
// Both answers over the same snippets. The written rows are the control and
// answer the same either way, which is what says the axis is about the word
// and not about the builtin.
func TestAnExpandedCommandReportsRatherThanRuns(t *testing.T) {
	for _, c := range []struct {
		name, src string

		reporting       string
		reportingStatus int
		running         string
		runningStatus   int
	}{
		{
			"an expanded command names what it was given",
			`c=command; $c echo hi`,
			"echo\n", 1, "hi\n", 0,
		},
		{
			"however the expansion was written",
			`c=command; "$c" echo hi`,
			"echo\n", 1, "hi\n", 0,
		},
		{
			"and through a substitution too",
			`$(echo command) echo hi`,
			"echo\n", 1, "hi\n", 0,
		},
		// The controls: a written word runs in both answers, and quoting
		// does not make it an expansion.
		{
			"a written command runs",
			`command echo hi`,
			"hi\n", 0, "hi\n", 0,
		},
		{
			"and a quoted one is still written",
			`"command" echo hi`,
			"hi\n", 0, "hi\n", 0,
		},
		// A written reporting letter wins, so the axis is a default for the
		// letters rather than a refusal of the run.
		{
			"a written -v is the same either way",
			`c=command; $c -v echo`,
			"echo\n", 0, "echo\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, tc := range []struct {
				answer Answer
				want   string
				status int
			}{
				{Yes, c.reporting, c.reportingStatus},
				{No, c.running, c.runningStatus},
			} {
				setup := func(r *Runner) {
					sem := *r.Semantics
					sem.ExpandedCommandOnlyReports = tc.answer
					sem.CommandReportsEveryOperand = Yes
					sem.CommandCountsAMissingOperand = Yes
					r.Semantics = &sem
				}
				out, st := run(t, c.src, setup)
				if out != tc.want || st != tc.status {
					t.Errorf("answer %v: %s =\n%q at %d\nwant\n%q at %d", tc.answer, c.src, out, st, tc.want, tc.status)
				}
			}
		})
	}
}
