// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
)

// Diagnostics is how zsh reports failure.
// ownDiagnostics is this dialect's table for a builtin that has no runner's to
// read, on the heap: the table is about 10 KB and its methods take it by
// pointer, so a copy made on a frame would be a copy on every call of the
// function holding it (#6610).
func ownDiagnostics() *interp.Diagnostics {
	d := Diagnostics()
	return &d
}

func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// What it says instead of leaving under `setopt ignoreeof`, measured
		// 2026-10-06 through a pseudo-terminal. See
		// Diagnostics.EndOfInputRefused (#6239).
		EndOfInputRefused:                "zsh: use 'exit' to exit.",
		EndOfInputRefusedInALoginSession: "zsh: use 'logout' to logout.",
		// A numbered tilde whose digits overflow: measured 2026-10-03,
		// `~99999999999999999999` warns `number truncated after 19 digits:
		// 99999999999999999999` and goes on. See
		// interp.Runner.warnTruncatedTildeNumber (#5694).
		NumberedTildeTruncated: "number truncated after %d digits: %s",
		PositionTruncated:      "number truncated after %d digits: %s",
		// A table literal mixing `[key]=` heads with bare words, refused
		// whichever way round it was written and naming neither the variable nor
		// the element: measured 2026-09-23, `typeset -A a; a=([zero]=5 four)` is
		// `bad [key]=value syntax for associative array` and the script stops.
		// See interp.Semantics.MixedTableLiteral (#4241).
		MixedTableLiteralRefusal: "bad [key]=value syntax for associative array",
		// A scalar store over a name holding a table, in the option state
		// that refuses one. Measured 2026-09-26 under `setopt ksharrays`,
		// `typeset -A h=(one 1); h=string` is `h: attempt to set associative
		// array to scalar` on standard error and the shell leaves at 1; the
		// `h+=string` spelling earns the same sentence, and from inside a
		// function the function's name goes in front of it where the file and
		// line otherwise do. The name is the one verb.
		// See interp.Semantics.ScalarStoredOverATableIsRefused (#4617).
		ScalarStoredOverATable: "%s: attempt to set associative array to scalar",
		// And the same store over a table this shell **produces** rather than
		// one a script stored, which is a sentence of its own and is written
		// with `ksharrays` *off* — where a stored table is not refused at all.
		// Measured 2026-09-26, `aliases=string` is `aliases: attempt to set
		// slice of associative array` and the shell leaves at 1; `functions`,
		// `commands` and `galiases` each answer the same way, and under the
		// option all four take the sentence above instead. See
		// interp.Runner.producedTableRefusesAScalarStore (#4639).
		ScalarStoredOverAProducedTable: "%s: attempt to set slice of associative array",
		// A table literal whose bare elements came to an odd number of
		// fields, which this shell will not pair off. Measured 2026-09-26,
		// `typeset -A h=(a 1 b)` is `bad set of key/value pairs for
		// associative array` on standard error and the shell leaves at 1; from
		// inside a function the function's name is what goes in front of it.
		// See interp.Semantics.BareElementsInATableLiteralMustPairOff (#4594).
		UnpairedTableLiteralElements: "bad set of key/value pairs for associative array",
		// The four loops this shell has, and the builtin's name is stripped
		// back out of the front of it because this dialect puts it in the
		// location: `zsh:break:1: not in while, …`.
		LoopControlOutsideALoop: "%[1]s: not in while, until, select, or repeat loop",
		// The count is judged by its *sign* rather than by whether it could
		// be read, and the sentence quotes the number it read and not the
		// word the script wrote: `break abc` is `argument is not positive:
		// 0` and `break -1` is `argument is not positive: -1`. Measured
		// 2026-09-14, and the script ends in both (#2800).
		LoopControlCount:               "%[1]s: argument is not positive: %[2]s",
		LoopControlCountNamesTheNumber: true,
		LoopControlCountStatus:         1,
		// No name in the sentence: zsh's location already carries it —
		// `./s.sh:break:1: too many arguments`.
		NumericOperandTooMany: "too many arguments",
		// zsh names itself, not the path it was invoked by. `/bin/zsh` and a
		// symlink called `myzsh` both say `zsh:`, and so does the shell run
		// as `exec -a weirdname /bin/zsh` — measured all three ways, because
		// the first alone looks like a base name rather than a fixed one.
		// The other three shells print argv[0] whole.
		SelfName: "zsh",
		// `x=${q?boom}` at a prompt leaves 5, where a bare assignment's other
		// failures leave 1. Measured 2026-10-15 on 5.9.2 run `-f -i`; see
		// interp.Runner.paramErrorInABareAssignment for the rows (#6580).
		ParamErrorInABareAssignmentStatusAtAPrompt: 5,
		// A pattern this shell will not compile, standing as a `case` arm,
		// ends the shell at **nought** from `-c` and at 1 from a script file
		// or from standard input. Measured 2026-09-27 on 5.9.2 run `-f`, both
		// streams discarded: `case '[a' in ([a) …` and `case zzz in
		// (x[[:alpha:]) …` are each 0, 1 and 1 over the three routes, while a
		// condition holding either pattern is 2 on every route and every
		// other pattern surface is 1 on every route. The complaint reaches
		// standard error and the shell leaves either way, so the status is
		// the whole of what a caller loses (#4765).
		CasePatternRefusalLeavesNought: true,
		// With one exception, and it goes the other way from ksh93's: an
		// option refused at an invocation names the **whole word** the shell
		// was started by, where SelfName above would have written `zsh`.
		// Measured 2026-09-16 invoking it as `/opt/homebrew/bin/zsh` —
		// `-o zzznosuch` and `--zzznosuch` both write the path, and an
		// unset parameter under `-c` writes `zsh:1:`. The route is the
		// split, not the sentence.
		InvocationOptionRefusalNamesTheInvocation: true,
		// Read part of a numeral too large for the machine word, and say so —
		// the only column that says anything at all. The count is of the digits
		// it got through and the second verb is the digit run as written, with
		// any radix prefix off it: `$(( 0xffffffffffffffff ))` is `number
		// truncated after 15 digits: ffffffffffffffff`. Not an error: stderr,
		// status 0, and the script carries on (#3202).
		ArithNumberTruncated: "number truncated after %[1]d digits: %[2]s",
		TypeKeyword:          "%[1]s is a reserved word",
		// The only one that names where the function came from. The second
		// verb is the file it was defined in, or the shell's own name where
		// the shell itself defined it — see Diagnostics.TypeFunctionFrom.
		// It was the fixed string `… from zsh` until #1706, which is the
		// answer for one of the three cases given to all of them.
		TypeFunctionFrom: "%[1]s is a shell function from %[2]s",
		// And the clause-less form, for the one route with no origin: a
		// program on standard input.
		TypeFunction: "%[1]s is a shell function",
		// The three kinds, each with its own sentence. A suffix alias names
		// the *extension*: `whence -v p.txt` is `txt is a suffix alias for
		// cat`.
		TypeAlias:       "%[1]s is an alias for %[2]s",
		TypeGlobalAlias: "%[1]s is a global alias for %[2]s",
		TypeSuffixAlias: "%[1]s is a suffix alias for %[2]s",
		CommandVAlias:   "alias %[1]s=%[2]s",
		// A name declared with `autoload` and not yet called is not an
		// ordinary function here and does not say it is: measured,
		// `whence -v myfn` and `type myfn` both write this line, and the
		// same name after one call writes TypeFunction instead.
		TypeUndefinedFunction:     "%[1]s is an autoload shell function",
		TypeUndefinedFunctionFrom: "%[1]s is an autoload shell function from %[2]s",
		TypeNotFound:              "%[1]s not found",
		TypeNotFoundUnprefixed:    true,
		// And on standard output, which is the other half of treating it as
		// an answer rather than a complaint: `type -- nope ls` prints the
		// miss and the hit on one stream, in the order they were asked for.
		// Measured: `type nope 1>/dev/null` prints nothing.
		TypeNotFoundOnStdout: true,
		// `command -V` complains the way `type` does, shell's name and all.
		CommandVNotFound: "%[1]s not found",
		// A function said back keeps its opening brace on the header's
		// line: `f () {`. The other engine gives the brace a line of its
		// own, which is the wording's fallback.
		FunctionListingHeader: "%[1]s () %[2]s",
		JobStarted:            "[%[1]d] %[2]d",
		// Two spaces before the marker, one after, and a state of its own in
		// lower case in a column nine wide with two spaces after it. zsh
		// never lists a finished job, so it needs no word for one.
		//
		// Nine-and-two rather than a flat eleven, which is the same for
		// every word that fits and is not the same for one that does not.
		// Measured 2026-09-25 on 5.9.2: `hangup` is followed by five
		// spaces and `interrupt` by two — eleven either way — while
		// `terminated` is followed by **two**, for twelve, and
		// `broken pipe` and `segmentation fault` by two as well. A flat
		// eleven would have written `terminated ` and run
		// `segmentation fault` straight into the command. Nothing moves for
		// the words this shell wrote before the signal ones arrived:
		// `done`, `running`, `suspended`, `continued` and `exit N` are all
		// nine or fewer (#4508).
		JobLine: "[%[1]d]  %[2]s %-9[3]s  %[4]s",
		// `jobs -l`, and `jobs -p` too: the process id after the marker,
		// with the same state column after it.
		JobLineLong: "[%[1]d]  %[2]s %[3]d %-9[4]s  %[5]s",
		// A backgrounded pipeline an element at a time, under the column the
		// state starts in. See interp.Diagnostics.JobElementLine (#5322).
		JobElementLine:     "       %-9[1]s  %[2]s",
		JobElementLineLong: "       %[1]s %-9[2]s  %[3]s",
		JobElementJoin:     " | ",
		// `jobs -d`, under the row rather than in it. Spaces either side of
		// the colon, and the directory with the home directory written `~`.
		// Measured 2026-09-25 on 5.9.2: `(pwd : /tmp/jobsdir/a)` from `/tmp`
		// and `(pwd : ~/x)` from the home directory (#4507).
		JobDirectoryLine: "(pwd : %[1]s)",
		// The builtin's name is in the location here rather than in the
		// sentence, which is this shell's rule for every message.
		// About its table rather than about the function, and the builtin
		// is named in the location as it is for every message here.
		UnsetFunctionNotFound: "no such hash table element: %[1]s",
		// The array alone, and a sentence about the assignment rather than
		// about the subscript.
		BadArraySubscript:         "%[1]s: assignment to invalid subscript range",
		ArithEmptySubscript:       "invalid subscript",
		ArithEmptySubscriptTarget: "not an identifier: %[1]s[]",
		// A plain `a[]=6` gets that same sentence, which is measured and
		// not inferred: 5.9.2 answers the assignment and the arithmetic
		// write with one line, where bash words the two apart (#3949).
		AssignEmptySubscript: "not an identifier: %[1]s[]",
		// One sentence either way, which is the measurement rather than a
		// copy: 2026-09-17, a script file, `typeset 'a[]'=v` and `typeset
		// 'a[]'` both write `./f.sh:2: not an identifier: a[]` and the script
		// ends at 1 — the same words the write of an empty subscript gets in
		// an expression, and no builtin in the location, where zsh's own
		// bad-name refusal writes one.
		DeclarationEmptySubscript:          "not an identifier: %[1]s[]",
		ValuelessDeclarationEmptySubscript: "not an identifier: %[1]s[]",
		// The same sentence at the parameter site, and its own field because
		// the two coincide here and do not in bash — see the field.
		EmptyParamSubscript: "invalid subscript",
		// And the same sentence for a `[` the word never closed, which is a
		// different construct wearing this shell's one complaint. See
		// interp.Diagnostics.BareSubscriptUnclosed.
		BareSubscriptUnclosed: "invalid subscript",
		// The subscript machinery's other complaint with the same words and
		// a different construct: `${(k)x[1,2]}` and `${a[(i)q,2]}` are each
		// a reading that needs one index over a subscript that names a span.
		SubscriptIsAnIndexAndARange: "invalid subscript",
		// And the sentence for a subscript that expanded to nothing, which
		// is the arithmetic reader's rather than the subscript machinery's:
		// measured, `zsh:1: bad math expression: empty string`, where the
		// written `${a[]}` on the line above is `invalid subscript`.
		EmptySubscriptTextExpanded: "bad math expression: empty string",
		// The two ways a subscripted array literal has no span to land in.
		// Worded apart from each other here, and both without the subscript:
		// what is reported is the name's kind rather than the number.
		ArrayValueToNonArray:      "%[1]s: attempt to assign array value to non-array",
		SliceOfAnAssociativeArray: "%[1]s: attempt to set slice of associative array",
		// `+=` at a subscript of a numeric name. The name is not in the
		// sentence at all here, where it is in both of the two above — the
		// location carries it, as it does for the messages this shell raises
		// from an expansion.
		AppendToANumericSlice: "attempt to add to slice of a numeric variable",
		// The identical sentence from `unset`, and the builtin is *not* in
		// the location for it — the store is speaking rather than `unset` —
		// which is why the two routes need two fields even where one shell
		// words them alike.
		UnsetSubscriptBeforeTheFirstElement: "%[1]s: assignment to invalid subscript range",
		// Through a literal it is the subscript alone, and the literal is
		// named for what it is rather than by the variable it fills.
		BadArrayLiteralSubscript: "bad subscript for direct array assignment: %[2]s",
		DeclareNoSuchVariable:    "no such variable: %[1]s",
		// Measured: `typeset -i64 a=100` and `typeset -i1 f=5` are both
		// refused, the name is left with nothing, and the script carries on.
		IntegerBadBase:           "invalid base (must be 2 to 36 inclusive): %[2]s",
		LocationNamesTheFunction: true,
		SetInvalidOptionName:     "no such option: %[1]s",
		SetImmovableOptionName:   "can't change option: %[1]s",
		// Restricted mode's sentences, measured 2026-10-02 on zsh 5.9.2
		// (#5155). The location names the builtin where one is speaking —
		// `zsh:cd:1: restricted`, `zsh:exec:1: ls: restricted` — and the
		// redirection's sentence names no word at all.
		RestrictedCommandName:             "%[1]s: restricted",
		RestrictedRedirect:                "writing redirection not allowed in restricted mode",
		RestrictedVariable:                "%[1]s: restricted",
		RestrictedUnset:                   "%[1]s: restricted",
		RestrictedExec:                    "%[2]s: restricted",
		RestrictedCommandOption:           "%[3]s: restricted",
		RestrictedHashPath:                "restricted: %[1]s",
		OptionLetterEchoedWithAMinus:      true,
		RestrictedVariableNamesTheBuiltin: true,
		// The bare letter with a dash written in front of it: this shell
		// echoes `-q` for `set +q` as dash does, rather than the sign it was
		// asked with. `set` is named in the location, not in the sentence.
		SetInvalidOptionLetter: "bad option: -%[2]s",
		// The one shell that answers 1 for both, which is what separates it
		// from the two that answer 1 for one spelling and 2 for the other.
		SetInvalidOptionNameStatus:   1,
		SetInvalidOptionLetterStatus: 1,
		// A denied `set -m` echoes the spelling it was asked with — `-m` or
		// `monitor` — and fails at 1, fatally like every `set` failure here.
		MonitorDenied:       "can't change option: %[1]s",
		MonitorDeniedStatus: 1,
		// `[[ -o nosuchoption ]]`. The same words `setopt` uses for the same
		// mistake, and measured to be: one message, said at the condition's
		// own location rather than a builtin's. The status is 3, which is
		// neither of the two a condition otherwise gives.
		UnknownConditionOption: "no such option: %[1]s",
		// A known conditional operator with the wrong number of operands,
		// which this shell's grammar accepts and this refuses when it runs.
		// The operator is named and the surplus word is not — `[[ -n ]]`,
		// `[[ -n x y ]]` and `[[ -n x -z "" ]]` are one sentence (#965).
		UnknownCondition: "unknown condition: %[1]s",
		// And what a condition this shell could not *read* says, which is a
		// pair of sentences rather than one: the word named moves with how
		// many words the group holds, and the second word chooses between
		// the two lead-ins. Measured 2026-09-15 — `[[ p q r ]]` is
		// `condition expected: q` and `[[ p q ]]` is `parse error:
		// condition expected: p`. See Diagnostics.ConditionExpected (#2846).
		ConditionExpected:            "condition expected: %[1]s",
		ConditionExpectedPrefixed:    "parse error: condition expected: %[1]s",
		UnknownConditionOptionStatus: 3,
		// `[[ -prefix … ]]` and `[[ -suffix … ]]` reached outside a
		// completion function. No verbs: the sentence names neither the
		// operator nor the operand, measured over all three operand shapes.
		CompletionConditionOutsideCompletion: "condition can only be used in completion function",
		// zsh knows `-f` — it means functions to its own typeset — so what
		// it refuses is the combination, and it says so without naming the
		// letter it names in every other refusal.
		ExportFunctionOptionRefused: "invalid option(s)",
		JobRunning:                  "running",
		SignalDescriptions:          signalDescriptions(),
		// Only ever seen in a completion notice: zsh's listing never
		// mentions a job that has ended.
		JobDone: "done",
		// And one that exited non-zero is called by its status. Measured
		// 2026-10-02 on zsh 5.9.2: through a pseudo-terminal under `-fi`,
		// `(exit 3) &` is announced `[1]  + exit 3     ( exit 3; )` at the
		// next prompt, and under `-f -c` the first element of `exit 3 |
		// /bin/sleep 0.3 &` lists as `exit 3` (#5322).
		JobExited: "exit %[1]d",
		// A job a signal ended is named for the signal rather than called
		// done, in this shell's own words for it — see signalDescriptions,
		// which is why the number the second verb offers is not taken:
		// `[1]  + terminated  sleep 30`, where the host's C library would
		// have written `Terminated: 15` (#4508).
		JobSignaled: "%[1]s",
		JobStopped:  "suspended",
		// The two jobs-at-exit sentences are located on the script route —
		// `<script>:3: you have running jobs.` — where a session writes
		// `zsh:` with no line, and a held `exit N` there leaves with 1
		// rather than with N. Measured 2026-09-26; see the fields (#4545).
		JobsAtExitLocatedInAScript: true,
		// And on the `-c` route: the end names nothing, an `exit` names the
		// jobs where a diagnostic of its line would be, and the warning is
		// at line 1. Measured 2026-10-05 (#6035).
		JobsAtExitOnACommandString: true,
		// See interp.Diagnostics.FinishedJobsReportedAtExit (#6148).
		FinishedJobsReportedAtExit: true,
		HeldExitInAScriptStatus:    1,
		// And the three stops that are not ^Z carry a word of their own.
		// Measured 2026-09-26 — see Diagnostics.JobStoppedBySignal for the
		// table and for why this is keyed on the signal rather than worded
		// with SignalDescriptions: `(signal)` is not what this shell calls
		// SIGSTOP anywhere else (#4527).
		JobStoppedBySignal: jobStopWords,
		// ^Z is a sentence rather than a listing row here, and the shell
		// names itself in it: `zsh: suspended  sleep 40`, two spaces, no job
		// number. Under a newline of its own, as bash's is.
		JobStoppedNotice:           "%[3]s: suspended  %[4]s",
		JobStoppedNoticeOnANewLine: true,
		// `fg` and `bg` both print a listing row, and the state in it is
		// JobLine's 11-wide column with the resume's own word in it. Not
		// always `continued`: measured 2026-09-15 on a pseudo-terminal,
		// `fg` on a job that is *running* prints `[1]  + running    sleep 1`
		// — the row `jobs` prints — and only a job the shell had to send a
		// continue to is called continued. So the state is a verb here and
		// `continued` is JobContinued, beside JobRunning and JobStopped
		// where a state word belongs (#2838).
		JobResumedInForeground: "[%[1]d]  %[2]s %-9[4]s  %[3]s",
		// `bg` never sees the other state: a job that is already running is
		// refused below rather than resumed, so the only row this prints is
		// the continued one. Spelled with the literal word for that reason —
		// the format that takes the verb is the one that has two answers.
		JobResumedInBackground: "[%[1]d]  %[2]s continued  %[3]s",
		JobContinued:           "continued",
		// `bg` on a job that is not stopped is refused, and the sentence
		// names neither the builtin nor the job — this shell's location
		// prefix already carries the builtin, as `bg: %1: no such job` does.
		// Status 1, where bash complains about the same thing and still
		// reports success. Measured 2026-09-15 from a script with `set -m`
		// on a pseudo-terminal and again at an interactive prompt (#2838).
		JobAlreadyInBackground:       "job already in background",
		JobAlreadyInBackgroundStatus: 1,
		// The shell names itself here too, and the held `exit` reports
		// nothing — measured, `echo $?` after the refusal says 0, where
		// bash's says 1.
		StoppedJobsAtExit: "%[1]s: you have suspended jobs.",
		RunningJobsAtExit: "%[1]s: you have running jobs.",
		// And what it says once it goes anyway, having sent the signal
		// `setopt hup` asks for. The shell names itself here too; `jobs`
		// however many, including one, which is the reference's own wording
		// and not a plural this rendering chose. Measured 2026-09-25 through
		// a pseudo-terminal: `setopt no_check_jobs`, `sleep 3 &`, `exit`
		// writes `zsh: warning: 1 jobs SIGHUPed` on the error stream and the
		// job is gone. See interp.Runner.SendsHangupToJobsAtExit.
		JobsHUPedAtExit: "%[1]s: warning: %[2]d jobs SIGHUPed",
		// The reason first and the name after it, which is zsh's shape and
		// nobody else's. Lowercased, which LowercaseReason already says.
		// Same either way — zsh does not distinguish opening from creating.
		CannotOpen: "%[2]s: %[1]s",
		// zsh names nobody: `cat <&qq` and `cat <&""` are both `file number
		// expected`, so the wording uses neither verb and the empty case has
		// nothing of its own to say. The writing side never reaches this —
		// GreatAmpTarget sends it to the file.
		DuplicationTargetIsNotADescriptor: "file number expected",
		SubstitutionTargetWithMore:        "invalid syntax for process substitution in redirection",
		JoinedModeMismatch:                "file mode mismatch on fd %[1]d",
		// A refused `>&p` names the facility rather than the word:
		// `coprocess: bad file descriptor`, measured 2026-09-16 with no
		// coprocess running. ksh93 quotes the `p` the script wrote.
		CoprocessDuplicationTargetName: "coprocess",
		CannotCreate:                   "%[2]s: %[1]s",
		// Under `noclobber`, a name holding something that is not a regular
		// file is opened — `>/dev/null` writes — and where that open fails
		// for a reason of its own zsh says `file exists` about it anyway.
		// Measured: a directory, a socket, a dangling symlink and a
		// `/dev/tty` with no controlling terminal all answer `file exists`
		// here and answer with the open's own reason in the other three.
		NoclobberRefusalCoversAFailedOpen: true,
		// The null-command hook's own refusal, for a command that is only
		// redirections and a `NULLCMD` with nothing in it. No verbs, and the
		// same sentence whether the parameter was unset or emptied.
		RedirectionWithNoCommand: "redirection with no command",
		NotABuiltin:              "no such builtin: %[1]s",
		CommandStringParsedWhole: true,
		// Reading a program from standard input, a line that does not parse
		// is reported and the next line is read anyway. This shell alone,
		// and this route alone: the same program in a file stops it.
		StdinProgramSurvivesAParseFailure: true,
		ArithInfinity:                     "Inf",
		ArithNotANumber:                   "NaN",
		ArithFloatDigits:                  17,
		ArithFloatKeepsPoint:              true,
		SelectPrompt:                      "?# ",
		// The reason before the name is zsh's shape everywhere, and here it
		// drops the reason altogether: a script operand that is missing, one
		// that is mode 000 and a directory all get the same sentence and the
		// same 127. So zsh knows the file would not open and declines to say
		// which way — the one shell in the panel that splits neither the
		// wording nor the status.
		ScriptNotFound: "can't open input file: %[1]s",
		// `zsh -c` with nothing behind it names the option last, which is
		// why this is a verb rather than a fixed sentence. Measured
		// 2026-09-22 on zsh 5.9.2.
		InvocationMissingOptionArgument: "string expected after %[1]s",
		// At 1, where the other columns exit with their usage status 2.
		// Measured 2026-10-03 on `zsh -c` and `zsh -o` alike.
		InvocationMissingOptionArgumentStatus: 1,
		// A `=~` pattern that will not compile is a match that did not
		// happen, at 1, with the engine's reason and not the pattern.
		// Measured 2026-10-03 on zsh 5.9.2. See interp.Diagnostics.InvalidRegex.
		InvalidRegex:            "failed to compile regex: %[2]s",
		InvalidRegexStatus:      1,
		ScriptNotFoundStatus:    127,
		ScriptNotReadableStatus: 127,
		Location:                interp.LocationTightLine,
		// And no line at a prompt, which is the same answer this shell gives
		// a program on standard input: measured 2026-09-11 under `-i`,
		// `if; then` then end of input is `zsh: parse error near `\n'` where
		// a script file is `s.sh:3: parse error near `\n'`.
		PromptLocation: interp.LocationNameOnly,
		// And a builtin's complaint at a prompt is the *builtin's* name
		// alone — no shell, no line — which is the same answer this shell
		// gives a program on standard input and a different one from the
		// shell's own messages above: measured 2026-09-11 under `-i`,
		// `cd /nope` is `cd: no such file or directory: /nope` where
		// `nosuchcmd` is `zsh: command not found: nosuchcmd` (#2024).
		PromptBuiltinLocation: interp.LocationBuiltinNameOnly,
		BadSubstitution:       "bad substitution",
		// The position is 1-based and counts from the `$`: `${(Y)x}` errors
		// at 4, and a group that runs out of text errors just past the end.
		ExpansionFlagsError: "error in flags near position %[1]d in '%[2]s'",
		BadPattern:          "bad pattern: %s",
		// A space before the colon, and it is this shell's own spelling
		// rather than a typo carried forward: measured 2026-09-26 on zsh
		// 5.9.2 at every `-m` surface, `zsh:typeset:1: bad pattern : [`.
		BadSelectionPattern: "bad pattern : %s",
		// No verbs: this shell names neither the escape nor the value it
		// refused. Measured 2026-09-11 under `LC_ALL=C`, the whole line is
		// `zsh:1: character not in range` (#1851).
		CodePointOutsideTheLocale: "character not in range",
		// An element a declaration will not create, in the three ways this
		// shell will not create one — measured 2026-09-07. The base and the
		// subscript are the verbs, so what is quoted back is `a[1]` and not
		// the whole operand.
		//
		// There is no BuiltinBadSubscript here any more. The two entries it
		// held were reachable only through a path that never ran: a
		// declaration's operand was glob-matched before the builtin saw it,
		// and `export a[0]=v`'s complaint turned out to belong to the array
		// store rather than to the name check — it is what a bare
		// `a[0]=v` says, since the first element is number one (#1203).
		ReadonlyElementRefusal: "%[1]s[%[2]s]: can't create readonly array elements",
		IntegerElementRefusal:  "%[1]s[%[2]s]: inconsistent array element or slice assignment",
		LocalElementRefusal:    "%[1]s[%[2]s]: can't create local array elements",
		// The container letter's is the sentence a whole-name kind change
		// gets rather than one of its own, which is what makes it worth
		// having beside the three above: only the wording tells a script
		// which refusal it ran into.
		ContainerElementRefusal: "%[1]s[%[2]s]: inconsistent type for assignment",
		// `set -A 1v q` does not name the builtin in its location where
		// `unset 1x` and `typeset 1w` from this same shell do.
		// `read` joins `set` in it: `zsh:1: not an identifier: 1bad` has no
		// `read:` in the location where `zsh:export:1:` has the builtin.
		// `printf` joins the two: measured 2026-09-17, its refusal is
		// `./f.sh:1: not an identifier: 1x` with no builtin in the location,
		// where `typeset 1x` is `./f.sh:typeset:1: …`.
		BadNameRefusalHidesTheBuiltin: map[string]bool{
			"set": true, "read": true, "printf": true,
			// And `getopts`, measured 2026-09-18: `getopts x 1bad -x` is
			// `<file>:N: not an identifier: 1bad` with no `getopts` in the
			// location (#3555).
			"getopts": true,
		},
		// `set -t`, the one letter this shell has and refuses to move. It is
		// `singlecommand` under a borrowed spelling, one of the five options
		// about being interactive that this shell will not let a script
		// change — and it answers the letter exactly as it answers the name:
		// measured 2026-09-10, `set -t` is `can't change option: -t` at 1 and
		// stops the script, the same sentence, status and fatality as
		// `set -o singlecommand`, with the letter echoed back rather than the
		// option's own name.
		//
		// Before this it rode UnimplementedOptionLetters and said `-t is not
		// implemented yet`, which is this implementation confessing to
		// something the shell itself refuses (#1716).
		//
		// **And it no longer rides here either** (#5057). `t` is now a row of
		// setLetterOptions naming `singlecommand`, and that is the same route
		// `-i` and `-Z` already took: the name refuses the same way, so the
		// letter does. Measured with the entry deleted — `set -t` is the same
		// sentence and status, `zsh -t script` still takes the option at
		// invocation and still writes `t` in `$-`, and all thirty-six letter
		// rows and twelve controls are unmoved. A letter in two tables is one
		// place for them to disagree, and a mutation that deleted this one
		// killed nothing, which is how it was found.
		// History expansion's three complaints, and zsh words all three
		// unlike anybody else — measured 2026-09-15 through a
		// pseudo-terminal. The reference goes at the *end* and loses the `!`
		// that introduced it, and a failed substitution names nothing at all.
		HistoryEventNotFound:          "event not found: %[2]s",
		HistorySubstitutionFailed:     "substitution failed",
		HistoryModifierFailed:         "modifier failed: %[1]s",
		HistoryBadWordSpecifier:       "no such word in event",
		HistoryNoPreviousSubstitution: "no previous substitution",
		// The trace attribute is a **variable** letter this engine records
		// and lists (#3101) and a **function** mark it does not. This shell
		// writes the mark inside the body — `f () {` then a `# traced`
		// comment line then the statements — where the one other shell with
		// the letter writes a `declare -ft f` row after the body, so the two
		// are not one rendering and this column's half is not built. Named
		// as missing on a `-f` line alone, which is the narrowest true
		// statement: `typeset -t v=1` is a declaration here and only
		// `typeset -ft f` and the bare `typeset -ft` are refused.
		// Empty, and the emptiness is the measurement: the `t` that was
		// here for all three words is built now (#5067), and there is no
		// other letter this shell spells on a function line and this engine
		// does not. The field stays named in this literal's shape by its
		// absence — a letter that joins it later goes back here rather than
		// into UnimplementedOptionLetters, which would refuse it on a
		// variable line too.
		UnimplementedOptionLetters: map[string]string{
			// **`set` has left this table entirely**, and the emptiness is
			// the measurement. zsh gives a single letter to far more of its
			// options than the rest of the panel does — measured 2026-09-05,
			// it refuses only b, c, j, q and z of the fifty-two — and
			// thirty-three of the ones it has were listed here as letters
			// this shell had not built, while `set -o <name>` already took
			// and moved every one of the options behind them. The letters
			// were the only thing missing, so they are a table of names now
			// rather than a table of refusals: see setLetterOptions in
			// setopt.go, which #2578 measured a letter at a time.
			//
			// The three reasons a letter is in neither table are worth
			// keeping, because each is a different sentence:
			//
			//   - `-A` assigns an array and is implemented, in
			//     Semantics.SetArrayLetter.
			//   - `-t`, `-i` and `-Z` are options this shell will not
			//     *move*, which zsh words differently from a letter nobody
			//     has: `can't change option: -i`. `-t` reaches that through
			//     ImmovableOptionLetters above and the other two through
			//     setLetterOptions, since the names they abbreviate already
			//     refuse the same way.
			//   - `-b`, `-c`, `-j`, `-q` and `-z` are letters zsh itself has
			//     not got, so the substrate's own bad-letter refusal is
			//     already zsh's answer.
			// zsh's hash past `-r`: `-d` is the *named directory* table
			// rather than a forgetting, `-f` hashes every command on PATH
			// at once, `-v` reports each entry as it is made and `-L` lists
			// the table as `hash` commands. Measured 2026-09-13 by asking zsh
			// 5.9.2 for every letter of the alphabet: it takes these six and
			// refuses the rest, so a script asking for one is told it is
			// missing here rather than told zsh has not got it.
			//
			// **`-m` has left this list** (#4445): it reads the operands as
			// patterns over either table and is implemented, in
			// Semantics.HashReadsOperandsAsPatterns. A letter in the accepted
			// set and still named here is refused as missing while it works,
			// which is why the two tables move together.
			//
			// **`-v` has left it the same way** (#4964): it reports each
			// operand the call answered, in
			// Semantics.HashReportsEachEntry.
			//
			// **`-d` left it in the same change, and it was dead text rather
			// than a refusal.** hashOptionLetters adds the letter behind
			// HashDefinesANamedDirectory, which this shell answers Yes, so
			// the accepted set has always won and `hash -d a=/tmp` has always
			// worked — the entry here said "zsh has this and we have not"
			// about a letter that was implemented. That is exactly the drift
			// the comment above warns about, read from the other side: a
			// letter named here while the accepted set spells it is either a
			// wrong refusal or, as here, a line nothing reads.
			//
			// **And `-f` left it with #5266**: it fills the table from PATH,
			// in hashFill.
			"hash": "L",
			// read's letters about a terminal or the line editor —
			// -e/-E echoing, -z and the zle pair -c/-l. The -p
			// coprocess is implemented as its measured refusal — see
			// ReadNoCoprocess. zsh's read also says nothing at all about a
			// dead -u descriptor and reports 1, which is why no
			// ReadBadFileDescriptor wording appears here.
			//
			// `-k` has left this list and joined ReadOptions above, spelled
			// `k#` — a number, optional — which is the one letter in the
			// panel with that shape. It reads characters from the terminal;
			// interp/readkeys.go carries what was measured. `-q` has left it
			// the same way and for the same reason: it is that read with a
			// yes-or-no verdict on the end, and the two letters compose. The
			// two tables move together on purpose: a letter in the accepted
			// set and still named here is refused as missing while it works,
			// and a letter in neither is `bad option` for something zsh has.
			//
			// **`-e`, `-E` and `-z` have all left this list** (#4963,
			// #4966): the first two are the echo pair and the third reads
			// the editor buffer stack, and all three are implemented — in
			// ReadOptions above, in
			// Semantics.ReadEchoLettersWriteTheValues and in
			// editorbuffer.go. A letter in the accepted set and still named
			// here is refused as missing while it works, which is why the
			// two tables move together.
			"read": "cl",
			// typeset's letters this engine does not hold: the float
			// format (-E), the key read (-k) and tracing (-t).
			// `-H`, `-U`, `-T`, `-h` and `-m` have left this list — they are
			// implemented, in DeclareOptions above.
			// `L`, `R` and `Z` have left this list: they are the width
			// attributes and are implemented, in DeclareOptions and
			// DeclareOptionsTakingANumber above.
			//
			// **`-b`, `-c` and `-n` have left it because this shell has not
			// got them**, which is the other way a letter leaves: naming one
			// here says "zsh has it and we do not", and refuses it as
			// missing where zsh refuses it as unknown. Measured 2026-09-15
			// on zsh 5.9.2, `env -i` with a scratch HOME, in a function so
			// all three words are reachable — `typeset -b v=1`, `-c` and
			// `-n` are each `bad option` under `typeset`, `declare` and
			// `local` alike, while `-E`, `-k` and `-t` are silently taken.
			// `-n` is the one that was load-bearing: #2553 built the name
			// reference for bash and ksh93 on the premise that this shell
			// spelled it too, and it does not.
			"typeset": "k",
			// `w` has left this list and joined TypeOptions above, in the same
			// change: a letter in both is refused as missing while it works,
			// and a letter in neither is `bad option` for something this
			// shell has. TestNoLetterIsBothAcceptedAndCalledMissing is the
			// invariant.
			// `s` and `S` left this list with `w` and for the same reason,
			// in #4742: `whence -sv` and `whence -Sv` already wrote the two
			// lines, so `type` refusing them was one spelling of a question
			// answering where the other did not.
			// `v` has left it the other way, the way `-b`, `-c` and `-n` left
			// typeset's list above: this shell has not got it and neither
			// has zsh. Naming it here said "zsh has it and we do not", so
			// `type -v` was refused as missing (`-v is not implemented yet`,
			// at 2) where zsh refuses it as unknown — `bad option: -v`, at 1
			// (#5221). Measured 2026-09-30 on zsh 5.9.2, `-f` on a script
			// file: `type -v ls`, `type -v`, `type -wv` and `type -vw` are
			// all `bad option: -v` at 1, and `-t`, `-q` and `-z` refuse the
			// same way here already, which is the control that says the
			// refusal road was right and only the letter was misfiled.
			// `whence -v` is unaffected — that is whence's own letter.
			//
			//
			// `m` has left it the first way, by being built (#5230): `type -m`
			// is `whence -vm`, the pattern lookup, and a zsh `type` that is
			// handed the letter is answered by whence's own walk. See
			// typeBuiltin in whence.go.
			// jobs' letters that are zsh's own: -d names the directory the
			// job was started in, and -z and -Z are about the process
			// title rather than about the job table.
			// `-d` came off this list in #4507: the directory line it
			// writes needed the job table to record where each job
			// started, which nothing here did before.
			"jobs":    "zZ",
			"declare": "k",
			// The same list as `typeset` and `declare`, which is the point:
			// `-F` is one attribute and the three names declare it alike.
			// It was here and in neither of theirs, which is the same split
			// LocalOptions had — the letter refused under one name and
			// answered under the other (#1594).
			// `local`'s list is *not* `typeset`'s, and `m` is why: this
			// shell's `local` has no `-m` to be missing — `local -m q`
			// inside a function is `bad option: -m` there — so naming it
			// unimplemented told a script the letter was on its way when
			// nothing was coming. `L`, `R` and `Z` are the padding letters
			// `local` really does spell and this engine does not.
			//
			// `-k` was gone for the same reason `-b`, `-c` and `-n` are:
			// measured in the same run, `local -k v=1` inside a function is
			// `bad option: -k` where `typeset -k` and `declare -k` are taken
			// — so this list was one letter shorter than theirs and not the
			// same list after all. `-t` was the last of it and left with
			// #3101, so `local` has no row here at all now.
			// `integer`'s own short list is empty now: the letters typeset
			// is missing that `integer` refuses outright — b, c, E and m —
			// are bad options under this name and belong in neither field,
			// and `-h`, `-L`, `-R` and `-Z` are in IntegerOptions above.
			// `private`'s own two, and they are `local`'s missing letters
			// rather than `typeset`'s: measured 2026-09-27 a letter at a
			// time on zsh 5.9.2, `private -E v` and `private -m 'v*'` are
			// each taken there and neither has a route under a scoped
			// declaration here. `-T`, `-f`, `-g`, `-M`, `-n` and `-z` are
			// in neither table, because that shell's `private` refuses them
			// too — so the substrate's own bad-letter refusal is already
			// its answer, and `-g` is the one worth reading twice: the
			// letter means *do not take a scope*, which is the opposite of
			// what the word asks for.
			"private": "Em",
			// `functions`' own letters, none of which is `typeset`'s: -u
			// and -U mark a name for autoloading, -k and -z pick which
			// shell the autoloaded file is read as, -t and -T trace, -x
			// sets the listing's indent, -c makes one function another's
			// copy and -W is the `zsh/parameter` writability flag. -m and
			// -M are implemented, in FunctionsOptions above, and so are
			// -u and -U: they mark, and with no operands they are the
			// listing a bare `autoload` writes (#1996).
			// `-x` has left this list: it sets the listing's indent and is
			// implemented, in FunctionsOptions above (#4442). `-s` has left
			// it too: it is the string form of `-M`, and on a line without
			// `-M` it is a letter that decides nothing and is taken in
			// silence, which is what that shell does with it (#4443).
			// `-t` and `-T` have left this list: they mark a function for
			// tracing and are implemented, in FunctionTraceLetters above
			// (#5067).
			"functions": "kz",
		},
		// `u` alone, and that is the measurement rather than an omission:
		// `functions +u` and `typeset +fu` are `invalid option(s)` at 1
		// where `functions +U` and `typeset +fU` are a listing at 0, though
		// both letters mark. So the refused set is strictly narrower than
		// Semantics.FunctionLettersThatMarkUndefined and cannot be derived
		// from it (#1996).
		// The line a listing writes inside a traced function's body —
		// measured 2026-09-28 on zsh 5.9.2, `functions -t f; functions`
		// writes `f () {`, then this, then the statements. The same text for
		// `-t` and for `-T`; the listing does not say which letter marked
		// it. See Diagnostics.TracedFunctionListingLine.
		TracedFunctionListingLine: "# traced",
		MarkingLettersUnderPlus:   "u",
		// `functions`' own wording and not `autoload`'s `bad option: -Q`,
		// because the line never reached a marking: `zsh:functions:1:
		// invalid option(s)` and `zsh:typeset:1: invalid option(s)`, each
		// naming the word that was written.
		MarkingUnderPlusRefusal: "invalid option(s)",
		// The builtin's name is stripped to the location prefix as ever:
		// `zsh:read:1: -p: no coprocess`, measured with no coprocess to
		// read, which is the only state this shell has.
		// `zsh:functions:1: number expected after -x`, and not the
		// `argument expected: -x` the same builtin says when nothing followed
		// the letter at all. Measured 2026-09-26 on zsh 5.9.2, both at 1.
		FunctionsIndentNeedsANumber: "%[1]s: number expected after -%[2]s",
		// `zsh:functions:1: -Ms: must take a single string argument`, at 1
		// and with nothing registered. Measured 2026-09-26 on zsh 5.9.2.
		MathFunctionStringArity: "%s: -Ms: must take a single string argument",
		// `zsh:read:1: only one array argument allowed`, at 1 and before
		// the read. Measured 2026-09-26 on zsh 5.9.2.
		ReadOnlyOneArray: "read: only one array argument allowed",
		ReadNoCoprocess:  "read: -p: no coprocess",
		// `zsh:read:1: argument expected: -d` — the letter after the
		// sentence, unlike everyone else, and status 1 like every other
		// option complaint here.
		OptionNeedsArgument: "%[1]s: argument expected: -%[2]s",
		ReadonlyVariable:    "read-only variable: %s",
		// A frame writing a name one of its callers declared `private` over
		// nothing. Measured 2026-09-27 on zsh 5.9.2, `-f`, under `-c` and
		// from a script file alike: `g: v: can't change parameter attribute`,
		// the script ending at 1 — for a scalar, for an array, for a `+=`
		// append and for `typeset -g v=7`, and with the function's name in
		// the location exactly as the readonly refusal above has it.
		//
		// It is a different sentence from that one, which is what says the
		// two rules are different rules: a callee assigning a frozen name in
		// the same shell is `g: read-only variable: rr`, also fatal, also 1.
		PrivateParameterWrite: "%s: can't change parameter attribute",
		// And `private` over a name the running call has already declared:
		// `f:private: can't change scope of existing param: v`, status 1,
		// the script carrying on and the binding standing there untouched.
		// Measured the same day, and the same sentence whether the name was
		// made by `private` or by `local` — where `local` or `typeset` over
		// a *private* is taken at 0.
		PrivateRedeclaresName: "%[1]s: can't change scope of existing param: %[2]s",
		// The computed end and the start, in that order, where bash's
		// sentence for the same bound names the length as it was written.
		SubstringEndBehindTheStart: "substring expression: %[2]s < %[3]s",
		// And a kind letter aimed at a name this shell holds a slot for:
		// `f:typeset: path: can't change type of a special parameter`, the
		// shell ending at 1 with the rest of the script unrun, and the
		// written word in the location under every declaration utility.
		// Measured 2026-09-27; see dialect/zsh/kindfixedparameters.go.
		SpecialParameterKind: "%s: can't change type of a special parameter",
		// And the same name refusing an array literal on a declaration word,
		// which is a second sentence rather than the bare assignment's:
		// measured 2026-09-27, `private v; local v=(p q)` is this and
		// `private v; v=(p q)` is ArrayValueToNonArray above. See
		// interp/privatekindfixed.go.
		ArrayValueToANonArraySpecial: "%s: can't assign array value to non-array special",
		// A numeric letter beside an array literal over a name already set
		// in the cell the line lands on. See the field (#6566).
		TypeLetterOverAHeldName: "%s: can't assign array value to non-array",
		// The one dialect that words the refusal to unset exactly as it words
		// the refusal to assign, and the only one that does not name `unset`.
		UnsetReadonly: "read-only variable: %s",
		// `ulimit -a`, row for row as the engine writes it — the flag
		// first, no pipe row, and no resident-set row at all.
		// The sixteen rows zsh 5.9 prints on Linux. This shell lists in the
		// *kernel's* order rather than one of its own — see
		// UlimitListingInKernelOrder — so the sequence here is a reading
		// order and not the output: on a kernel that numbers nine limits
		// this prints nine, in that kernel's sequence.
		//
		// Measured 2026-09-18 on zsh 5.9.2 (macOS arm64) and 5.9 in the
		// panel's Alpine image. macOS numbers the resident set and the
		// address space alike and this shell prints the address-space row
		// for it, which is why it has no `-m` there and refuses the letter
		// while bash, listing its own table, reads it (#2806).
		//
		// `-N 15` is the one row with no letter behind it: the option form
		// is `-N <number>`, which nothing else in the panel has, so the row
		// prints and the letter is not read.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "-t: cpu time (seconds)              ", Letter: 't', Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "-f: file size (blocks)              ", Letter: 'f', Res: interp.ResourceFileSize},
			{Prefix: "-d: data seg size (kbytes)          ", Letter: 'd', Res: interp.ResourceData, Scale: 1024},
			{Prefix: "-s: stack size (kbytes)             ", Letter: 's', Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "-c: core file size (blocks)         ", Letter: 'c', Res: interp.ResourceCore},
			{Prefix: "-m: resident set size (kbytes)      ", Letter: 'm', Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "-u: processes                       ", Letter: 'u', Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "-n: file descriptors                ", Letter: 'n', Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "-l: locked-in-memory size (kbytes)  ", Letter: 'l', Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "-v: address space (kbytes)          ", Letter: 'v', Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "-x: file locks                      ", Letter: 'x', Res: interp.ResourceFileLocks, Scale: 1},
			{Prefix: "-i: pending signals                 ", Letter: 'i', Res: interp.ResourcePendingSignals, Scale: 1},
			{Prefix: "-q: bytes in POSIX msg queues       ", Letter: 'q', Res: interp.ResourceMessageQueues, Scale: 1},
			{Prefix: "-e: max nice                        ", Letter: 'e', Res: interp.ResourceSchedulingPriority, Scale: 1},
			{Prefix: "-r: max rt priority                 ", Letter: 'r', Res: interp.ResourceRealtimePriority, Scale: 1},
			{Prefix: "-N 15: rt cpu time (microseconds)   ", Res: interp.ResourceRealtimeTime, Scale: 1},
		},
		UlimitListingInKernelOrder: true,
		// `-N <number>`: the one option in the panel whose *operand* names
		// the resource, by the kernel's own number. It exists because this
		// shell has a row its letters do not cover — `-N 15: rt cpu time
		// (microseconds)` — and the number is the platform's, which is what
		// `ulimit -N 7` says: 10666 processes on macOS and 1024 open files
		// on Linux, from the same shell. Measured 2026-09-18 on zsh 5.9.2
		// (macOS arm64) and zsh 5.9 in the pinned Alpine image (#3667).
		UlimitNumberedOption: interp.UlimitNumberedOption{
			Letter:      'N',
			NeedsNumber: "number required after -%[1]s",
			BadNumber:   "invalid number: %[1]s",
			OutOfRange:  "can't read limit: invalid argument",
		},
		// Lazy rather than QuoteShell: this shell drops the empty `''`
		// segments that closing and reopening leaves, so `ab'` traces as
		// `'ab'\\'` where bash writes `'ab'\\'''`. Measured 2026-09-13 against
		// bash 5.3.15; the two shared a value until #2695 because the corpus
		// row used `it's`, the one shape they agree on.
		TraceQuoting: interp.QuoteShellLazy,
		TraceEscape:  interp.TraceEscapeControlNotation,
		// The path inside a `type`, `command -V` or `whence -v` sentence is
		// written back the way this shell writes a word — the same spelling,
		// against the same alphabet below — while `command -v`, `whence`,
		// `where` and `which` write the resolved path plain. Measured
		// 2026-09-19 with a directory on PATH holding an executable called
		// `a b`: `type 'a b'` is `a b is '/…/bb/a b'` and `command -v 'a b'`
		// is `/…/bb/a b`. The *name* is bare in both, which is what keeps
		// this apart from NameReportQuoting (#3702).
		TypeSentencePathQuoting: interp.QuoteShellLazy,
		// No position rule at all: every character it quotes it quotes
		// anywhere, which is what makes the *shape* of this field necessary
		// rather than a longer string in one shared set. It is the only one
		// of the three that quotes `=`, and it quotes `^` with bash and
		// leaves `!` bare with ksh93. Measured 2026-09-12.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[]{}~#=^",
		},
		// The `[` that opens a test is bare and the `]` that closes it is an
		// argument like any other: `[ 1 -lt 2 ']'`.
		TraceBareBracket: interp.TraceBracketCommandWordBare,
		// A declaration utility's operand is written as an assignment —
		// the target bare and only the value quoted — where bash quotes
		// the whole word. Measured 2026-09-18: `typeset x="a b"` is
		// `typeset x='a b'` here and `typeset 'x=a b'` there, and
		// `typeset x=a=b` is `typeset x='a=b'` here, where that shell
		// has no reason to quote at all. The same word after `echo` is
		// one quoted word in both, which is what says it is the command
		// in front of it that decides (#3654).
		TraceAssignmentOperand: interp.TraceAssignmentOperandValue,
		TraceStyle:             interp.TraceNameLine,
		TraceForHeader:         interp.TraceForAssign,
		TraceArrayLiteral:      interp.TraceArraySpaced,
		// The prefix goes on the command's own line, with the trace prefix
		// written a second time between the assignments and the words when
		// the command is one this shell runs itself: `+x.sh:3> A=3 +x.sh:3>
		// f zz` for a function and `+x.sh:4> B=4 /bin/echo c` for an
		// external.
		TracePrefixAssignment: interp.TracePrefixOnTheCommandLineRepeatingThePrefix,
		// The subject and the arm's patterns, once per arm it tries, where
		// bash prints the header as written once and the other two print
		// nothing. The line count is the information: it says how far down
		// the arms the subject got.
		TraceCaseHeader: interp.TraceCaseArm,
		// One line for the whole condition rather than one per primary, and
		// the operands quoted: `[[ -n a && -n b ]]` is one line here and two
		// in bash and ksh93.
		TraceCondition:        interp.TraceCondWhole,
		TraceConditionQuoting: interp.QuoteShell,
		// `=~` traced in the condition module's spelling, operands as
		// written (#5347).
		TraceRegexMatch:             "-regex-match",
		TracePatternEscapesLiterals: true,
		// A name the locale cannot print is written in `$'…'`; see the
		// field for the measurement.
		FunctionListingNameDollarQuotes: true,
		// The two arithmetic sites disagree in this shell alone: a `(( ))`
		// command is wrapped in spaced parentheses and the three parts of a
		// `for ((;;))` header are written bare — `i=0`, `i<2`, `i++`.
		TraceArithForPart: interp.TraceArithBare,
		// zsh names the last token it read and nothing else.
		EvalNaming:       interp.SourceReplacesShell,
		SourceFileNaming: interp.SourceReplacesShell,
		EvalSourceName:   "(eval)",
		// A runtime failure at the top level of a sourced file names the
		// file — `./inc.sh:1: command not found: nosuch` — while one inside
		// a function still names the function, which the answer above wins.
		LocationNamesTheCurrentFile: true,
		// A dot script's failure is diagnosed under the name it could not
		// open, which is the same rename DollarZeroNames makes on the path
		// where the file does open (#2958).
		DotFailureNamesTheFileItCouldNotOpen: true,
		// And text `eval` is running is named for itself over both — see
		// Diagnostics.LocationNamesTheEvalText, where the nine rows are.
		LocationNamesTheEvalText: true,
		// At a prompt it is the typed line's outside a function, and a
		// script's inside one (#6075).
		EvalTextAtAPromptSpeaksAsTheLine: true,
		// zsh does not quote the expression, where the other three do.
		ArithError:          "%[2]s",
		ArithInvalidBase:    "invalid base (must be 2 to 36 inclusive): %[1]s",
		ArithRecursionLimit: "math recursion limit exceeded: %[1]s",
		// A call the shell will not enter because `$FUNCNEST` of them are
		// already active. **The sentence names the parameter and not the
		// number**, which is the whole of why it is worth having the
		// parameter behind it: a shell that grew the wording without the
		// name a script can move would be telling the reader to increase
		// something it does not read. Measured 2026-09-27 on 5.9.2 with a
		// runaway recursion, at the default bound and at `FUNCNEST=5`
		// alike — the count is never in it. **Nor is the function's name**:
		// that goes in the location, which is
		// interp.FunctionNesting.RefusalNamesTheCalleeInTheLocation and is
		// why this format takes no verb where bash's takes two. See
		// funcnesting.go for the bound and for what the refusal costs.
		FunctionNestingLimit: "maximum nested function level reached; increase FUNCNEST?",
		// And it is the name the expression was written with, not the one the
		// bound stopped on: `a=b; b=a` is blamed on `a` here and on `b` in the
		// two shells above.
		ArithRecursionBlamesTheWrittenName: true,
		OptionListingWidth:                 optionListingWidth,
		KillListing:                        interp.KillListingSpaceJoined,
		// The event a refused range names, which is not always the `1` an
		// empty list used to be the only route to: `fc -l -1` on that same
		// empty list says `no such event: 0`, and `fc -l 99` says 99.
		FcNoSuchEvent: "no such event: %[1]d",
		// Its pair, for a refused range whose two ends are different
		// numbers — `fc -l 6 7` on five entries — where there is no one
		// event to name.
		FcNoEventsInRange: "no events in that range",
		// And the operand that named nothing is named here, where bash says
		// only that it found no command: `fc -l zzz` is `event not found:
		// zzz`, and so is `fc -l -0`, which this shell does not read as a
		// relative operand at all.
		FcNoCommandFound: "event not found: %[1]s",
		// The line this shell is standing on, which `fc -s` and the editor
		// road may not reach. A sentence about what running it would do,
		// and it names no operand — `fc -s 5`, `fc -s 99` and a bare
		// `fc -s` on a one-entry list all come to these words.
		FcCurrentLineRecurses: "current history line would recurse endlessly, aborted",
		// And the range this shell will not run in the order it was asked
		// for, which also names no operand.
		FcBackwardsRange: "history events can't be executed backwards, aborted",
		// The editor's file left empty, named by the path the person never
		// saw.
		FcEmptyEdit:  "read error on %[1]s",
		NoJobControl: "no job control in this shell.",
		// The refusal the verbs that would *act* on a job give in a subshell
		// holding nothing but the parent's. No verb in the sentence: the
		// builtin's name is in the location, which is this shell's rule for
		// every message. Measured 2026-09-25 under `-fm` on a
		// pseudo-terminal — `<script>:wait:6: can't manipulate jobs in
		// subshell`, at 1, and `disown` the same (#4538).
		JobsNotManipulableInASubshell: "can't manipulate jobs in subshell",
		FdVariableWithoutADescriptor:  "parameter %[1]s does not contain a file descriptor",
		FdVariableFailedClose:         "failed to close file descriptor %[1]d: %[2]s",
		// The two refusals a frozen name draws at a `{name}` redirection.
		// This shell names the *redirection* rather than the assignment, and
		// the preposition carries which direction could not happen — a number
		// cannot go **to** a readonly parameter, and cannot be taken **from**
		// one. Measured 2026-09-29, script files; the generic `read-only
		// variable:` is not written at either site, which is why the open
		// wording replaces the store's sentence rather than following it the
		// way bash's `cannot assign fd to variable` does.
		FdVariableReadonlyOpen:  "can't allocate file descriptor to readonly parameter %[1]s",
		FdVariableReadonlyClose: "can't close file descriptor from readonly parameter %[1]s",
		// And after the open refusal, when the file was opened for reading,
		// a second line: this platform's sentence for errno zero, then the
		// target — `undefined error: 0: /dev/null` on darwin and `success:
		// /dev/null` on Linux, measured 2026-10-15 (#6523). The lowercase is
		// LowercaseReason's, applied where the line is written.
		FdVariableReadonlyReadOpen: errnoText(0),
		// `{1a}>f`, which this grammar lexes and the store refuses (#6522).
		FdVariableBadName:          "not an identifier: %[1]s",
		FdVariableWouldBeClobbered: "can't clobber parameter %[1]s containing file descriptor %[2]d",
		// `mkdir dir; v=$(<dir)` — the read after a successful open, which
		// this shell alone words. The name is the word as it expanded, not
		// the path the working directory made of it (#1778).
		FileSubstitutionReadError: "error when reading %[1]s: %[2]s",
		// A failed write to a stream this command did not itself close.
		// Neither the builtin nor the number is named, so the format has one
		// verb where the others have two (#1363).
		InheritedClosedStreamWriteError: "write error: %[2]s",
		// The builtin's own complaint, reached only where the write failed
		// with EPIPE -- the closed-descriptor axis answers No above and
		// returns before this is read, which is why one wording serves both
		// errnos here without making the quiet route speak. The builtin is
		// named by the location rather than by a verb, so the format uses
		// only the reason, exactly as the sentence above it does (#770).
		BuiltinWriteError: "write error: %[2]s",
		// A conditional with no `:` is its own sentence; a conditional
		// missing either value is the ordinary end of input, so
		// ArithConditionalThen and ArithConditionalElse stay empty and fall
		// through to it.
		ArithConditionalColon: "bad math expression: ':' expected",
		// And a `:` with no `?` in front of it, which is a complaint only a
		// reader that took the colon and then read a second value can make.
		ArithColonWithoutQuestion: "bad math expression: ':' without '?'",
		// The same split ksh93 makes, said the other way round: the text
		// that could not be an operand is named where there is one, and the
		// end of the string is named where there is not.
		// A substring range that begins with a letter is a modifier list
		// here, and an unknown modifier is named unless the segment began
		// with a known one — `${x:i}` names `i` and `${x:ha}` names nothing.
		UnrecognizedModifier:      "unrecognized modifier `%[1]s'",
		UnrecognizedModifierAlone: "unrecognized modifier",
		ArithIllegalByte:          "bad math expression: illegal character: %[1]s",
		ArithOperandExpected:      "bad math expression: operand expected at `%[1]s'",
		// The same family's sentence for `++` on something that cannot be
		// assigned to, and it names neither the operator nor the operand:
		// `$(( 1++ ))` is `bad math expression: lvalue required`. Measured
		// 2026-09-14 (#2420).
		ArithIncrementNeedsAPlace: "bad math expression: lvalue required",
		ArithBadFloatConstant:     "bad floating point constant",
		ArithExpressionRanOut:     "bad math expression: operand expected at end of string",
		ArithOperatorExpected:     "bad math expression: operator expected at `%[1]s'",
		// And the text is cut at ten bytes: `(( 1 abcdefghijk ))` names
		// `abcdefghij...'. Measured 2026-10-02 (#5138).
		ArithBlamedTextShownTo:   10,
		ArithUnmatchedCloseParen: "bad math expression: unexpected ')'",
		// The byte `illegal character` names outside a group, met where a
		// group's `)` belongs. See the field (#6803).
		ArithRefusedByteInAGroup: "bad math expression: ')' expected",
		// The output format's own two, neither of which opens with `bad math
		// expression:` and neither of which names the text it refused —
		// measured 2026-09-12, `zsh:1: bad output format specification` for
		// `$(( [# 16] 1 ))` and `zsh:1: bad base syntax` for `$(( [16] 1 ))`.
		// One character apart and worded apart, which is why the parser tells
		// them apart rather than calling both a bad specifier.
		ArithBadOutputFormat: "bad output format specification",
		ArithBadBaseSyntax:   "bad base syntax",
		// The math functions, which only this shell has. The three at the
		// call stand alone rather than being wrapped as a bad math
		// expression — measured 2026-09-08, `zsh:1: unknown function:
		// nosuchmf` where an ordinary failure in the same place opens with
		// `bad math expression:`. The four at the registration are the
		// builtin's and name it in the location the way its option
		// complaints do: `zsh:functions:1: -M: too many arguments`.
		//
		// `no such function` names the *implementation*: a registration is
		// never checked when it is made, so `functions -M mf 1 1 nodef` is
		// status 0 and this arrives at the first call.
		MathFunctionUnknown:         "unknown function: %[1]s",
		MathFunctionArgumentCount:   "wrong number of arguments: %[1]s",
		MathFunctionMissingImpl:     "no such function: %[1]s",
		MathFunctionTooManyOperands: "%[1]s: -M: too many arguments",
		MathFunctionBadName:         "%[1]s: -M %[2]s: bad math function name",
		MathFunctionBadMinimum:      "%[1]s: -M: invalid min number of arguments: %[2]s",
		MathFunctionBadMaximum:      "%[1]s: -M: invalid max number of arguments: %[2]s",
		// The operator names its two-character spelling whichever of the two
		// was written: `$((#\))` says `after ##` as readily as `$((##))`.
		ArithCharacterMissing: "bad math expression: character missing after ##",
		SyntaxUnexpected:      "parse error near `%[1]s'",
		// A refused IO number is named by the operator behind it, except a
		// `{name}` outside a `case` word, which is named by its text. See
		// interp.Diagnostics.SyntaxUnexpectedIONumberIsItsOperator (#6552).
		SyntaxUnexpectedIONumberIsItsOperator:              true,
		SyntaxUnexpectedFdVariableInAWordListIsItsOperator: true,
		// The operator that backgrounds a command and lets go of it is
		// written two ways here and named by one of them — `&!` in a
		// refusal comes back `&|`. It is the only pair this shell folds;
		// see interp.Diagnostics.SyntaxUnexpectedNamesTheDisowningOperatorWithAPipe
		// for the measurement and for the pairs that keep their spelling.
		SyntaxUnexpectedNamesTheDisowningOperatorWithAPipe: true,
		// The sentence this shell writes in front of that one when the
		// function being defined is named by an alias. Two lines, and the
		// second is the parse failure above located at the parentheses —
		// which is why this is a remark rather than the error. See
		// syntax.Dialect.AliasAtAFunctionName (#3643).
		FunctionNameIsAnAlias: "defining function based on alias `%[2]s'",
		// The newline is spelled `\n` here rather than by name, and is
		// blamed on the line it ends (#1364).
		SyntaxUnexpectedNewline:          "parse error near `\\n'",
		UnexpectedNewlineIsOnTheNextLine: true,
		// The same echo bash gives, in this shell's sentence: `"zzz"` and
		// `'a b'` come back with their quotes on (#1239).
		UnexpectedWordNaming: interp.UnexpectedWordIsSourceText,
		// zsh numbers a function body that never began from the parentheses
		// it was due after rather than from the top of the input, and writes
		// no line at all where the two stand together. `f() ;` reports as
		// zsh: parse error near `;', `foo()` on line 3 of a file is still
		// s.sh:1:, and `if true` — an input that ran out just as much —
		// reports the file's own line, as zsh:1: parse error near `true'.
		MissingFuncBodyCountsFromItsParens: true,
		// zsh lets the body of a `$( … )` that never closed say what was
		// wrong with it before complaining about the substitution, and puts
		// both messages on that refusal's line. `v=$(echo hi; for` is
		// `:2: parse error near `\n'` and then the quote at `:2:`, where
		// `v=$(echo hi` — a body that parses — is the quote alone.
		UnterminatedSubstitutionWritesItsBodysRefusal: true,
		ForName: "parse error near `%[1]s'",
		// A C-style `for` header with fewer than two separators, named by
		// the text of its last part and by nothing where that part is blank.
		// Measured 2026-09-12: `for ((i=0))` is `parse error near `i=0'`,
		// `for ((;2))` names `2`, and `for ((;))`, `for ((1;))` and
		// `for (( x ; ))` are a bare `parse error`. More than two separators
		// is not refused here at all — see
		// syntax.Dialect.ForArithExtraSeparators (#2225).
		ForArithHeader:       "parse error near `%[1]s'",
		ForArithHeaderNoPart: "parse error",
		Unterminated:         "parse error near `%[5]s'",
		UnmatchedQuote:       "unmatched %[1]s",
		UnmatchedCmdSubst:    "parse error near `%[3]s'",
		// This shell prints at most twenty bytes of the word and marks it,
		// and marks it at exactly twenty as well — see the field. The other
		// three print the whole word or none of it.
		UnmatchedNearMaxBytes: 20,
		// And a control character in that text is written as an escape
		// rather than sent to the terminal — a tab as `\t`, a `\001` as
		// `^A`. Measured 2026-09-17 one byte at a time; see
		// Diagnostics.NearTextEscapesControlCharacters for the seven rows
		// and for where the high half stops (#3563).
		NearTextEscapesControlCharacters: true,
		// A word a diagnostic names, in the caret notation; see the field.
		DiagnosticNamesAWordVisibly: true,
		// And the same cut on the second message a substitution body refused
		// at expansion time is given — see the field.
		SubstitutionParseFailureQuotesTheWord: true,
		SubstitutionParseFailureSentence:      "parse error in command substitution",
		// A here-document body's refusal stands after the document and is
		// followed by the bare sentence rather than by the word's quote or
		// by an open context the body never held. See
		// interp/heredocbodyrefusallocation.go (#4714).
		HeredocBodyRefusalIsLocatedAfterTheDelimiter: true,
		HeredocBodyRefusalSentence:                   "parse error",
		// The word again, the same as for `$(` — and for `$[` too, which
		// this shell has and refuses the same way.
		UnmatchedArithSubst: "parse error near `%[3]s'",
		UnmatchedBraceSubst: "closing brace expected",
		// And `bad substitution` where text read at expansion time ran out
		// before an operator. Measured 2026-10-02 (#5138).
		RawTextBraceRanOutInTheName: "bad substitution",
		SyntaxErrorStatus:           1,
		// zsh alone answers "a syntax error" differently depending on where it
		// read the text: 1 from -c, 126 from a file `.` opened.
		SourcedSyntaxErrorStatus: 126,
		// And the same number for a sourced file given up over an *error*,
		// where a fatal error that reaches the top of a script reports 1.
		SourcedFatalStatus: 126,
		// And a parse failure in a startup file leaves the status the line
		// before it left, where `.` returns 126 for the same text: measured
		// 2026-10-04 over a `.zshenv` whose last command left 0, 1 and 7,
		// the `-c` command sees 0, 1 and 7 (#5869).
		StartupParseFailureStatus: interp.StartupParseStatusKept,
		// And a script, standard input or a prompt keeps a failing status
		// over any parse failure: `sh -c "exit 7"` and then `echo )` ends a
		// script at 7. See interp.FailingStatusKeptOverAnyFailure (#5989).
		ParseFailureKeepsAFailingStatus: interp.FailingStatusKeptOverAnyFailure,
		// And a run-time failure in one is located as in a file even at a
		// prompt: `.zshrc:3: command not found` under `zsh -i`, as under
		// `zsh -i -c` (#5870).
		StartupFileLocatedAsAFileAtAPrompt: true,
		// The builtin the script wrote rather than a literal `.`, because
		// this shell has two names for it and says the one it was given.
		// Measured 2026-09-12 on zsh 5.9.2 with nothing of that name
		// anywhere: `source nope.sh` is `zsh:source:1: no such file or
		// directory: nope.sh` and `. nope.sh` is `zsh:.:1: …`. The name is
		// in the location either way — see NamesBuiltinInLocation — and
		// spelling it here lets the rule that strips a duplicated name reach
		// the second one too. A literal `.` survived it and made every
		// `source` failure read `zsh:source:1: .: no such file …`.
		DotCannotOpen:       "%[3]s: no such file or directory: %[1]s",
		DotCannotOpenStatus: 127,
		DotNoOperand:        "%[1]s: not enough arguments",
		DotNoOperandStatus:  1,
		// zsh leads with the reason, lowercased, and names the command after
		// it — the reverse of the other three.
		// zsh alone says something for a shift it survives; bash says
		// nothing, and the two that speak up here treat it as fatal.
		ShiftTooMany: "shift count must be <= $#",
		// A sentence of its own for the other end, naming neither the count
		// nor the word.
		ShiftNegativeCount: "argument to shift must be non-negative",
		// The two opt-in scope lints, whose switches the option table turns
		// on — `setopt warncreateglobal` and `setopt warnnestedvar`. Both
		// name the function twice, once in the location this dialect writes
		// in front of a diagnostic raised inside a body and once in the
		// sentence itself, which is the reference's own shape. Measured on
		// zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26:
		// `f() { gv=1 }; f` is `f: scalar parameter gv created globally in
		// function f` and `outer() { local lv=1; inner }; inner() { lv=2 }`
		// is `inner: scalar parameter lv set in enclosing scope in function
		// inner`.
		GlobalCreatedInAFunction:     "%[1]s parameter %[2]s created globally in function %[3]s",
		EnclosingScopeSetInAFunction: "%[1]s parameter %[2]s set in enclosing scope in function %[3]s",
		StdinLocation:                interp.LocationNameOnly,
		StdinBuiltinLocation:         interp.LocationBuiltinNameOnly,
		// See Diagnostics.StdinIsLocatedAsAPrompt.
		StdinIsLocatedAsAPrompt: true,
		CannotExecute:           "%[2]s: %[1]s",
		// A `#!` line naming an interpreter that is not there, which this
		// shell reads the file to find out about — and words the other way
		// round from bash, with the complaint before the name it could not
		// find. Measured 2026-09-25 on 5.9.2: `./bad` whose first line is
		// `#!/nonexistent/interp` is `zsh:1: ./bad: bad interpreter:
		// /nonexistent/interp: no such file or directory`, keeping the line
		// bash drops and numbering it 127 where bash says 126. A script
		// testing `$? -eq 127` for "not found" is reading that number.
		BadInterpreter:       "%[1]s: bad interpreter: %[2]s: %[3]s",
		BadInterpreterStatus: 127,
		// zsh says "command not found" for a bare name it could not resolve,
		// where the other three say "not found".
		// zsh leads with the complaint and names the command after it, for a
		// command word exactly as for `exec`.
		NotFound:     "command not found: %[1]s",
		ExecNotFound: "command not found: %[1]s",
		// The builtin's name comes from the location here, not the message.
		// No "kill:" in front of any of these: zsh puts the builtin's name in
		// the location instead, which NamesBuiltinInLocation already says.
		// No "printf:" in front: zsh puts the builtin in the location.
		// The reason first and the operand after it, which is the reverse of
		// everyone else — and lowercased, which LowercaseReason already says.
		// The sign is the wording's second verb here, where the other four
		// dialects spell a `-` into the format: this is the one shell that
		// reads a `+`-prefixed word as an option, so it is the one shell
		// whose complaint has two signs to write. `getopts a o +z` is `bad
		// option: +z` and `getopts a: o +a` is `argument expected after +a
		// option`. Measured 2026-09-17 on 5.9.2.
		GetoptsBadOption:       "bad option: %[2]s%[1]s",
		GetoptsMissingArgument: "argument expected after %[2]s%[1]s option",
		// Not a usage line at all here — this shell counts the operands and
		// says so, and it is the same sentence several of its builtins write
		// for too few words. Measured 2026-09-14: `<script>:getopts:1: not
		// enough arguments`, status 1 rather than the 2 the rest report, and
		// the script carries on (#2801).
		BuiltinUsage: map[string]string{
			"getopts": "getopts: not enough arguments",
		},
		GetoptsUsageStatus: 1,
		CdCannotChange:     "%[2]s: %[1]s",
		// A third operand to the substitution form, at this shell's ordinary
		// `cd` status rather than a usage one — which is where it parts
		// company with bash over the same sentence.
		CdTooManyOperands: "cd: too many arguments",
		// And `cd old new` where old is not in the current directory's path,
		// which names the operand where ksh93's does not.
		CdBadSubstitution:        "cd: string not in pwd: %[1]s",
		PrintfBadVerb:            "%[2]s: invalid directive",
		PrintfPositionOutOfRange: "%[1]d: argument specifier out of range",
		PrintfMissingVerb:        "%[1]s: invalid directive",
		UmaskBadMask:             "bad umask",
		// The builtin's name comes from the location, as everywhere in zsh.
		UnaliasNotFound: "no such hash table element: %[2]s",
		// `zsh:unalias:1: not enough arguments` — the location carries the
		// builtin's name, so the sentence does not.
		UnaliasNoPattern:       "not enough arguments",
		UnaliasAllWithOperands: "-a: too many arguments",
		UnaliasUsage:           "not enough arguments",
		// And this one is the exception to the line above: the builtin's
		// name is *not* in the location either. `alias -L` with
		// `aliases[x=y]=z` set writes `<file>:2: invalid alias 'x=y'
		// encountered while printing aliases` where its own option errors
		// are `<file>:alias:2: …`. Measured 2026-09-30; see
		// interp.Diagnostics.AliasListingInvalidName for the rest of the
		// table and interp.Runner.aliasListingSkipsTheName for where the
		// name is left out.
		AliasListingInvalidName: "invalid alias '%[1]s' encountered while printing aliases",
		UnsetNoOperands:         "%[1]s: not enough arguments",
		UnaliasNoOperandStatus:  1,
		// The builtin's name comes from the location here, as everywhere in
		// zsh, so it is not in the wording.
		UmaskBadSymbolicMode:     "bad symbolic mode permission: %[2]s",
		UmaskBadSymbolicOperator: "bad symbolic mode operator: %[2]s",
		UmaskBadOption:           "bad option: %[1]s",
		UmaskBadOptionStatus:     1,
		LetNoExpression:          "not enough arguments",
		UlimitBadOption:          "bad option: -%[1]s",
		UlimitBadNumber:          "invalid number: %[1]s",
		UlimitBadOptionStatus:    1,
		BuiltinBadOption:         "%[1]s: bad option: %[2]s",
		BadOptionNaming:          interp.BadOptionFirstUnknownLetter,
		WaitBadJob:               "wait: job not found: %[1]s",
		WaitBadJobStatus:         127,
		// The builtin's name rides the location, as ever.
		WaitNoSuchJob: "wait: %[1]s: no such job",
		KillNoSuchJob: "kill: %[1]s: no such job",
		// A spec that names nothing is worded by its shape, on every job
		// builtin alike; see Diagnostics.JobSpecNoCurrent for the rows.
		JobSpecNoCurrent:  "%[1]s: no current job",
		JobSpecNoPrevious: "%[1]s: no previous job",
		JobSpecNotFound:   "%[1]s: job not found: %[2]s",
		// The wording is the shared one; the number is not. Measured,
		// `jobs %9` reports 127 — a command that is not there — which is the
		// same number this shell's `wait` gives a job that is not there.
		NoSuchJobStatus:                  127,
		DisownNoCurrentJob:               "disown: no current job",
		WaitNotOurChild:                  "wait: pid %[1]d is not a child of this shell",
		TrapCouldNotParse:                "couldn't parse trap command",
		UmaskWhoAloneIsANumericComplaint: true,
		// The builtin's name comes from the location, so it is not in these.
		// The reason leads for `export` and `readonly` and trails for `unset`,
		// which is why this is a map.
		BuiltinBadName: map[string]string{
			// `set -A 1bad v` — the array letter's name operand. It gets the
			// leading-digit wording rather than this shell's usual "not
			// valid in this context", measured: `not an identifier: 1bad`.
			"set":      "not an identifier: %[2]s",
			"export":   "not valid in this context: %[2]s",
			"readonly": "not valid in this context: %[2]s",
			"unset":    "%[2]s: invalid parameter name",
			"local":    "not valid in this context: %[2]s",
			// `typeset` says the same as `local`, measured — and it is
			// reached only from `typeset -T`, which is the one shape of the
			// builtin that checks its operands are names. A bare
			// `typeset ':'` is refused in this shell and taken here; see the
			// issue filed from #1045.
			"typeset": "not valid in this context: %[2]s",
			"declare": "not valid in this context: %[2]s",
			// `integer` names itself in its own refusal here, where ksh93's
			// calls itself `typeset` — so it is an entry rather than a
			// rename. Measured: `integer 1x` is
			// `zsh:integer:1: not an identifier: 1x`.
			"integer": "not valid in this context: %[2]s",
			// `read` gets the leading-digit wording for every bad operand,
			// not just a numeric one: `read a-b` is `not an identifier: a-b`
			// where `export a-b` is `not valid in this context`. So there is
			// no BuiltinBadNameNumeric entry beside this — the two are the
			// same sentence here.
			"read": "not an identifier: %[2]s",
			// `printf -v` says exactly what `read` says, and says it to
			// `a-b` as well as to `1x` — measured 2026-09-17,
			// `printf -v 'a-b' %s Q` is `not an identifier: a-b`. So there
			// is no BuiltinBadNameNumeric row beside this one either.
			"printf": "not an identifier: %[2]s",
			// And `getopts`, which says the same to `1bad` and to `a-b`
			// alike — measured 2026-09-18, and the script ends on it, which
			// is Semantics.BadNameToGetoptsFatal (#3555).
			"getopts": "not an identifier: %[2]s",
		},
		// An operand that starts with a digit is a different complaint, for
		// the two that have one. `unset` says the same to both.
		BuiltinBadNameNumeric: map[string]string{
			"export":   "not an identifier: %[2]s",
			"readonly": "not an identifier: %[2]s",
			"local":    "not an identifier: %[2]s",
			"typeset":  "not an identifier: %[2]s",
			"declare":  "not an identifier: %[2]s",
			"integer":  "not an identifier: %[2]s",
		},
		// A word holding a bracket that never closes is judged by the
		// identifier rule rather than the context one, and the location
		// leaves the builtin out — see Diagnostics.BuiltinBadNameBracketed.
		// Measured 2026-09-12: `typeset 'm[a]b]'=v` is `<shell>:1: not an
		// identifier: m[a]b]` where `typeset 'a]'=v`, with no bracket to
		// open one, is `<shell>:typeset:1: not valid in this context: a]`.
		//
		// `export` is on the same side, measured 2026-09-19: `export
		// 'm[a]b]'=v` and `export 'a[1]+'=v` are both `<shell>:1: not an
		// identifier: <name>` where `export 'a]'=v` and `export 'x+'=v` keep
		// the builtin and the other wording. `readonly` is **not** — it
		// answers a bracketed name with `<name>: can't create readonly array
		// elements`, which is the element refusal rather than a name one, and
		// is filed on its own rather than spelled as a third wording here.
		BuiltinBadNameBracketed: map[string]string{
			"typeset": "not an identifier: %[2]s",
			"declare": "not an identifier: %[2]s",
			"local":   "not an identifier: %[2]s",
			"integer": "not an identifier: %[2]s",
			"export":  "not an identifier: %[2]s",
		},
		BuiltinBadOptionStatus: 1,
		PrintfUsage:            "not enough arguments",
		PrintfUsageStatus:      1,
		TrapBadSignal:          "undefined signal: %[1]s",
		KillNoSuchProcess:      "kill %[1]s failed: no such process",
		// The same sentence with the errno in it, for the sends this shell
		// makes that the two above do not cover: `kill -99 $$` is `kill
		// <pid> failed: invalid argument`, measured. The other two are this
		// wording with ESRCH and EPERM written out, which is why all three
		// read alike.
		KillSendFailed:            "kill %[1]s failed: %[4]s",
		KillNotPermitted:          "kill %[1]s failed: operation not permitted",
		KillInvalidSignal:         "unknown signal: %[3]s",
		KillIllegalOption:         "unknown signal: %[3]s",
		KillNotAPid:               "illegal pid: %[1]s",
		KillMissingSignalArgument: "%[1]s: argument expected",
		// The third route a signal spec takes here, and it is a wording of
		// its own rather than a spelling of either other one: a word written
		// where a number goes and that is not one. Verb 5 is the operand as
		// the script wrote it, which is what carries the dash for `kill -9x`
		// and leaves it off for `kill -n 9x`. Measured 2026-09-17 on zsh
		// 5.9.2 — and the listing hint does *not* follow this one, where it
		// follows both of the others (#3167).
		KillInvalidSignalNumber: "invalid signal number: %[5]s",
		KillUsage:               "not enough arguments",
		// -L, capitalized, and measured against the zsh the panel resolves:
		// 5.9.2 from Homebrew says -L where Apple's /bin/zsh 5.9 says -l.
		// Probing whichever zsh came first on PATH is how the lowercase one
		// got here.
		KillUnknownSignalHint: "type kill -L for a list of signals",
		// zsh is the one dialect that does not treat "nothing to signal" as a
		// usage error worth a different number from any other failure.
		KillUsageStatus: 1,
		// The only dialect that reads the word as an operator and says so:
		// `[ -Q x -a -n x ]` is `zsh:[:1: unknown condition: -Q` (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorNamed,
		TestUnaryExpected:       "unknown condition: %[1]s",
		TestBinaryExpected:      "condition expected: %[1]s",
		// The two-word form carries a prefix the longer ones do not, and
		// the refusal is read from the word counts of the segments between
		// the connectives. See interp/testcountedsegments.go for the rule
		// and the forty-five lists it was measured and then predicted over
		// (#4513).
		TestTwoWordOperatorExpected:                 "parse error: condition expected: %[1]s",
		TestRefusalCountsTheWordsBetweenConnectives: true,
		TestIntegerExpected:                         "integer expression expected: %[1]s",
		// The whole substitution as it was written, not its inside.
		ProcessSubstitutionNotInCondition: "process substitution %[1]s cannot be used here",
		// An empty `=~` right operand, which this shell refuses in its
		// engine's words and leaves the condition **false** over — 1, the
		// status a match that did not happen gives, where bash calls the
		// same operand a failure of the construct and ends at 2. Measured
		// 2026-09-18, `[[ abc =~ "" ]]` in a script file (#3279).
		EmptyRegexOperand:       "failed to compile regex: empty (sub)expression",
		EmptyRegexOperandStatus: 1,
		TestTooManyArguments:    "too many arguments",
		// `-a` and `-o` are words this shell knows — as the connectives —
		// so one standing where a unary operator belongs is a string with a
		// word left over rather than an operator it has never heard of.
		// Measured: `test -a f` is `too many arguments` and `test -Q f` is
		// `unknown condition: -Q`.
		TestConnectiveIsALeftoverWord: true,
		TestOperandExpected:           "argument expected",
		TestMissingBracket:            "']' expected",
		TimesDecimals:                 2,
		TimesArguments:                "times: too many arguments",
		// The `time` keyword reports one line per pipeline element that
		// forked, labeled with the element as written, and nothing for one
		// that did not — `time true` prints nothing at all here. A bare
		// `time` reports a `shell` and a `children` line in the same shape.
		TimeLayout: interp.TimePerCommand,
		// And the variable that shapes that layout's line, which this
		// column has and which stood empty here until #4910: the value was
		// supplied by #4912 and nothing read it, so a script setting
		// `TIMEFMT='%*E'` got the default layout here and its own there.
		//
		// The name alone would have been wrong. This shell's format is a
		// **different vocabulary** from the one `TIMEFORMAT` is written in
		// — they share `%%` and `%P` and nothing else, `%U` means the same
		// thing in both and renders differently, and the digit that is a
		// precision there is literal text here — so the verbs are named
		// beside the variable. See interp/timeformatunits.go for the sweep.
		TimeFormatVariable: "TIMEFMT",
		TimeFormatVerbs:    interp.TimeFormatVerbsElapsedWithUnits,
		TimeBare:           interp.TimeBareShellAndChildren,
		PathNotFound:       "no such file or directory: %[1]s",
		// zsh lowercases every strerror string it quotes, where the other
		// three print the C string as it comes.
		// zsh names the builtin that is speaking between its own name and the
		// line: `zsh:shift:1:`. A rule rather than a handful of cases, and
		// the first shell in the panel measured to do it — BusyBox ash is the
		// second, with a space after the colon rather than none, which is the
		// location style's own punctuation and not a second axis (#2761).
		NamesBuiltinInLocation: true,
		// Two refusals this shell locates as its own rather than the
		// builtin's, where the other shell that names builtins keeps the
		// name. `readonly r=1; unset r` is `zsh:2: read-only variable: r` and
		// `exec nosuchcmd` is `zsh:1: command not found: nosuchcmd` — the
		// second word for word what a bare command word reports. Both were
		// rules in the substrate until a dialect disagreed with them; see the
		// fields for the rows.
		UnsetReadonlyIsTheShellsOwn: true,
		ExecNotFoundIsTheShellsOwn:  true,
		// Except for a math complaint, which this shell writes as its own:
		// `let '1+'` is `zsh:1: bad math expression: …` where its `cd` is
		// `zsh:cd:1: …`, and `let` with no operand at all *is* the builtin's
		// and does say `zsh:let:1:`. See the field.
		ArithErrorNamesTheBuiltin: false,
		LowercaseReason:           true,
		DirectoryReason:           "Permission denied",
		// The kernel's `not a directory`, at the status of a path that is not
		// there (#6092). See interp.Diagnostics.NotADirectoryExecStatus.
		NotADirectoryExecStatus: 127,
		HashNotFound:            "no such command: %[1]s",
		HashNamedDirNotFound:    "hash: no such directory name: %[1]s",
		// `rehash` and `unhash` are this shell's own names for the two
		// halves of the table, and each refuses under its own name.
		// Measured 2026-09-18 on zsh 5.9.2: `rehash foo` is `too many
		// arguments`, `unhash` alone is `not enough arguments`, and every
		// table answers a name it has not got with the identical `no such
		// hash table element` — all three at 1 (#3109).
		RehashTooManyArguments: "too many arguments",
		UnhashNoOperands:       "not enough arguments",
		UnhashElementNotFound:  "no such hash table element: %[2]s",
		HashNamedDirBadName:    "hash: invalid character in directory name: %[1]s",
		// `ls=/bin/ls`, the shape an assignment would have — zsh and ksh93
		// both write the table that way.
		HashListing: interp.HashListingNameEqualsPath,
	}
}
