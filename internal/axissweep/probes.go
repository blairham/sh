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
			Field:   "DebugTrapRefiresOnEnteringAFunction",
			Cases:   []string{"opt/a-debug-trap-fires-twice-for-a-function-call"},
			Reading: "the row runs three commands with the DEBUG trap let into the calls, so it writes three `D` lines where the trap fires once per command and five where a call fires it again on the way in",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/a-debug-trap-fires-twice-for-a-function-call"]
				switch strings.Count(r.Stdout, "D") {
				case 0:
					return "", "no D was written at all, so this shell has no DEBUG condition and the row asks it nothing"
				case 1:
					return "", "one D and no more: the trap never reached inside a call here, so the row cannot say what entering one does"
				case 3:
					return "No", ""
				case 5:
					return "Yes", ""
				}
				return "", "the row wrote a count that is neither one per command nor two per call, so the reading does not fit it"
			},
		},
		{
			Field:   "AliasInvalidNameFatal",
			Cases:   []string{"alias/a-name-holding-a-character-it-may-not-carry"},
			Reading: "the row defines an alias under a name two of the panel will not take, then prints `st=`; a shell that refuses the name and reaches that line is not ended by the refusal, and one that refuses it and never reaches the line is",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["alias/a-name-holding-a-character-it-may-not-carry"]
				switch {
				case strings.TrimSpace(r.Stderr) == "":
					return "", "the name was taken without complaint, so this shell checks no alias name and there is no refusal for the axis to be about"
				case strings.Contains(r.Stdout, "st="):
					return "No", ""
				case strings.Contains(r.Stdout, "one"):
					return "Yes", ""
				}
				return "", "the row did not reach the definition at all, so nothing here is about the refusal"
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
		{
			Field:   "ArithShortCircuitEvaluatesTheRightOperand",
			Cases:   []string{"arith/short-circuit-is-observable"},
			Reading: "`x=0; $((0 && (x=9)))` prints the value and then x, so the second field is 9 in a shell that ran the operand its short circuit had already decided and 0 in one that did not",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["arith/short-circuit-is-observable"]
				// The first field is the operator's value and is 0 under
				// either reading, so only the second can answer. Reading the
				// whole cell would make the two readings look alike.
				switch strings.TrimSpace(r.Stdout) {
				case "[0][9]":
					return "Yes", ""
				case "[0][0]":
					return "No", ""
				}
				return "", "the row printed neither pair, so the expression never reached an assignment"
			},
		},
		{
			Field: "ForHeaderArithmeticErrorIsFatal",
			// Two rows, because one cannot tell the two silences apart. A
			// shell that gave up the input prints no `st=`, and so does one
			// that never had a C-style `for` to give up — dash and ash answer
			// the failing row with a syntax error about the loop variable,
			// which is a refusal of the grammar and not a reading of the
			// header. The clean row is what separates them.
			Cases:   []string{"core/c-style-for", "arith/a-for-header-part-that-will-not-evaluate"},
			Reading: "`for (( i=0; i<1/0; i++ )); do echo body; done; echo \"st=$?\"` prints `st=` in a shell that ends the loop and reads the next line, and nothing at all in one that gives up the input — read only where the clean C-style `for` row shows the shell has the construct",
			Read: func(cells map[string]oracle.Result) (string, string) {
				// Whether the construct exists at all, asked first: the
				// failing row's silence means nothing until this says the
				// shell can run a header that works.
				if clean := cells["core/c-style-for"]; clean.Status != 0 ||
					strings.TrimSpace(clean.Stdout) != "012" {
					return "", "this shell has no C-style `for` — it refuses a header that evaluates cleanly, so the failing row is a syntax error rather than an answer about the failure"
				}
				r := cells["arith/a-for-header-part-that-will-not-evaluate"]
				// The body must not have run under either reading, so `body`
				// in the output means the row measured something other than
				// the header failing.
				if strings.Contains(r.Stdout, "body") {
					return "", "the loop body ran, so the header did not fail and the row says nothing about what a failure does"
				}
				if strings.Contains(r.Stdout, "st=") {
					return "No", ""
				}
				return "Yes", ""
			},
		},
		{
			Field: "BadSetOptionNameFatal",
			Cases: []string{"opt/an-unknown-long-name-is-refused"},
			// `zzznosuch` deliberately, and not the `autocd` row beside it
			// in the corpus. A shell that *has* the name answers with a
			// silent 0 and says nothing about what a refusal would do, and
			// the panel has such a shell — zsh owns `autocd`. Reading that
			// row would score zsh's cell as "carried on, so not fatal",
			// which is the no-evidence-as-agreement shape this file exists
			// to refuse. A name nobody owns makes all seven columns speak;
			// the empty-stderr guard below is what keeps that a check rather
			// than a claim about the snippet.
			Reading: "`set -o zzznosuch; echo \"on=$?\"` reaches its `echo` in a shell a refused `set -o` name does not end, and prints nothing at all in one it does",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return refusedSetOptionFatal(cells["opt/an-unknown-long-name-is-refused"], "on=")
			},
		},
		{
			Field: "BadSetOptionNameAtInvocationExitsZero",
			Cases: []string{"opt/an-unknown-long-name-at-an-invocation"},
			// The one axis here whose row is an *invocation* rather than a
			// snippet, and it needs the same control for the same reason: a
			// shell that owned `zzznosuch` would run `echo hi` and leave at
			// 0, which is the exact pair of observations a Yes is. The
			// stderr guard and the `hi` guard together are what tell the two
			// zeros apart.
			Reading: "`<shell> -o zzznosuch -c 'echo hi'` refuses the name and runs nothing in all seven; the status it leaves with is 0 in the shell where declining is not a failure, and nonzero in the rest",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/an-unknown-long-name-at-an-invocation"]
				if strings.TrimSpace(r.Stderr) == "" {
					return "", "this shell refused nothing — the recorded cell holds no refusal, so the row says nothing about what one would report"
				}
				if strings.Contains(r.Stdout, "hi") {
					return "", "the command string ran, so the shell did not decline and the row is not about a refusal"
				}
				if r.Status == 0 {
					return "Yes", ""
				}
				return "No", ""
			},
		},
		{
			Field: "SetOLetterAttachesItsName",
			Cases: []string{"opt/a-set-o-name-welded-to-the-letter"},
			// The welded characters spell `errexit` — a name every column
			// has — so that the two readings part on *behavior* rather than
			// on wording. A name nobody owns would be refused under both,
			// and the row could then be read only by comparing four
			// different complaints.
			//
			// The empty-stderr control is the same one the two axes below
			// carry, pointing the other way: a cell that said nothing and
			// did not turn errexit on has not answered the question, and
			// scoring it `No` would make silence agree with whatever the
			// preset already held.
			Reading: "`set -oerrexit zzznosuch; …; case $- in *e*)` leaves errexit **on** and says nothing in a shell that reads `errexit` as the name, and refuses `zzznosuch` as the name with errexit still off in one that gives `-o` the next word instead",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/a-set-o-name-welded-to-the-letter"]
				on := strings.Contains(r.Stdout, "e=on")
				if strings.TrimSpace(r.Stderr) == "" {
					if on {
						return "Yes", ""
					}
					return "", "this shell refused nothing and turned errexit on for nothing — the recorded cell holds neither reading"
				}
				if on {
					return "", "the cell holds a refusal *and* errexit on, which neither reading produces"
				}
				return "No", ""
			},
		},
		{
			Field: "SetValidatesOptionLettersFirst",
			Cases: []string{"opt/a-bad-letter-behind-a-good-one"},
			// Only the two columns that survive a refused letter can answer
			// this from a recorded cell at all, and that is the honest state
			// of it rather than a gap to paper over: in the other five the
			// refusal ends the script before it can say what was applied, so
			// the record holds a fatality and not an answer. They were asked
			// with `command set` in front, which is a probe and not a row.
			//
			// Two controls, and both are the #2645 one pointing at this row's
			// two ways of holding nothing. A cell with empty standard error
			// refused nothing, so it says nothing about what a refusal
			// leaves behind; and a cell whose standard output never reaches
			// the `u=` word was ended by the refusal, which is the other
			// axis's answer and not this one's. Scoring either `No` would
			// make silence agree with whatever the preset already held.
			Reading: "`set -u -q; …; case $- in *u*)` leaves nounset **off** in a shell that reads every option word's letters before applying one, and **on** in a shell that applies them as it goes",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/a-bad-letter-behind-a-good-one"]
				if strings.TrimSpace(r.Stderr) == "" {
					return "", "this shell refused nothing — the recorded cell holds no refusal, so the row says nothing about what one leaves applied"
				}
				switch {
				case strings.Contains(r.Stdout, "u=off"):
					return "Yes", ""
				case strings.Contains(r.Stdout, "u=on"):
					return "No", ""
				}
				return "", "the refusal ended the script before it could say what had been applied — the cell holds a fatality rather than an answer"
			},
		},
		{
			Field: "BadSetOptionLetterFatal",
			Cases: []string{"opt/an-unknown-letter-is-refused"},
			// The mirror, and the reason the two are separate axes at all:
			// BusyBox ash answers `No` above and `Yes` here. The row is a
			// letter no panel member has, for the same reason the row above
			// is a name none of them has.
			Reading: "`set -q; echo \"st=$?\"; echo alive` reaches its `echo`s in a shell a refused option letter does not end, and prints nothing at all in one it does",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return refusedSetOptionFatal(cells["opt/an-unknown-letter-is-refused"], "st=")
			},
		},
		{
			Field: "WaitRemembersAReapedJob",
			Cases: []string{"axis/wait-remembers-a-reaped-job"},
			// The first status is the control and is why this row cannot be
			// read from the second line alone. A column that could not
			// resolve `%1` never reaped anything, so its second `wait` is
			// about a job the shell still holds rather than about a memory of
			// one — and scoring that as `No` would count a shell that never
			// reached the question as one that answered it.
			Reading: "the row waits for a job by name and then for the same job by process id: `again=7` is a shell that still has the status of a job it has reported, and anything else is one that has let it go",
			Read: func(cells map[string]oracle.Result) (string, string) {
				out := cells["axis/wait-remembers-a-reaped-job"].Stdout
				if !strings.Contains(out, "first=7") {
					return "", "the wait by name did not report the job's own status, so this shell never reaped a job here and the second wait is not about a memory of one"
				}
				switch {
				case strings.Contains(out, "again=7"):
					return "Yes", ""
				case strings.Contains(out, "again="):
					return "No", ""
				}
				return "", "the row did not reach its second wait at all"
			},
		},
	}
}

// refusedSetOptionFatal is the reading both `set` refusal axes take, given
// the cell and the word its row prints after the refusal.
//
// Folded rather than written twice, because the two differ in nothing but
// which row they read and a fix landing in one of a pair is this tree's
// recurring failure. The guard is the load-bearing part: a cell with nothing
// on standard error is a shell that **accepted** the option, and a shell that
// accepted it has not been asked the question. Without that, "carried on
// because there was no refusal" and "carried on past a refusal" are one
// reading, and the first is scored as agreement.
func refusedSetOptionFatal(r oracle.Result, printed string) (string, string) {
	if strings.TrimSpace(r.Stderr) == "" {
		return "", "this shell accepted the option — the recorded cell holds no refusal at all, so the row says nothing about what a refusal would do"
	}
	switch {
	case strings.Contains(r.Stdout, printed):
		return "No", ""
	case r.Status != 0:
		return "Yes", ""
	}
	return "", "the script neither reached the `echo` after the refusal nor failed, so the row says nothing about fatality"
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
