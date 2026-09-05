// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package dash answers the substrate's questions the way dash does.
package dash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what dash parses.
func Dialect() syntax.Dialect {
	// dash is the POSIX shell language and nothing more.
	d := syntax.POSIX()
	// dash expands aliases in a script, with no option to turn on.
	d.ExpandAliases = true
	// Not a construct it adds but how it reads one it already has: a name
	// followed by `(` is a function definition here, whether or not the `)`
	// comes next, which is what decides the token a malformed one is blamed
	// on.
	d.FuncDefAtParen = true
	// And having committed, it checks: a name with punctuation is refused
	// once the parens close — `Bad function name`.
	d.FunctionNamePunctuation = false
	// One operator may stand where a case pattern belongs, opening an arm
	// that matches nothing — measured by running it, not inferred from the
	// error it causes elsewhere.
	d.CasePatternAcceptsOperator = true
	return d
}

// Semantics is what dash means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.Yes
	s.SetFTurnsOffGlobbing = interp.Yes
	// The one shell that refuses the -h letter POSIX names.
	s.SetHasTheHLetter = interp.No
	// Job control wants the tty: with none, `set -m` earns the remark
	// `can't access tty; job control turned off` — a remark, measured, not
	// a failure: the option stays off and `set` still reports 0.
	s.MonitorNeedsATerminal = interp.Yes
	// DefaultOptionLetters stays empty on purpose: measured, dash's `$-`
	// starts blank however it is invoked, save the route letter `s` on
	// standard input, which no dialect models.
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// $(( )) with nothing in it wants a primary and stops the script.
	s.EmptyArithExpressionIsAnError = interp.Yes
	// A bare read wants a name here, where the other three fill REPLY.
	s.ReadRequiresAVariableName = interp.Yes
	// The odd one out on `echo … | sh`: dash takes the program off standard
	// input in blocks and keeps what it took, so a `read` in the script
	// finds end of input and the data line is run as a command. The other
	// three read a line at a time and leave the rest on the descriptor.
	s.StdinProgramReadInBlocks = true
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter. ksh93
	// and zsh let `-s` name them instead.
	s.StdinOptionNamesTheOperands = interp.No
	// read takes -r and, alone among its letters, bash's -p prompt — an
	// argument, printed only to a terminal. The rest of bash's set (-s, the
	// counts, -d, -t, -u) is refused as unknown here.
	s.ReadOptions = "rp:"
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// dash single-quotes every listed value; it has no declare, so this
	// style exists for the two -p listings alone.
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysEscaped
	s.EchoInterpretsEscapes = interp.Yes
	s.LengthOfSpecialIsCount = interp.No
	s.UnterminatedBracket = interp.BracketNoMatch
	// `.` with no filename at all is not an error here: dash does nothing and
	// reports success, where the other three complain. Everything else about
	// `.` and `eval` is the POSIX answer, which dash keeps and the others have
	// each moved away from.
	s.DotWithNoOperandIsAnError = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.No
	s.SIGPrefixAccepted = interp.No
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PrintfTimeConversion = interp.No
	s.PrintfQuote = interp.PrintfQuoteAbsent
	// The three `$'…'` axes are left unanswered on purpose: dash has no
	// `$'…'` at all — `$'a\tb'` is the six characters it was written as,
	// dollar included — so the grammar refuses the form before any of them
	// can be asked. An answer here would be an invention.
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.Yes
	// dash has no options here at all, so every letter is refused.
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.No
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.No
	// The sole holdout on the pseudo-conditions: `trap … ERR` is refused
	// with the same words any word that names no signal gets, and DEBUG
	// and RETURN with it. Measured, and the refusal does not end the
	// script — the status is 1 and the next command runs.
	s.TrapHasErrCondition = interp.No
	s.TrapHasDebugCondition = interp.No
	s.TrapHasReturnCondition = interp.No
	// A subshell's listing shows only what survived the entry — the ignored
	// signals — in every boundary measured: `(trap)`, `$(trap)`, a pipeline
	// element and a background job all print nothing for a handled trap and
	// `trap -- '' INT` for an ignored one. The POSIX answer, restated here
	// because it was measured rather than assumed. Whether a kept listing
	// includes EXIT is left unanswered: nothing is ever kept to ask it of.
	s.SubshellKeepsTrapListing = interp.No
	s.PipelineElementKeepsTrapListing = interp.No
	s.BackgroundJobKeepsTrapListing = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.No
	s.UmaskPrintsFourDigits = interp.Yes
	// dash parses no options for `alias`, so `-p` is a name there.
	s.AliasParsesOptions = interp.No
	s.AliasHasPrintOption = interp.No
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskSetsWithoutAWho = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	s.ShiftReadsOptions = interp.No
	s.WaitReadsOptions = interp.Yes
	// Only numbers, `%%`, `%+` and `%-` resolve here: a `%name` is a job
	// that is not there. `wait` complains about it with its own wording and
	// status, and there is no -n and no disown at all.
	s.JobSpecsByName = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	s.WaitNWaitsForTheNextJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.CommandRejectsUnknownOption = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.No
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.Yes
	// A special builtin's failure is fatal, and a bad name is one — for all
	// three of them.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.DeclarationTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	s.JobsListFinishedJobs = interp.Yes

	// `jobs`' letters: POSIX's pair and nothing else. `-r`, `-s`, `-n` and
	// `-x` are all "Illegal option" here, which is why the letter set is a
	// dialect answer rather than one string in the engine.
	s.JobsOptions = "lp"
	s.JobsPidsOnlyOption = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	s.ExportCarriesFunctions = interp.No
	s.AnnouncesBackgroundJob = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.Yes
	s.ChildInterruptEndsTheScript = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.No

	// `local` reads no options at all here — `local -r x` declares a
	// variable named `-r` and then refuses it as the bad name it is — so
	// LocalOptions stays empty. A bare `local` writes nothing, and a bare
	// `set` lists the variables alone, every value single-quoted with an
	// embedded quote doubled out: `'quo'"'"'te'`.
	s.BareLocalListing = interp.BareLocalListsNothing
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteAlwaysDoubled
	// Unreachable behind the answer above — dash's `type` has no options at
	// all, so `-t` is a name — but the axis is answered rather than left
	// looking forgotten.
	s.TypeNamesTheKindWithDashT = interp.No

	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword:  "%[1]s is a shell keyword",
		TypeFunction: "%[1]s is a shell function",
		// No shell name and no location in front of it, alone among the
		// messages dash prints.
		TypeNotFound:           "%[1]s: not found",
		TypeNotFoundUnprefixed: true,
		// And on standard output, where the sentences about the names it
		// *could* account for go. dash has no option letters on `type` at
		// all, so `type -t f cd if ls` reads every word as a name and prints
		// five lines — two misses and three answers — in one stream, in
		// order. Measured: `type nope 1>/dev/null` prints nothing.
		TypeNotFoundOnStdout: true,
		// `command -V` says it the same way, shell's name and all.
		CommandVNotFound: "%[1]s: not found",
		// And a missing *command*'s status rather than a plain failure.
		TypeNotFoundStatus: 127,
		// `[1] + ` — a space each side of the marker — then a 27-wide state.
		// The command column is empty for a `&` job because dash kept no
		// text for one, which JobKeepsBackgroundCommand answers; a job dash
		// stopped itself does print its command here.
		JobLine: "[%[1]d] %[2]s %-27[3]s%[4]s",
		// `jobs -l`: the process id after the marker, and the state column
		// narrowed by exactly what the id took — dash is the one shell in
		// the panel that keeps the command column where it was. The 21 is
		// the measured width for a five-digit id; nothing sees the
		// difference, because dash's command column is empty for a `&` job
		// and what moves is trailing whitespace.
		JobLineLong: "[%[1]d] %[2]s %[3]d %-21[4]s%[5]s",
		// The words and nothing else — no process id, no command, and alone
		// among this shell's messages, no name and no line in front of it.
		KilledCommandNotice: "%[2]s",
		// The operator is part of the sentence here: this shell writes `-o`
		// whichever way it was asked.
		ParamNullOrNotSet:    "parameter not set or null",
		SetInvalidOptionName: "set: Illegal option -o %[1]s",
		// Said whichever spelling asked, so the verb goes unused; status 0,
		// the field's default, is what makes it a remark rather than an
		// error.
		MonitorDenied: "set: can't access tty; job control turned off",
		// Backticks alone: this shell numbers a `$( … )` body from the file
		// like the other three, and a backquoted one from one.
		BackquotedSubstitutionRestartsLines: true,
		KilledCommandNoticeUnprefixed:       true,
		JobRunning:                          "Running",
		// dash names the signal that stopped it rather than calling it
		// stopped: `Suspended: 18`, where 18 is SIGTSTP.
		JobStopped: "Suspended: %[1]d",
		JobDone:    "Done",
		JobExited:  "Done(%[1]d)",
		Location:   interp.LocationColonLine,
		// The one shell in the panel that tells neither failure from the
		// other: a script operand that is missing and one that will not open
		// share a wording and a status, and the status is the 2 it gives a
		// usage error rather than the 127 or 126 the other three reach for.
		// The reason is its own — `No such file`, from FileNotFound — and it
		// writes the line it has not reached yet, `<shell>: 0: cannot open …`,
		// which nothing else in the panel does.
		ScriptNotFound:               "cannot open %[1]s: %[2]s",
		ScriptNotFoundStatus:         2,
		ScriptNotReadableStatus:      2,
		InvocationNamesTheUnreadLine: true,
		// dash names what it wanted instead, and calls the token by its
		// class rather than by name.
		// A bad digit is not a diagnosis dash reaches at all: the literal
		// simply ends there and what follows is left over, so `08`, `09`,
		// `0778` and `0x1z` are all "expecting EOF" — the same sentence it
		// gives $((1 2)). Ours calls it a digit too great for its base and
		// dash words that as the leftovers it would have seen.
		DigitTooGreatForBase:  "expecting EOF",
		ArithOperandExpected:  "expecting primary",
		ArithOperatorExpected: "expecting EOF",
		SyntaxUnexpected:      "Syntax error: \"%[1]s\" unexpected",
		SyntaxUnexpectedWord:  "Syntax error: word unexpected",
		SyntaxExpecting:       " (expecting \"%[1]s\")",
		// dash does not name the token when it is a redirection operator.
		SyntaxRedirectUnexpected: "Syntax error: redirection unexpected",
		ForName:                  "Syntax error: Bad for loop variable",
		Unterminated:             "Syntax error: end of file unexpected (expecting \"%[4]s\")",
		// With nothing open there is nothing it could have been expecting,
		// and the parenthetical goes rather than standing empty.
		UnterminatedNoConstruct: "Syntax error: end of file unexpected",
		UnmatchedQuote:          "Syntax error: Unterminated quoted string",
		UnmatchedBackquote:      "Syntax error: EOF in backquote substitution",
		UnmatchedCmdSubst:       "Syntax error: end of file unexpected (expecting \")\")",
		UnmatchedBraceSubst:     "Syntax error: Missing '}'",
		SyntaxError:             "Syntax error: %s",
		BadSubstitution:         "Bad substitution",
		// The builtin in front, which its plain form does not have.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"export": true, "readonly": true},
		ReadonlyVariable:              "%s: is read only",
		InvalidNumber:                 "Illegal number: %s",
		NumericArgument:               "%[1]s: Illegal number: %[2]s",
		ArithError:                    "arithmetic expression: %[2]s: \"%[1]s\"",
		FileNotFound:                  "No such file",
		TestNamesFirstOperand:         true,
		// 2 rather than the 1 the other three report, for a read and a write
		// alike. Not fatal — the script carries on — so this is a different
		// question from FatalErrorStatusIsOne, which is about a failure that
		// ends the shell.
		RedirectFailureStatus: 2,
		// A verb, and a different one for each direction.
		CannotOpen:   "cannot open %[1]s: %[2]s",
		CannotCreate: "cannot create %[1]s: %[2]s",
		// The name twice — `dash: 1: echo: echo: I/O error` — and its own
		// fixed reason whatever the errno was, so the format never mentions
		// %[2]s. Measured on echo, printf, pwd and type: the doubling is the
		// builtin's, not the location's.
		BuiltinWriteError: "%[1]s: %[1]s: I/O error",
		// Its own text for ENOENT, and a different one each way: a read that
		// finds nothing is "No such file", a write that cannot make one is
		// "Directory nonexistent". The OS says "No such file or directory"
		// for both.
		DirectoryNotFound: "Directory nonexistent",
		ShiftTooMany:      "shift: can't shift that many",
		SyntaxErrorStatus: 2,
		// dash ends the script rather than reporting a status here, so only the
		// wording speaks — and it uses two, where the other three use one.
		//
		// The reason verb goes unused in the first: dash truncates strerror's
		// "No such file or directory" to "No such file" for this message
		// alone, so the text is spelled out rather than taken from the error.
		// An indexed format may ignore an argument, which is what makes that
		// safe.
		DotCannotOpen: ".: cannot open %[1]s: No such file",
		DotNotFound:   ".: %[1]s: not found",
		// A command word names no builtin; `exec` names itself.
		CannotExecute:     "%[1]s: %[2]s",
		ExecCannotExecute: "exec: %[1]s: %[2]s",
		ExecNotFound:      "exec: %[1]s: not found",
		// One message for either operator position.
		// dash names no pid at all, and prints a blank line after the one
		// message it has for a target — measured rather than assumed, because
		// an invisible trailing newline is exactly what a golden record is for.
		// No reason at all, so `cd` onto a file and `cd` onto nothing read
		// identically here — the one shell whose message cannot tell you
		// which it was.
		GetoptsBadOption:       "Illegal option -%[1]s",
		GetoptsMissingArgument: "No arg for -%[1]s option",
		// Nothing in front of it at all — the only diagnostic in the panel
		// that names neither the shell nor a line.
		GetoptsUnprefixed: true,
		CdCannotChange:    "cd: can't cd to %[1]s",
		CdStatus:          2,
		PrintfBadNumber:   "printf: %[1]s: expected numeric value",
		// Every complaint about an argument is 2 here, as it is elsewhere.
		PrintfBadVerbStatus: 2,
		PrintfBadVerb:       "printf: %[2]s: invalid directive",
		PrintfBadOption:     "printf: Illegal option %[1]s",
		UmaskBadMask:        "umask: Illegal number: %[1]s",
		// No shell and no line in front of either, which dash does almost
		// nowhere else.
		AliasNotFound:             "%[1]s: %[2]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasNotFound:           "%[1]s: %[2]s not found",
		UnaliasNotFoundUnprefixed: true,
		// dash quotes the whole argument back and does not say what in it
		// was wrong, so there is no operator wording to go with this.
		UmaskBadSymbolicMode:  "umask: Illegal mode: %[1]s",
		UmaskBadOption:        "umask: Illegal option %[1]s",
		UmaskBadMaskStatus:    2,
		UlimitBadOption:       "ulimit: Illegal option -%[1]s",
		UlimitBadNumber:       "ulimit: bad number",
		UlimitBadNumberStatus: 2,
		BuiltinBadOption:      "%[1]s: Illegal option %[2]s",
		// The same sentence kill already had for its own missing argument,
		// measured for the builtins' shared reader with `read -p`.
		OptionNeedsArgument:   "%[1]s: No arg for -%[2]s option",
		ShiftBadNumber:        "shift: Illegal number: %[1]s",
		WaitBadJob:            "wait: Illegal number: %[1]s",
		WaitBadJobStatus:      2,
		WaitNoSuchJob:         "wait: No such job: %[1]s",
		WaitNoSuchJobStatus:   2,
		KillNoSuchJob:         "kill: No such job: %[1]s",
		LocalOutsideAFunction: "local: not in a function",
		// One wording for all three, naming the part in front of any `=`.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: %[2]s: bad variable name",
			"readonly": "%[1]s: %[2]s: bad variable name",
			"unset":    "%[1]s: %[2]s: bad variable name",
			"local":    "%[1]s: %[2]s: bad variable name",
		},
		// A `local` name that starts with a digit is refused with the
		// builtin's name left off — `1y: bad variable name` against
		// `local: -r: bad variable name` — where its other bad names keep
		// it. Measured from both shapes.
		BuiltinBadNameNumeric: map[string]string{
			"local": "%[2]s: bad variable name",
		},
		BuiltinBadNameStatus: 2,
		// `ulimit -a`, row for row as the engine writes it.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "time(seconds)        ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "file(blocks)         ", Res: interp.ResourceFileSize},
			{Prefix: "data(kbytes)         ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "stack(kbytes)        ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "coredump(blocks)     ", Res: interp.ResourceCore},
			{Prefix: "memory(kbytes)       ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "locked memory(kbytes) ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "process              ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "nofiles              ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "vmemory(kbytes)      ", Res: interp.ResourceAddressSpace, Scale: 1024},
		},
		PrintfUsage:             "printf: usage: printf format [arg ...]",
		TrapBadSignal:           "trap: %[1]s: bad trap",
		TrapBadSignalUnprefixed: true,
		KillNoSuchProcess:       "kill: No such process\n",
		KillNotPermitted:        "kill: Operation not permitted\n",
		KillInvalidSignal:       "kill: invalid signal number or name: %[1]s",
		// The first character alone: dash stopped reading there.
		KillIllegalOption:         "kill: Illegal option -%[2]s",
		KillNotAPid:               "kill: Illegal number: %[1]s",
		KillMissingSignalArgument: "kill: No arg for %[1]s option",
		KillUsage: "kill: Usage: kill [-s sigspec | -signum | -sigspec] [pid | job]... or\n" +
			"kill -l [exitstatus]",
		// dash answers every complaint about its arguments with 2, which is the
		// one dialect where the option and the operand are not two questions.
		KillUsageStatus:      2,
		KillBadOptionStatus:  2,
		KillArgumentStatus:   2,
		TestUnaryExpected:    "%[2]s: %[1]s: unexpected operator",
		TestBinaryExpected:   "%[2]s: %[1]s: unexpected operator",
		TestIntegerExpected:  "%[2]s: Illegal number: %[1]s",
		TestTooManyArguments: "%[2]s: too many arguments",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing ]",
		// Six decimal places, the most of any shell in the panel.
		TimesDecimals: 6,
		// dash hands the path to execve rather than checking first, so a
		// directory comes back as a permission error.
		DirectoryReason:     "Permission denied",
		ReadArgCount:        "read: arg count",
		OptionListingHeader: "Current option settings",
		OptionListingWidth:  16,
		KillListing:         interp.KillListingZeroFirst,
		// And when that directory was the PATH search's only match, dash
		// names it in the message and still numbers the failure 127.
		DirectoryOnPathStatus: 127,
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(r *interp.Runner) {
	// dash has no `builtin`.
	r.Unregister("builtin")
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	// fc really is an external here: `command -v fc` answers /usr/bin/fc.
	r.Unregister("fc")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// dash has neither `typeset` nor `declare`. The core provides `typeset`
	// because three of the four do; the one that does not takes it away, the
	// same way ksh93 takes `local` away.
	r.Unregister("typeset")
	// dash has no disown: real process groups or not, the name is simply
	// not a builtin there and resolves like any other missing command.
	r.Unregister("disown")
	// And no `let`. The other three evaluate arithmetic with it; dash has
	// only `$(( ))`, and reports `let: not found` like any other command it
	// has never heard of.
	r.Unregister("let")
	// This shell has no `enable` either.
	r.Unregister("enable")
}
