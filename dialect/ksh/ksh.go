// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ksh answers the substrate's questions the way ksh93 does.
package ksh

import (
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ksh93 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// ksh93 expands them in a script too.
	d.ExpandAliases = true
	// ksh93 has neither `local` nor `declare`, so `local a=(x)` is the same
	// syntax error there that `echo a=(x)` is — the rule follows the name
	// into the shell that has it.
	d.DeclarationUtilities = map[string]bool{
		"typeset": true, "export": true, "readonly": true,
	}
	d.ParamIndirection = true
	// `$"..."`, the locale-translatable string: with no catalog it is a
	// plain double-quoted string with the `$` stripped. Not core, because
	// dash and zsh keep the `$` as a literal.
	d.DollarDoubleQuote = true
	// ksh93 alone refuses an unrecognized ${...} operator while reading the
	// script; the other three wait until the expansion is reached.
	d.BadSubstitutionAtParseTime = true
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
	// The measured ksh93 is 93u+ 2012, which has no `&>`. Later ksh93u+m
	// does, twelve years apart under the same name — which is the divergence
	// syntax.Dialect's own comment names as the reason its fields are called
	// after constructs rather than after shells. A dialect built for the
	// newer build sets this back to true; the panel measures the older one.
	d.AmpersandRedirect = false
	// `times` is a reserved word here, so `times foo` is a syntax error
	// rather than a builtin ignoring an argument — the one place in the panel
	// where which builtin a shell has changes what parses.
	d.TimesIsReserved = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// Extended patterns wherever a pattern may stand.
	d.ExtendedPattern = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// A bare `|` in a `=~` operand belongs to the regular expression.
	d.RegexTakesAlternation = true
	return d
}

// Semantics is what ksh93 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// Measured from a script file, where `echo $-` reports `hB`; ksh93's
	// route letters — `c` under -c, `s` when reading a command string or
	// standard input — are the front end's and stay unmodeled.
	s.DefaultOptionLetters = "hB"
	s.ArithIntegerOperatorRefusesFloat = interp.Yes
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.No
	s.SelectLayout = interp.SelectMenuVertical
	s.SelectPromptNeedsTerminal = interp.Yes
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	// It complains about `alias nope` and says nothing about `unalias nope`,
	// which is why these are two questions.
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.No
	s.AliasNotFoundStatusCounts = interp.Yes
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteWhenNeededDollar
	s.TrapQuoting = interp.ListingQuoteWhenNeededDollar
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.Yes
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskSetsWithoutAWho = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.Yes
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	s.ShiftReadsOptions = interp.Yes
	s.WaitReadsOptions = interp.Yes
	s.CommandRejectsUnknownOption = interp.No
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.Yes
	s.TrapBodyRunsWhatParsed = interp.No
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	s.TrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	s.ExitTrapFiresPastTheEnd = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.No
	// hash is an alias for `alias -t` here, and a name that resolves to
	// nothing is a silent success.
	s.HashReportsAMissingName = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// echo reads -n and -e; a word carrying -E is an operand. \e expands,
	// \x does not.
	s.EchoOptions = "ne"
	s.EchoExpandsEscEscape = interp.Yes
	// read takes -r and -s plus -A, whose array is the first operand where
	// bash's -a takes it as the option's argument, and the same -d, -n, -N,
	// -t and -u. A short -n is a success here; a short -N reports 1 and
	// leaves the variable empty.
	s.ReadOptions = "rsAd:n:N:t:u:"
	s.ReadPartialCountSucceeds = interp.Yes
	s.ReadExactCountKeepsPartial = interp.No
	// typeset in a keyword function hides the caller's value, as bash's
	// local does.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.Yes
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithInvalidOctalDigitIsError = interp.No
	s.IndirectionYieldsName = interp.Yes
	s.BraceExpansion = interp.Yes
	s.BracketCaretNegates = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	s.ExitArgument = interp.ExitArgLenient
	s.UnsetPositionalIsAllowed = interp.Yes
	s.TraceShowsItsOwnDisabling = interp.No
	s.TraceAssignmentsSeparately = interp.Yes
	// An unparseable `eval` is reported and survived here, unlike dash, but a
	// file `.` cannot open still ends the script — so the two halves of the
	// POSIX "a special builtin's failure is fatal" rule are answered
	// differently, which is why they are two axes.
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotPassesArguments = interp.Yes
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	s.SignalDeathStatusIsTwoFiftySix = interp.Yes
	s.PipefailOption = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.No
	// Alone in the panel: `PATH=` finds nothing here, where dash, bash and
	// zsh still search the current directory.
	s.EmptyPathIsTheCurrentDirectory = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.PrintfOutputPrecedesComplaint = interp.Yes
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCControl
	s.PrintfQuote = interp.PrintfQuoteSingle
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.Yes
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.Yes
	s.TrapPrintsBareWithConditions = interp.Yes
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	// The one dialect that refuses `trap EXIT`, and the refusal is fatal.
	s.TrapOneArgumentIsACondition = interp.No
	// ERR and DEBUG but not RETURN. Both follow the script into functions;
	// only DEBUG follows it into a subshell — measured, a command
	// substitution there captures the DEBUG handler's output and not the
	// ERR handler's.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.No
	s.ErrTrapRunsInsideFunctions = interp.Yes
	s.ErrTrapRunsInSubshells = interp.No
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugTrapRunsInSubshells = interp.Yes
	// `(trap)` and `$(trap)` still list the parent's traps, EXIT included —
	// measured, and the working state is still reset: `trap 'echo x' USR1;
	// (trap)` prints the trap it will not fire. A pipeline element or a
	// background job lists nothing: `trap | cat` is empty where `(trap)` is
	// not, which is the split issue #339 measured.
	s.SubshellKeepsTrapListing = interp.Yes
	s.KeptTrapListingIncludesExit = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	// Fatal to `export` and `readonly` and not to `unset`, which prints the
	// same kind of complaint, returns 1 and carries on. Not `unset` being
	// less special: a bad *option* to it is fatal, just above.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.DeclarationTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	s.JobsListFinishedJobs = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	s.ExportCarriesFunctions = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.Yes
	s.CdRefusesUnknownOption = interp.Yes
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.Yes
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes

	return s
}

// Diagnostics is how ksh93 reports failure.
// kshKillUsage is printed on its own for `kill` with no operands, and again
// after an unknown option — with no shell name or line in front of it either
// time, which is not how ksh93 prints anything else.
const kshKillUsage = "Usage: kill [-lL] [-n signum] [-s signame] job ...\n" +
	"   Or: kill [ options ] -l [arg ...]"

func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword: "%[1]s is a keyword",
		// ksh93's `type` is `whence -v`, and the message says so.
		TypeExternal:            "%[1]s is a tracked alias for %[2]s",
		TypeFunction:            "%[1]s is a function",
		TypeNotFound:            "whence: %[1]s: not found",
		JobStarted:              "[%[1]d]\t%[2]d",
		JobNoticeShowsAmpersand: true,
		// `[1] + ` then a 25-wide state, and the leading space on the
		// running one is ksh's own: a stopped job's `Stopped` starts a
		// column earlier than a running job's `Running`, and the two lines
		// end in the same place.
		JobLine: "[%[1]d] %[2]s %-25[3]s%[4]s",
		// The process id and the words, with a colon between them and no
		// command after: this shell names the line and the process but does
		// not say back what was running.
		KilledCommandNotice:  "%[1]d: %[2]s",
		ParamNull:            "parameter null",
		UnsetBadFunctionName: "unset: %[1]s: invalid function name",
		SetInvalidOptionName: "set: %[1]s: bad option(s)",
		SignalDescriptions:   signalDescriptions(),
		JobRunning:           " Running",
		JobStopped:           "Stopped",
		JobUnknownCommand:    "<command unknown>",
		// ksh93 lists a job that has already ended as "Running", and it is
		// not a reaping race — it still says so after `wait`. Recorded as
		// the word ksh uses rather than corrected, which would be inventing
		// a shell that does not exist. The leading space is ksh's too: see
		// JobLine below.
		JobDone:       " Running",
		JobDoneNotice: " Done",
		// The name first, then the verb with the OS string bracketed after
		// it — the same shape ksh93 uses for `.`, which DotCannotOpen
		// already says.
		CannotOpen: "%[1]s: cannot open [%[2]s]",
		// No BuiltinWriteError: `echo hi >&-` reports 1 here and says
		// nothing, which is the semantics axis answering and the wording
		// staying empty.
		// Measured: ksh93 reports a failed open at the line before the redirect.
		RedirectFailureLine: interp.LineBeforeRedirect,
		// A target that expanded to nothing gets neither the reason nor the
		// "create" wording, whichever direction the redirection was.
		EmptyRedirectTarget:         "%[1]s: cannot open",
		CannotCreate:                "%[1]s: cannot create [%[2]s]",
		ArithFailureStatus:          1,
		ArithInfinity:               "inf",
		ArithNotANumber:             "nan",
		ArithFloatDigits:            15,
		ArithFloatKeepsPoint:        false,
		SelectPrompt:                "#? ",
		Location:                    interp.LocationLineWordAfterFirst,
		TraceQuoting:                interp.QuoteDollar,
		ScriptLocation:              interp.LocationLineWord,
		BuiltinLocation:             interp.LocationBracketLineAfterFirst,
		ScriptBuiltinLocation:       interp.LocationBracketLine,
		ParseFailureNamesItsOwnLine: true,
		ReadonlyVariable:            "%s: is read only",
		ShiftTooMany:                "shift: %d: bad number",
		ArithError:                  "%[1]s: %[2]s",
		DivisionByZero:              "divide by zero",
		// ksh93 names the innermost keyword still awaiting a partner: `if`
		// on its own, and the `then` inside it once that has been consumed.
		EvalNaming:             interp.SourceBeforeLocation,
		SourceFileNaming:       interp.SourceBeforeLocation,
		SourceFileIsTheBuiltin: true,
		ArithOperandExpected:   "more tokens expected",
		ArithOperatorExpected:  "arithmetic syntax error",
		// ksh93 does not call this a bad substitution: it is a syntax error
		// naming the character it could not read.
		BadSubstitution:  "syntax error at line %[2]d: `%[1]s' unexpected",
		SyntaxUnexpected: "syntax error at line %[3]d: `%[1]s' unexpected",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus:     1,
		ForName:           "%[1]s: invalid variable name",
		Unterminated:      "syntax error at line %[6]d: `%[3]s' unmatched",
		SyntaxErrorStatus: 3,
		// The status is never reached — a file `.` cannot open ends the script
		// here — but the wording is, and it names the operand and the reason in
		// brackets rather than after a colon.
		DotCannotOpen:          ".: %[1]s: cannot open [%[2]s]",
		DotNoOperandUnprefixed: true,
		// A value that is not a number is a parameter name in ksh93, so the
		// failure is that the name is unset rather than that the text is not
		// a number.
		InvalidNumber: "%[1]s: parameter not set",
		// No name in front of it and no `.:` either: ksh93 prints a usage
		// line bare, the same way it prints `kill`'s.
		DotNoOperand:       "Usage: . [ options ] name [arg ...]",
		DotNoOperandStatus: 2,
		// The reason goes in brackets, as it does for `.`.
		// A command word names no builtin; `exec` names itself.
		CannotExecute:     "%[1]s: cannot execute [%[2]s]",
		ExecCannotExecute: "exec: %[1]s: cannot execute [%[2]s]",
		// Not "cannot execute": ksh93 distinguishes a missing command from one
		// that will not run, and only the second gets the brackets.
		ExecNotFound:           "exec: %[1]s: not found",
		GetoptsBadOption:       "-%[1]s: unknown option",
		GetoptsMissingArgument: "-%[1]s: argument expected",
		CdCannotChange:         "cd: %[1]s: [%[2]s]",
		// One message for both, where bash names which variable was missing.
		CdHomeNotSet:              "cd: bad directory",
		CdOldpwdNotSet:            "cd: bad directory",
		PrintfBadVerb:             "printf: %[1]s: unknown format specifier",
		PrintfBadOption:           "printf: %[1]s: unknown option",
		TrapConditionRequired:     "trap: condition(s) required",
		PrintfBadOptionShowsUsage: true,
		UmaskBadMask:              "umask: %[1]s: bad number",
		// The name leads and the builtin follows it, which is the reverse of
		// everyone else — and unprefixed.
		AliasNotFound:             "%[2]s: %[1]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasUsage:              "Usage: unalias [-a] name...",
		UnaliasUsageUnprefixed:    true,
		UmaskBadSymbolicMode:      "umask: %[1]s: bad format",
		UmaskBadOption:            "umask: %[1]s: unknown option",
		UmaskUsage:                "Usage: umask [-S] [mask]",
		UmaskUsageUnprefixed:      true,
		LetNoExpression:           "Usage: let [ options ] [expr ...]",
		LetNoExpressionStatus:     2,
		LetNoExpressionUnprefixed: true,
		UlimitBadOption:           "not supported",
		UlimitBadNumber:           "ulimit: %[1]s: parameter not set",
		BuiltinBadOption:          "%[1]s: %[2]s: unknown option",
		BadOptionNaming:           interp.BadOptionWholeWord,
		WaitBadJob:                "wait: %[1]s: Arguments must be %%job, process ids, or job pool names",
		WaitBadJobStatus:          1,
		// ksh93 has `--version` here, which this shell does not.
		UnimplementedOptionLetters: map[string]string{
			// ksh93 answers --version on most builtins, and has its own
			// letters for these two.
			"wait": "-",
			// What is left of read's letters: compound -C, the -p coprocess,
			// -S's csv splitting and -v's default text.
			"read": "-CpSv",
			"type": "-afpqv",
		},
		// ksh93's one sentence for a dead -u descriptor, the number not
		// named; the non-number wordings per letter are not modeled yet, so
		// those fall back to the substrate's.
		ReadBadFileDescriptor: "read: bad file unit number [Bad file descriptor]",
		// Two wordings, split between `export` and the other two, and the
		// operand quoted back as given.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: %[2]s: is not an identifier",
			"readonly": "%[1]s: %[2]s: invalid variable name",
			"unset":    "%[1]s: %[2]s: invalid variable name",
		},
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinUsage: map[string]string{
			"set":      "Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]",
			"export":   "Usage: export [-p] [name[=value]...]",
			"readonly": "Usage: readonly [-p] [name[=value]...]",
			"read": "Usage: read [-ACprsSv] [-d delim] [-u fd] [-t timeout] [-n count] [-N count]\n" +
				"            [var?prompt] [var ...]",
			"trap":  "Usage: trap [-p] [action condition ...]",
			"wait":  "Usage: wait [ options ] [job ...]",
			"jobs":  "Usage: jobs [ options ] [job ...]",
			"shift": "Usage: shift [ options ] [n]",
			"unset": "Usage: unset [-nfv] name...",
		},
		PrintfUsage:           "Usage: printf [ options ] format [string ...]",
		PrintfUsageUnprefixed: true,
		TrapBadSignal:         "trap: %[1]s: bad trap",
		KillNoSuchProcess:     "kill: %[1]s: no such process",
		KillNotPermitted:      "kill: %[1]s: permission denied",
		KillInvalidSignal:     "kill: %[1]s: unknown signal name",
		KillNotAPid:           "kill: %[1]s: Arguments must be %%job, process ids, or job pool names",
		KillUsage:             kshKillUsage,
		// An unknown option is a usage error to ksh93 in both senses: it prints
		// the usage after the complaint, and it reports the usage status where
		// an unknown signal *name* reports 1.
		KillIllegalOption:         "kill: -%[1]s: unknown option\n" + kshKillUsage,
		KillMissingSignalArgument: "kill: %[1]s: signame argument expected\n" + kshKillUsage,
		KillUsageStatus:           2,
		KillBadOptionStatus:       2,
		KillUsageUnprefixed:       true,
		KillTargetUnprefixed:      true,
		TestUnaryExpected:         "%[2]s: %[1]s: unknown operator",
		TestBinaryExpected:        "%[2]s: %[1]s: unknown operator",
		TestIntegerExpected:       "%[2]s: %[1]s: integer expected",
		TestTooManyArguments:      "%[2]s: too many arguments",
		TestOperandExpected:       "%[2]s: argument expected",
		TestMissingBracket:        "[: ']' missing",
		// Labeled lines, one figure each, and no children's times at all —
		// genuinely less information than the other three report.
		TimesLayout:   interp.TimesUserAndSystem,
		TimesDecimals: 2,
	}
}

// Apply adjusts the substrate to ksh93: one removal and one addition.
//
// Which builtins a shell provides is neither grammar nor a conflict of
// meaning, so it is not a Dialect flag or a Semantics axis. It is what the
// extension seam is for, and a dialect uses it exactly as anything else built
// on the substrate would — the difference being that this one also takes
// something away.
//
// `local` goes because ksh93 is the only shell in the panel without it: it
// spells the same idea `typeset` and reports `local` as a command that was not
// found, so a script finding it here would be relying on something the real
// shell does not have.
//
// `source` arrives because ksh93 has it as a synonym for `.` and dash does
// not, which makes the name a dialect's answer. It is the same function under
// a second name rather than a second implementation.
func Apply(r *interp.Runner) {
	// The `set -o` names beyond the ones every shell has.
	r.AddSetOptions(
		"braceexpand",
		"histexpand",
		"keyword",
		"privileged",
	)
	// ksh93 has a `builtin` of its own and it is a different command: it
	// *registers* builtins rather than running one, so `builtin echo hi`
	// there looks for a builtin called `hi`. Measured and not built, and
	// taking the name away is more honest than leaving bash's meaning under
	// it.
	r.Unregister("builtin")
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	// This shell has no `enable`.
	r.Unregister("enable")
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	// Three decimal places, where the two other shells that have SECONDS
	// report whole seconds.
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.FormatFloat(rr.SecondsFrom(), 'f', 3, 64)
	})
	r.Unregister("local")
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
}
