// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"strings"
	"syscall"

	"github.com/blairham/sh/internal/oracle"
)

// The probes: every axis this tree can read off the golden record today.
//
// Each one names the corpus rows it reads and says, in code, what the
// recorded cells are taken to show. Two rules keep the list honest, and both
// are checked rather than agreed:
//
//   - **A reading never names a shell.** It is handed one column's cells and
//     answers from them, so bash, zsh, ksh, dash and ash are graded by the
//     same sentence and a dialect added tomorrow is graded by it too. A
//     reading with a `switch dialect` in it would be a second copy of the
//     presets, grading them against themselves.
//   - **Silence is a value.** Where the recorded cells cannot show the axis —
//     the shell has no arrays, the row asks it nothing, the construct is a
//     command it does not have — the reading says so in words. That pair goes
//     into the ledger as `-`, and the count of them is printed. It must never
//     be possible to read "the record could not say" as "the preset agrees".
//
// Adding a probe is the way this instrument grows, and the bar for one is a
// recorded cell that *discriminates*: TestEveryProbeDiscriminates requires
// each reading to answer two different ways across the seven panel columns,
// because a reading that answers the same thing for every shell is not
// reading anything.

// Probes are the axes the golden record can grade today.
func Probes() []Probe {
	return []Probe{
		{
			Field:   "ArrayBaseIsZero",
			Cases:   []string{"axis/array-base"},
			Reading: "`a=(x y); echo \"${a[1]}\"` prints the second element where subscripts start at zero and the first where they start at one",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/array-base"]
				switch {
				case r.Status != 0:
					return "", "this shell has no array literal — the recorded cell is a syntax error, so the row asks it nothing about a base"
				case strings.TrimSpace(r.Stdout) == "y":
					return "Yes", ""
				case strings.TrimSpace(r.Stdout) == "x":
					return "No", ""
				}
				return "", "neither element came back, so the row did not reach a subscript"
			},
		},
		{
			Field:   "EchoInterpretsEscapes",
			Cases:   []string{"axis/echo-backslash"},
			Reading: "the row compares `echo 'a\\tb'` against its own text and prints `expanded` where the escape was taken",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch strings.TrimSpace(cells["axis/echo-backslash"].Stdout) {
				case "expanded":
					return "Yes", ""
				case "literal":
					return "No", ""
				}
				return "", "the row printed neither word, so it did not reach the comparison"
			},
		},
		{
			Field:   "LastPipelineElementInCurrentShell",
			Cases:   []string{"axis/pipeline-last-element"},
			Reading: "`echo x | read v` leaves `v` set in a shell that runs the last element itself and unset in one that gives it a subshell",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch strings.TrimSpace(cells["axis/pipeline-last-element"].Stdout) {
				case "[x]":
					return "Yes", ""
				case "[]":
					return "No", ""
				}
				return "", "the row printed neither `[x]` nor `[]`, so `read` never ran"
			},
		},
		{
			Field:   "DollarZeroNamesTheInnermostCall",
			Cases:   []string{"axis/dollar-zero-in-function"},
			Reading: "`f() { echo \"$0\"; }; f` prints the function's name where `$0` follows the innermost call and the shell's where it does not",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/dollar-zero-in-function"]
				if r.Status != 0 || strings.TrimSpace(r.Stdout) == "" {
					return "", "the function never ran, so nothing named anything"
				}
				if strings.TrimSpace(r.Stdout) == "f" {
					return "Yes", ""
				}
				return "No", ""
			},
		},
		{
			Field:   "ShiftPastEndFatal",
			Cases:   []string{"axis/shift-past-end"},
			Reading: "`shift 5; echo survived` reaches the `echo` in a shell the overshoot does not end, whatever it says about it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/shift-past-end"]
				switch {
				case strings.Contains(r.Stdout, "survived"):
					return "No", ""
				case r.Status != 0:
					return "Yes", ""
				}
				return "", "the shell neither carried on nor failed, so the row says nothing about fatality"
			},
		},
		{
			Field:   "LocalOutsideAFunctionIsAnError",
			Cases:   []string{"axis/local-outside-a-function"},
			Reading: "`local x=2` at the top level of a script is refused in a shell that has the builtin and answers the question, and silently sets a global in one that does not",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/local-outside-a-function"]
				if noSuchCommand(r) {
					return "", "this shell has no `local` builtin at all — the recorded cell is a command search failing, which is not an answer about the builtin"
				}
				if strings.TrimSpace(r.Stderr) == "" {
					return "No", ""
				}
				return "Yes", ""
			},
		},
		{
			Field:   "LocalOutsideAFunctionIsFatal",
			Cases:   []string{"axis/local-outside-a-function"},
			Reading: "the same row reaches its last line, `echo end`, in a shell the refusal does not end",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/local-outside-a-function"]
				if noSuchCommand(r) {
					return "", "this shell has no `local` builtin at all, so there is no refusal for the row to be fatal about"
				}
				switch {
				case strings.Contains(r.Stdout, "end"):
					return "No", ""
				case r.Status != 0:
					return "Yes", ""
				}
				return "", "the script neither reached its end nor failed, so the row says nothing about fatality"
			},
		},
		{
			Field:   "QuitIgnoredWhenNotInteractive",
			Cases:   []string{"signal-death/quit-is-not-fatal-in-every-shell"},
			Reading: "`kill -QUIT $$; echo after` prints `after` in a shell that was born ignoring the signal and dies of SIGQUIT in one that was not",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return survivesQuit(cells["signal-death/quit-is-not-fatal-in-every-shell"])
			},
		},
		{
			Field: "QuitResetRestoresTheDefault",
			Cases: []string{
				"signal-death/quit-is-not-fatal-in-every-shell",
				"signal-death/a-reset-takes-the-ignore-away-in-one-shell",
			},
			Reading: "after `trap - QUIT`, a shell that had been ignoring SIGQUIT either keeps the ignore and prints `after` or is handed the default action and dies of it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				// Only a shell that was ignoring SIGQUIT in the first place
				// has an ignore for the reset to take away. One that dies of
				// an untrapped QUIT anyway answers this row identically
				// whichever reading is taken, which is exactly why the
				// dialects for those columns leave the axis unanswered.
				if ignores, _ := survivesQuit(cells["signal-death/quit-is-not-fatal-in-every-shell"]); ignores != "Yes" {
					return "", "this shell dies of an untrapped SIGQUIT anyway, so there is no ignore for a reset to take away and the row asks it nothing"
				}
				r := cells["signal-death/a-reset-takes-the-ignore-away-in-one-shell"]
				switch {
				case strings.Contains(r.Stdout, "after"):
					return "No", ""
				case r.Signal == syscall.SIGQUIT:
					return "Yes", ""
				}
				return "", "the shell neither carried on nor died of SIGQUIT after the reset"
			},
		},
		{
			Field:   "HangupIsAnOrderlyExit",
			Cases:   []string{"signal-death/hangup-is-an-exit-in-one-shell"},
			Reading: "`kill -HUP $$` leaves a shell that treats the signal as an exit reporting a status, where every other one dies of SIGHUP and reports no status at all",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["signal-death/hangup-is-an-exit-in-one-shell"]
				switch {
				case r.Signal == syscall.SIGHUP:
					return "No", ""
				case r.Status > 0:
					return "Yes", ""
				}
				return "", "the shell neither died of SIGHUP nor exited nonzero, so the row says nothing about which of the two it did"
			},
		},
		{
			Field:   "ExitTrapRunsOnSignalDeath",
			Cases:   []string{"kill/exit-trap-after-a-fatal-signal"},
			Reading: "an EXIT trap set before an untrapped fatal signal prints its `bye` in a shell that counts dying as exiting",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["kill/exit-trap-after-a-fatal-signal"]
				if r.Signal != syscall.SIGINT {
					return "", "this shell did not die of the signal at all, so the row cannot say whether dying would have run the trap"
				}
				if strings.Contains(r.Stdout, "bye") {
					return "Yes", ""
				}
				return "No", ""
			},
		},
		{
			Field:   "FatalErrorStatusIsOne",
			Cases:   []string{"axis/arith-error-status"},
			Reading: "a division by zero is a fatal shell error, and the status it leaves is the status every fatal error in that shell leaves",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch cells["axis/arith-error-status"].Status {
				case 1:
					return "Yes", ""
				case 2:
					return "No", ""
				}
				return "", "the shell left neither 1 nor 2 behind, so this row is measuring something other than the fatal-error status"
			},
		},
		{
			Field:   "FailedExpansionAbandonsTheLine",
			Cases:   []string{"axis/failed-expansion-abandons-the-line"},
			Reading: "a failed expansion on the middle line of a script either gives up that line and carries on to `after`, or ends the shell there",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/failed-expansion-abandons-the-line"]
				switch {
				case strings.Contains(r.Stdout, "after") && r.Status == 0:
					return "Yes", ""
				case !strings.Contains(r.Stdout, "after") && r.Status != 0:
					return "No", ""
				}
				return "", "the shell neither carried on at status 0 nor stopped nonzero, so the row is not asking this axis here"
			},
		},
		{
			Field:   "ReadonlyReassignmentFatal",
			Cases:   []string{"axis/readonly-reassign"},
			Reading: "assigning to a readonly name from a script either ends the shell or leaves it to reach `echo survived`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/readonly-reassign"]
				switch {
				case strings.Contains(r.Stdout, "survived"):
					return "No", ""
				case r.Status != 0:
					return "Yes", ""
				}
				return "", "the shell neither carried on nor failed, so the row says nothing about fatality"
			},
		},
		{
			Field:   "AssignmentPrefixPersistsAfterAFunction",
			Cases:   []string{"axis/whether-a-prefix-to-a-function-persists"},
			Reading: "`v=9 f` leaves `v` holding 9 in a shell where the prefix persists and 1 in one that takes it back",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch strings.TrimSpace(cells["axis/whether-a-prefix-to-a-function-persists"].Stdout) {
				case "[9]":
					return "Yes", ""
				case "[1]":
					return "No", ""
				}
				return "", "the row printed neither value, so the call never happened"
			},
		},
		{
			Field:   "PrefixToAFunctionIsExported",
			Cases:   []string{"axis/whether-a-prefix-to-a-function-is-exported"},
			Reading: "the function asks `export -p` about the name its own prefix set, and answers YES where the prefix carried the export attribute in",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch strings.TrimSpace(cells["axis/whether-a-prefix-to-a-function-is-exported"].Stdout) {
				case "YES":
					return "Yes", ""
				case "NO":
					return "No", ""
				}
				return "", "the function printed neither word, so it never ran"
			},
		},
	}
}

// survivesQuit is the reading of the plain SIGQUIT row, shared because a
// second axis is asked only where this one says the shell survives.
//
// Folded rather than copied: a second helper reading the same row is how a
// fix lands in one of them and not the other, which has happened here often
// enough to be a rule.
func survivesQuit(r oracle.Result) (string, string) {
	switch {
	case r.Signal == syscall.SIGQUIT:
		return "No", ""
	case strings.Contains(r.Stdout, "after") && r.Status == 0:
		return "Yes", ""
	}
	return "", "the shell neither died of SIGQUIT nor carried on from it, so the row says nothing about the disposition"
}

// noSuchCommand reports that the recorded cell is a command search failing
// rather than a builtin answering.
//
// A shell without `local` reaches every row about it through PATH, and the
// resulting `not found` is not that shell's answer to anything. Reading it as
// one is the shape this whole file exists to prevent, one level down.
func noSuchCommand(r oracle.Result) bool {
	return strings.Contains(r.Stderr, "not found")
}
