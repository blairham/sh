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
			Field: "ArithFloatOverflowIsZero",
			Cases: []string{"axis/arith-float-overflow"},
			// Read off the row's **first** line, which is the numeral. The
			// third line is in the case as a control and must not be read
			// here: it is `inf` in both columns, so a reading taken from it
			// would grade every float shell alike and answer nothing.
			Reading: "`echo $((1e400))` is an infinity in a shell that saturates a numeral too large for a double and a zero in one that loses it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/arith-float-overflow"]
				first, _, _ := strings.Cut(strings.TrimSpace(r.Stdout), "~")
				switch {
				case r.Status != 0 && first == "":
					return "", "this shell has no floats — the recorded cell is a refusal of the word `1e400` while reading it, so nothing here says what it would have done with a value out of range"
				case strings.Contains(strings.ToLower(first), "inf"):
					return "No", ""
				case strings.Trim(first, "-+") == "0":
					return "Yes", ""
				}
				return "", "the first line was neither an infinity nor a zero, so the row did not reach the numeral"
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
			Field: "DollarZeroNames",
			Cases: []string{"axis/dollar-zero-in-function", "cmd/function-keyword-with-several-names"},
			Reading: "`f() { echo \"$0\"; }; f` prints the function's name where `$0` follows every call; " +
				"where it does not, `function a b { echo \"[$0]\"; }; a` still prints `[a]` in the shell " +
				"whose `function` keyword is what carries `$0`, and the shell's own name in the ones that never move it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				plain := cells["axis/dollar-zero-in-function"]
				if plain.Status != 0 || strings.TrimSpace(plain.Stdout) == "" {
					return "", "the function never ran, so nothing named anything"
				}
				if strings.TrimSpace(plain.Stdout) == "f" {
					return "DollarZeroIsTheInnermostCall", ""
				}
				// The keyword row is asked only once the parenthesis
				// spelling has said no, so a shell that moves `$0`
				// everywhere is never read off a row about the keyword.
				if strings.Contains(cells["cmd/function-keyword-with-several-names"].Stdout, "[a]") {
					return "DollarZeroIsTheInnermostKeywordFunction", ""
				}
				return "DollarZeroIsTheShellsOwnName", ""
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
			Field:   "ExecFailureRunsExitTrap",
			Cases:   []string{"exec/failed-exec-trap-diverges"},
			Reading: "an EXIT trap set before an `exec` of a bare name the PATH search had nothing for prints its `TRAP` in a shell that still runs the handler",
			Read: func(cells map[string]oracle.Result) (string, string) {
				// Ungraded until #3983, which is half of why the axis went
				// on saying "true in dash and bash" while bash disagreed
				// with it on four failures out of five. A pair nothing
				// grades reads exactly like a pair that agrees.
				r := cells["exec/failed-exec-trap-diverges"]
				if strings.Contains(r.Stdout, "TRAP") {
					return "Yes", ""
				}
				return "No", ""
			},
		},
		{
			Field: "ExecFailureOnAPathnameRunsExitTrap",
			Cases: []string{
				"exec/failed-exec-on-a-pathname-trap-diverges",
				"exec/failed-exec-on-a-file-found-on-path-trap-diverges",
			},
			Reading: "an EXIT trap set before an `exec` that had a *file* in hand and could not start it prints its `TRAP` in a shell that still runs the handler — read from the two roads to a pathname together, an operand with a slash and a name the PATH search resolved, which must agree",
			Read: func(cells map[string]oracle.Result) (string, string) {
				// Both rows or neither. They are the same question asked by
				// the two ways a pathname can be arrived at, and a column
				// that answered them differently would mean the axis is
				// about the *slash* after all rather than about the
				// pathname — which is the reading #3983 was filed with and
				// the second row exists to rule out. Saying so is better
				// than picking one and grading a preset against half a
				// disagreement.
				slash := strings.Contains(cells["exec/failed-exec-on-a-pathname-trap-diverges"].Stdout, "TRAP")
				found := strings.Contains(cells["exec/failed-exec-on-a-file-found-on-path-trap-diverges"].Stdout, "TRAP")
				if slash != found {
					return "", "this shell ran the trap by one road to a pathname and not the other, so the rows are measuring the slash rather than the pathname and no single answer reads off them"
				}
				if slash {
					return "Yes", ""
				}
				// The rows end the shell in every column — an `exec` that
				// could not happen is fatal everywhere, which is not an
				// axis — so silence here is the handler having been
				// dropped and not the row failing to reach it. Checked
				// rather than assumed: a status of 0 would mean the `exec`
				// succeeded and neither row asked anything.
				if cells["exec/failed-exec-on-a-pathname-trap-diverges"].Status == 0 {
					return "", "the `exec` did not fail at all here, so the row never reached the question of what a failure does to the handler"
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
			// The sibling of the function probe below, and the axis it is
			// easiest to read the wrong answer off: the shells that answer
			// `No` here are the two with a POSIX mode, and both move to
			// `Yes` under it. This row asks each shell under its **own
			// name** with no mode on, which is what makes it the preset's
			// default rather than the shell's whole answer — the mode's half
			// is the `set/` and `invoke/` rows, and this instrument grades
			// presets (#2659).
			Field:   "AssignmentPrefixPersistsOnSpecialBuiltin",
			Cases:   []string{"cmd/assignment-prefix-special-builtin"},
			Reading: "`x=1; x=2 export y=3` leaves `x` holding 2 in a shell that keeps a prefix written in front of a special builtin and 1 in one that takes it back",
			Read: func(cells map[string]oracle.Result) (string, string) {
				switch strings.TrimSpace(cells["cmd/assignment-prefix-special-builtin"].Stdout) {
				case "[2]":
					return "Yes", ""
				case "[1]":
					return "No", ""
				}
				return "", "the row printed neither value, so the prefix never reached the builtin"
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
			Field:   "DebugTrapPipelines",
			Cases:   []string{"axis/debug-trap-of-a-pipeline"},
			Reading: "the row writes a lower-case `d` per firing and pipes `a` through a command that upper-cases what it reads, so a `D` is an action whose output went **down the pipe** and therefore ran inside the element; with the closing `trap -` firing once itself, three `d` lines and no `D` is one firing per element made in the shell, and two `d` lines with no `D` is one firing for the pipeline",
			Read: func(cells map[string]oracle.Result) (string, string) {
				lower, upper := recordedLines(cells["axis/debug-trap-of-a-pipeline"].Stdout, "d", "D")
				switch {
				case lower == 0:
					return "", "no `d` was written at all, so this shell has no DEBUG condition and the row asks it nothing about a pipeline"
				case upper > 0:
					return "DebugTrapPipelineInEachElement", ""
				case lower == 3:
					return "DebugTrapPipelinePerSimpleElement", ""
				case lower == 2:
					return "DebugTrapPipelineOnceForThePipeline", ""
				}
				return "", "the row wrote a count of firings that fits none of the readings, so it cannot say which one this is"
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
			Field:   "AliasListingQuotesTheName",
			Cases:   []string{"alias/a-listing-spells-a-name-that-needs-quoting"},
			Reading: "the row lists an alias whose name holds a `#`, which every shell in the panel takes in a name and none of them would read back bare; a listing that quotes names writes `'a#b'=` and one that does not writes `a#b=`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["alias/a-listing-spells-a-name-that-needs-quoting"]
				switch {
				case strings.Contains(r.Stdout, "'a#b'="):
					return "Yes", ""
				case strings.Contains(r.Stdout, "a#b="):
					return "No", ""
				}
				return "", "the listing did not write the entry back, so there is no name in it to have been spelled either way"
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
			Field: "SetODeclinesADashWord",
			Cases: []string{"opt/a-set-o-name-that-begins-with-a-dash"},
			// Read from what the shell was left holding rather than from
			// the wording, because the two readings part on behavior:
			// errexit is on in a column that declined the word and off in
			// one that refused it as a name. The row's `-e` spells a letter
			// every column has for exactly that reason.
			//
			// The two controls point in opposite directions. A cell that
			// said nothing and did not turn errexit on has answered
			// neither reading, and scoring it would make silence agree with
			// whatever the preset already held. A cell that refused *and*
			// turned errexit on is not a shape either reading produces.
			Reading: "`set -o -e >/dev/null; …; case $- in *e*)` leaves errexit **on** and says nothing in a shell that will not take a dash word as the `-o` name, and refuses `-e` as a name with errexit still off in one that takes it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/a-set-o-name-that-begins-with-a-dash"]
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
			Cases: []string{"opt/a-refusable-letter-behind-a-refusable-name"},
			// Read from *which word was complained about* rather than from
			// what the shell was left holding, because the state is visible
			// only in the two columns that survive a refused letter. This
			// row is answered by every column that stops at one, which is
			// what makes the reading tell shells apart at all.
			//
			// The empty-stderr control is the #2645 one: a cell that refused
			// nothing has not been asked the question, and scoring it would
			// make silence agree with whatever the preset already held. The
			// second control is this row's own — a cell naming *both* words
			// is the dialect that reports every bad option, which is a
			// different axis and not an answer to this one.
			Reading: "`set -o zzznosuch -q` names the **letter** from the second word, and never the name, in a shell that reads every option word's letters before applying one; a shell that applies as it goes stops at the name in the first word and names that instead",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["opt/a-refusable-letter-behind-a-refusable-name"]
				if strings.TrimSpace(r.Stderr) == "" {
					return "", "this shell refused nothing — the recorded cell holds no refusal, so the row says nothing about which pass reached one first"
				}
				name := strings.Contains(r.Stderr, "zzznosuch")
				letter := strings.Contains(r.Stderr, "q")
				switch {
				case name && letter:
					return "", "the cell names both bad words, which is the dialect that reports every one of them rather than an answer about which pass ran first"
				case letter:
					return "Yes", ""
				case name:
					return "No", ""
				}
				return "", "the cell names neither of the two bad words, so it is not this row's refusal"
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
			Field: "PrintfGroupingFlag",
			Cases: []string{"axis/printf-grouping-flag"},
			// Standard error is the whole reading, and a cell with none of
			// it is the shell that *has* the flag rather than one that said
			// nothing. Stdout cannot carry this on its own: under the
			// harness's `LC_ALL=C` the separator is empty, so a shell that
			// honors the flag writes the same `1234567` a shell that threw
			// it away would — which is exactly how the issue behind #2665
			// came to record the C locale as grouping.
			Reading: "`printf \"[%'d]\" 1234567` is refused in a shell for which `'` is not a flag, the character arriving at the scan as a conversion it does not have, and is the plain number in one that takes it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["axis/printf-grouping-flag"]
				switch {
				case strings.TrimSpace(r.Stderr) != "":
					return "No", ""
				case strings.Contains(r.Stdout, "[1234567]"):
					return "Yes", ""
				}
				return "", "the row neither refused the directive nor wrote the number, so it did not reach the flag at all"
			},
		},
		{
			Field: "PrintfGroupingFlagAfterTheWidth",
			Cases: []string{"axis/printf-grouping-flag-after-the-width", "axis/printf-grouping-flag"},
			// The first row is the control and is why this cannot be read
			// from the second alone. A shell with no `'` flag at any
			// position refuses `%15'd` too, and scoring that refusal as `No`
			// would count a shell that was never asked where the flag may go
			// as one that answered "among the flags".
			Reading: "the row writes `%15'd` and `%.5'd`, which a shell that reads the flag only among its flags refuses — read beside axis/printf-grouping-flag, since a shell with no such flag at all refuses them for the other reason",
			Read: func(cells map[string]oracle.Result) (string, string) {
				if strings.TrimSpace(cells["axis/printf-grouping-flag"].Stderr) != "" {
					return "", "this shell has no `'` flag at any position — it refuses the plain `%'d` too, so its refusal here is about the flag and not about where the flag may be written"
				}
				// The row's **first** line and not the row, because its
				// third line is the control: `%'15d` is the same flag ahead
				// of the width, so every shell that has the flag at all
				// writes the padded number there. A reading that searched
				// the whole cell answered Yes for the entire panel, which is
				// what TestEveryProbeDiscriminates caught.
				r := cells["axis/printf-grouping-flag-after-the-width"]
				first, _, _ := strings.Cut(r.Stdout, "~")
				switch {
				case first == "[        1234567]":
					return "Yes", ""
				case strings.TrimSpace(r.Stderr) != "":
					return "No", ""
				}
				return "", "the row neither padded the number on its first line nor refused anything, so it did not reach the late flag"
			},
		},
		{
			Field: "PrintfNonFiniteIsConverted",
			Cases: []string{"axis/printf-non-finite-is-converted"},
			// The row's **first** line, because it is the only one whose
			// two answers differ in more than whitespace: `[INF][NAN][-INF]`
			// against `[inf][nan][-inf]`. The padding lines part the two
			// readings as well, but a reading built on counting spaces
			// cannot say which of a missing width and a missing value it
			// saw, and the third line's `[+inf]` is the same fact a second
			// time.
			Reading: "`printf '[%E][%G][%E]' inf nan -inf` capitalizes the word in a shell that puts a non-finite value through the conversion and leaves it lower case in one that writes the bare word",
			Read: func(cells map[string]oracle.Result) (string, string) {
				first, _, _ := strings.Cut(cells["axis/printf-non-finite-is-converted"].Stdout, "~")
				switch first {
				case "[INF][NAN][-INF]":
					return "Yes", ""
				case "[inf][nan][-inf]":
					return "No", ""
				}
				return "", "this shell never reached the conversion — it read `inf` and `nan` as something other than a non-finite value, so what it wrote says nothing about how it would have converted one"
			},
		},
		{
			Field: "PrintfAlternateFormAsksTheValue",
			Cases: []string{"axis/printf-alternate-form-at-nought"},
			// The row's **first** line, which carries both halves of the
			// reading in one string: the hexadecimal says whether a `0x`
			// goes in front of a nought, and the octal beside it is the
			// control that keeps the line from being read as "this shell
			// has no alternate form" — every column writes `[0]` there.
			// The second line parts the two readings as well and is not
			// what is read: its cells differ in a width as well as in a
			// prefix, and a reading that counted spaces could not say which
			// of the two it had seen.
			Reading: "`printf '[%#x][%#X][%#o]' 0 0 0` is `[0][0][0]` where C's `#` is read off the value and `[0x0][0X0][0]` where the prefix follows the digits",
			Read: func(cells map[string]oracle.Result) (string, string) {
				first, _, _ := strings.Cut(cells["axis/printf-alternate-form-at-nought"].Stdout, "~")
				switch first {
				case "[0][0][0]":
					return "Yes", ""
				case "[0x0][0X0][0]":
					return "No", ""
				}
				return "", "this column wrote neither form of the alternate prefix at a nought, so the row says nothing about which reading it has"
			},
		},
		{
			Field: "PrintfZeroFillCountsTheAlternatePrefix",
			Cases: []string{"axis/printf-zero-fill-counts-the-alternate-prefix"},
			// The row's **first** line, which is the reading at three
			// widths and nothing else. The second line is read by nobody:
			// it is the width the prefix eats into, where C's fill goes
			// empty two characters before the other reading's does, and a
			// column that wrote a short field there would still be one of
			// the two readings rather than a third. The third line is the
			// controls, which are unanimous and so say nothing.
			Reading: "`printf '[%#05x][%#05X][%#010x]' 7 255 255` is `[0x007][0X0FF][0x000000ff]` where the `0x` is counted against the width and `[0x00007][0X000FF][0x00000000ff]` where it is written past it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				first, _, _ := strings.Cut(cells["axis/printf-zero-fill-counts-the-alternate-prefix"].Stdout, "~")
				switch first {
				case "[0x007][0X0FF][0x000000ff]":
					return "Yes", ""
				case "[0x00007][0X000FF][0x00000000ff]":
					return "No", ""
				}
				return "", "this column laid the fill out as neither reading does, so the row says nothing about which of the two it has"
			},
		},
		{
			Field: "PrintfEscEscape",
			Cases: []string{"axis/printf-format-escape-splits-esc-from-capital-esc"},
			// The row's **first** line and the half of it in front of the
			// `3a`, which is the `\\e` alone. The half behind it is the other
			// letter and is read by the probe below; the second and third
			// lines are the controls and are unanimous, so a reading taken
			// from them would grade every column alike.
			Reading: "`printf 'a\\eZ' | od -An -tx1` is `61 1b 5a` where a format's `\\e` is the escape character and `61 5c 65 5a` where it is a backslash and an `e`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return escHalf(cells["axis/printf-format-escape-splits-esc-from-capital-esc"], 0, "65")
			},
		},
		{
			Field: "PrintfCapitalEscEscape",
			Cases: []string{"axis/printf-format-escape-splits-esc-from-capital-esc"},
			// The half of the same line behind the `3a`. Two probes over one
			// line because the panel divides differently at the two letters:
			// a reading that answered for both would have to call zsh and
			// BusyBox ash, which take one and not the other, something no
			// dialect field can hold.
			Reading: "`printf 'a\\EZ' | od -An -tx1` is `61 1b 5a` where a format's `\\E` is the escape character and `61 5c 45 5a` where it is a backslash and an `E`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return escHalf(cells["axis/printf-format-escape-splits-esc-from-capital-esc"], 1, "45")
			},
		},
		{
			Field: "PrintfBEscEscape",
			Cases: []string{"printf/a-b-escape-splits-esc-from-capital-esc"},
			// The `%b` site's own row, read the same way and with the same
			// split. Nothing probed this pair until #3225, and the ash
			// preset said `No` for years while the recorded cell said
			// `1b` — a disagreement no instrument was asking about, which
			// is the blind spot graded.txt exists to make visible. #3233
			// corrected the preset from its own measurement; this is what
			// would have caught it.
			Reading: "`printf '%b' 'a\\eZ' | od -An -tx1` is `61 1b 5a` where a `%b` argument's `\\e` is the escape character and `61 5c 65 5a` where it is a backslash and an `e`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return escHalf(cells["printf/a-b-escape-splits-esc-from-capital-esc"], 0, "65")
			},
		},
		{
			Field:   "PrintfBCapitalEscEscape",
			Cases:   []string{"printf/a-b-escape-splits-esc-from-capital-esc"},
			Reading: "`printf '%b' 'a\\EZ' | od -An -tx1` is `61 1b 5a` where a `%b` argument's `\\E` is the escape character and `61 5c 45 5a` where it is a backslash and an `E`",
			Read: func(cells map[string]oracle.Result) (string, string) {
				return escHalf(cells["printf/a-b-escape-splits-esc-from-capital-esc"], 1, "45")
			},
		},
		{
			Field: "PrintfNumberOperand",
			Cases: []string{"axis/printf-number-operand"},
			// The row's **first** line, because one line carries all three
			// readings: `1.5` parts reading something from reading nothing,
			// `1e3` parts a `strtoimax` that stops at the `e` from an
			// evaluation, and `010` parts C's octal from arithmetic's
			// decimal. A reading built on any one of the three could not
			// tell the other two apart.
			Reading: "`printf '[%d][%d][%d]' 1.5 1e3 010` is `[0][0][8]` in a shell that takes the whole operand or nothing, `[1][1][8]` in one that takes the number at the front, and `[1][1000][10]` in one that evaluates it",
			Read: func(cells map[string]oracle.Result) (string, string) {
				first, _, _ := strings.Cut(cells["axis/printf-number-operand"].Stdout, "~")
				switch first {
				case "[0][0][8]":
					return "PrintfNumberWholeOperand", ""
				case "[1][1][8]":
					return "PrintfNumberLeadingNumber", ""
				case "[1][1000][10]":
					return "PrintfNumberArithmetic", ""
				}
				return "", "this column answered the three operands in a combination no reading produces, so the row says nothing about which one it has"
			},
		},
		{
			Field: "PrintfRefusedOperandKeepsItsLeadingNumber",
			Cases: []string{"axis/printf-number-operand", "axis/printf-refused-operand-keeps-its-leading-number"},
			// Two rows, and the first one is not decoration: what a *refused
			// arithmetic* leaves behind can only be read from a column that
			// runs arithmetic at all. bash answers `[42][1]` to the second
			// row's first line and BusyBox ash answers `[0][0]`, which are
			// the two values this axis names — so a probe reading that line
			// alone would score three columns that never reach the question
			// as if they had answered it.
			Reading: "in a column that evaluates its operand, `printf '[%d][%d]' 42abc 1e3abc` is `[42][1000]` where a refused evaluation keeps the number `strtod` read off the front and `[0][0]` where it keeps nothing",
			Read: func(cells map[string]oracle.Result) (string, string) {
				reading, _, _ := strings.Cut(cells["axis/printf-number-operand"].Stdout, "~")
				if reading != "[1][1000][10]" {
					return "", "this column does not evaluate a printf operand at all, so nothing here can say what an evaluation it refused would leave behind"
				}
				kept, _, _ := strings.Cut(cells["axis/printf-refused-operand-keeps-its-leading-number"].Stdout, "~")
				switch kept {
				case "[42][1000]":
					return "Yes", ""
				case "[0][0]":
					return "No", ""
				}
				return "", "the row neither kept both leading numbers nor zeroed both, so it did not answer this one way"
			},
		},
		{
			Field: "PrintfC99FloatConversions",
			Cases: []string{"axis/printf-c99-float-conversions"},
			// The first line only, and read as a prefix rather than as a
			// whole string: the column that has the conversions and writes
			// twelve digits of significand answers the same question `yes`
			// as the ones that write the shortest run, and a reading that
			// demanded `[0x1.8p+0]` would have scored it as a shell without
			// the conversion at all.
			Reading: "`printf '[%F][%a][%A]' 1.5 1.5 1.5` writes `[1.500000]` and a hexadecimal float in a shell that has C99's three, and refuses each letter in one that does not",
			Read: func(cells map[string]oracle.Result) (string, string) {
				first, _, _ := strings.Cut(cells["axis/printf-c99-float-conversions"].Stdout, "~")
				switch {
				case strings.HasPrefix(first, "[1.500000][0x1.8"):
					return "Yes", ""
				case !strings.Contains(first, "0x") && strings.TrimSpace(cells["axis/printf-c99-float-conversions"].Stderr) != "":
					return "No", ""
				}
				return "", "the row neither converted the three letters nor refused them, so it did not answer this one way"
			},
		},
		{
			Field: "PrintfHexFloatDefaultIsTwelveDigits",
			Cases: []string{"axis/printf-c99-float-conversions"},
			// The row's **third** line, and it carries its own control. `0.1`
			// alone would say only that two columns write different numbers
			// of digits; `255` beside it is what says the difference is a
			// precision and not a minimum width, since a width would have
			// left `0x1.fep+7` alone and a precision pads it to
			// `0x1.fe0000000000p+7`. A column with no `%a` at all is not
			// asked, which is why the first line is read first.
			Reading: "in a column that has `%a`, `printf '[%a][%a]' 0.1 255` is `[0x1.999999999999ap-4][0x1.fep+7]` where the default precision is the shortest run that names the value and `[0x1.99999999999ap-4][0x1.fe0000000000p+7]` where it is twelve digits",
			Read: func(cells map[string]oracle.Result) (string, string) {
				out := cells["axis/printf-c99-float-conversions"].Stdout
				if first, _, _ := strings.Cut(out, "~"); !strings.HasPrefix(first, "[1.500000][0x1.8") {
					return "", "this shell has no `%a` at all, so nothing here can say what its default precision would have been"
				}
				lines := strings.Split(out, "~")
				if len(lines) < 3 {
					return "", "the row stopped before the line that carries the default precision"
				}
				switch lines[2] {
				case "[0x1.999999999999ap-4][0x1.fep+7]":
					return "No", ""
				case "[0x1.99999999999ap-4][0x1.fe0000000000p+7]":
					return "Yes", ""
				}
				return "", "the row wrote a significand neither default produces, so it did not answer this one way"
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
		{
			Field: "FatalErrorEndsAtTheCommandWord",
			Cases: []string{
				"cmd/posixbuiltins-does-not-bound-a-fatal-error-raised-inside",
				"cmd/command-bounds-any-fatal-error-raised-inside",
			},
			// Two rows, because no single one can ask every column. The
			// first turns a special builtin's refusal fatal by an option a
			// shell that has no `setopt` silently skips, which is the only
			// route to a column whose `command` reaches no builtin by
			// default; a column that does not call *that* refusal fatal is
			// asked nothing by it, and the second row — three producers
			// nobody's option decides — is what answers there. A column
			// whose `eval` is already a boundary of its own is silent on the
			// second and answered by the first.
			//
			// In both, the bare half is the control and is why neither can
			// be read from its `command` half alone: a column that printed
			// the rest of the `eval`'s text without the word never called
			// the error fatal, so nothing there was ever going to end a
			// script, and scoring its living `command` half as a boundary
			// would count a shell that was never asked as one that answered.
			Reading: "each row runs a fatal error inside an `eval`, once bare and once behind `command`, each in a subshell: a living `command` half after a bare half that stopped is a boundary at the word, and a `command` half that stopped with it is no boundary",
			Read: func(cells map[string]oracle.Result) (string, string) {
				if v, why := boundedAtTheCommandWord(
					cells["cmd/posixbuiltins-does-not-bound-a-fatal-error-raised-inside"],
					"[bare-in]", "[cmd-alive]", "[cmd="); v != "" {
					return v, why
				}
				v, why := boundedAtTheCommandWord(
					cells["cmd/command-bounds-any-fatal-error-raised-inside"],
					"[bp-alive]", "[cp-alive]", "[cp=")
				if v == "" && why == "" {
					why = "neither row put an error this shell calls fatal in front of a `command` that reached a builtin"
				}
				return v, why
			},
		},
		{
			Field: "APathnameOperandIsReportedAbsolute",
			Cases: []string{
				"axis/command-v-reports-a-pathname-operand",
				"axis/command-v-does-not-clean-a-pathname-operand",
			},
			// Two rows, and the second is what keeps the reading from being
			// satisfied by a cleaning. A shell that joined and then tidied
			// would write `<dir>/bb/tool` for the first row, which ends in
			// the operand and would read as the joining answer; on the
			// second it writes `<dir>/bb/tool` as well, where both real
			// readings keep the dot-dot. So the first row decides and the
			// second refuses a cell that lost part of the operand — which is
			// exactly the answer this shell used to give (#2931).
			Reading: "`command -v` on a relative pathname operand writes the operand back in a shell that reports it as written, and the working directory with the whole operand on the end in one that resolves it — neither shell cleaning anything out of the middle",
			Read: func(cells map[string]oracle.Result) (string, string) {
				v, why := pathnameOperandReading(cells["axis/command-v-reports-a-pathname-operand"], "./bb/tool")
				if v == "" {
					return v, why
				}
				again, whyAgain := pathnameOperandReading(cells["axis/command-v-does-not-clean-a-pathname-operand"], "./bb/../bb/tool")
				switch {
				case again == "":
					return "", whyAgain
				case again != v:
					return "", "the two rows disagree, so this shell is doing something to the operand that neither reading describes"
				}
				return v, ""
			},
		},
		{
			Field: "ReadonlyDeclaresALocal",
			Cases: []string{"readonly/inside-a-function-is-local"},
			// The `in=` half is a control rather than the reading: it is
			// `[1]` in every column, so a shell whose declaration failed
			// outright would be told apart from one that scoped it. The
			// `out=` half is the answer.
			Reading: "`b() { readonly B=1; }; b` leaves the name unset afterwards in a shell that gave the declaration a scope of its own and holding 1 in one that froze the shell's own name",
			Read: func(cells map[string]oracle.Result) (string, string) {
				r := cells["readonly/inside-a-function-is-local"]
				in, out, split := strings.Cut(strings.TrimSpace(r.Stdout), "~")
				switch {
				case !split || in != "in=[1]":
					return "", "the declaration inside the function did not take, so what the name holds afterwards says nothing about a scope"
				case out == "out=[unset]":
					return "Yes", ""
				case out == "out=[1]":
					return "No", ""
				}
				return "", "the name afterwards was neither the declared value nor absent, so the row did not reach the question"
			},
		},
	}
}

// pathnameOperandReading reads one `command -v <relative path>` cell.
//
// The operand itself is the discriminator and it is passed in, because both
// rows ask the same question of a different spelling: a cell that *is* the
// operand is the shell writing it back, and a cell ending in the operand
// after a slash is the shell putting its working directory in front of it. A
// cell that is neither has lost part of the operand, which is a cleaning and
// is no shell's answer.
//
// What stands in front is not inspected and cannot be: the harness replaces
// the scratch directory a run invented with a placeholder, so the joining
// cell reads `<tmp>/./bb/tool` rather than an absolute path. The suffix is
// the whole of the evidence, and it is enough — a cleaned answer ends in
// `/bb/tool` and never in `/./bb/tool`.
func pathnameOperandReading(r oracle.Result, operand string) (string, string) {
	out := strings.TrimSpace(r.Stdout)
	switch {
	case out == "":
		return "", "nothing was written, so the row never reached a lookup at all"
	case out == operand:
		return "No", ""
	case strings.HasSuffix(out, "/"+operand):
		return "Yes", ""
	}
	return "", "the cell is neither the operand nor something ending in the whole of it, so it does not show either reading"
}

// boundedAtTheCommandWord is the reading both #2755 rows take: a fatal error
// raised inside an `eval`, written once bare and once behind `command`, each
// in a subshell so a half that stops still leaves the next one to run.
//
// Folded rather than written twice for the reason the file already gives
// about pairs — two copies of one sentence is how a fix lands in one of them.
// The three markers are all that differ: what the bare half prints when it
// was **not** stopped, what the `command` half prints when it outlived the
// error, and the prefix that says the `command` half was reached at all.
//
// An empty answer with an empty reason means "this row did not ask", which
// lets a caller try the other row before giving up; an empty answer with a
// reason is the row asking and getting nothing.
func boundedAtTheCommandWord(r oracle.Result, bareRan, commandLived, commandReached string) (string, string) {
	switch {
	case !strings.Contains(r.Stdout, "[alive]"):
		return "", "the row did not reach its last line, so neither half of it can be read"
	case strings.Contains(r.Stdout, bareRan):
		return "", ""
	case noSuchCommand(r):
		return "", ""
	case strings.Contains(r.Stdout, commandLived):
		return "Yes", ""
	case strings.Contains(r.Stdout, commandReached):
		return "No", ""
	}
	return "", "the row reached neither end of its `command` half"
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

// recordedLines counts the recorded lines that are exactly a or exactly b.
//
// The record writes a newline as `~`, so a reading that split on "\n" would
// see one line for every cell and count nothing. Both separators are taken,
// because the same reading is applied to a live run's output when a probe is
// checked by hand.
func recordedLines(out, a, b string) (countA, countB int) {
	for _, line := range strings.FieldsFunc(out, func(r rune) bool {
		return r == '~' || r == '\n'
	}) {
		switch line {
		case a:
			countA++
		case b:
			countB++
		}
	}
	return countA, countB
}

// escHalf reads one of the two escape-character letters off a row whose first
// line is `printf` handing `a\eZ:a\EZ` to `od -An -tx1`, and is shared by the
// four probes over the two sites rather than written out four times: the
// format's reader and the `%b` argument's have different answers and the same
// shape, so one reading with the half named by an argument is the honest way
// to say that. half 0 is the text in front of the `3a` the colon writes and
// half 1 is the text behind it; letter is the byte the refusing columns write
// after the backslash — `65` for an `e` and `45` for an `E`.
//
// Silence where the bytes are neither, and never a guess. The trailing space
// od leaves is one column's and not another's — BusyBox's writes none — so
// the line is read as fields rather than compared as text.
func escHalf(cell oracle.Result, half int, letter string) (string, string) {
	first, _, _ := strings.Cut(cell.Stdout, "~")
	fields := strings.Fields(first)
	colon := -1
	for i, f := range fields {
		if f == "3a" {
			colon = i
			break
		}
	}
	if colon < 0 {
		return "", "the recorded line does not carry the two spellings either side of a colon, so the row asks this column nothing"
	}
	got := fields[:colon]
	if half == 1 {
		got = fields[colon+1:]
	}
	switch strings.Join(got, " ") {
	case "61 1b 5a":
		return "Yes", ""
	case "61 5c " + letter + " 5a":
		return "No", ""
	}
	return "", "this column wrote neither the escape character nor the backslash and the letter, so the row says nothing about whether it has this escape"
}
