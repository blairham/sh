// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package bash answers the substrate's questions the way bash 5 does.
package bash

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what bash 5 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// bash expands them interactively and needs `shopt -s expand_aliases`
	// otherwise, which is not modeled yet.
	d.ExpandAliases = false
	// The utilities that take an array assignment as an operand. bash has
	// all five.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	d.CaseContinue = true
	d.ParamCaseChange = true
	d.ParamIndirection = true
	d.FunctionKeywordParens = true
	// A name followed by `(` is a function definition here, whether or not
	// the `)` comes next.
	d.FuncDefAtParen = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// A bare `|` in a `=~` operand belongs to the regular expression.
	d.RegexTakesAlternation = true
	return d
}

// Semantics is what bash 5 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	s.ArrayScalarIsTheWholeArray = interp.No
	s.AssignmentUpdatesPipelineStatus = interp.Yes
	s.UnsetEndsTheProducedPipelineStatus = interp.No
	s.SelectLayout = interp.SelectMenuVerticalThenColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.AliasQuoteAlwaysEscaped
	s.SelectAssumesUnboundedWidth = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithNameValueRecurses = interp.Yes
	s.IndirectionYieldsName = interp.No
	s.BraceExpansion = interp.Yes
	s.BracketCaretNegates = interp.Yes
	s.RegexQuotingMakesLiteral = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	s.TypesetLocalNeedsKeywordFunction = interp.No
	s.ReadonlyReassignmentFatal = interp.No
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// The only shell in the panel that looks in the current directory once
	// PATH has missed. Measured: `PATH=/usr/bin:/bin; . f.sh` finds an f.sh in
	// the current directory here, and is "not found" in the other three.
	s.DotFallsBackToCurrentDirectory = interp.Yes
	s.ExecFailureRunsExitTrap = interp.Yes
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.Yes
	s.PipefailOption = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	s.ExitArgument = interp.ExitArgNumeric
	s.TraceAssignmentsSeparately = interp.Yes
	// bash reports success if it signaled anything at all, where the others
	// count failures one way or another.
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnySuccess
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	s.PrintfQuote = interp.PrintfQuoteBackslash
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.Yes
	s.UlimitBlockIsKilobyte = interp.Yes
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// bash reports every operand that is not a name, exports the ones that
	// are, and carries on with a status of 1.
	s.BadNameToDeclarationFatal = interp.No
	s.BadNameToUnsetFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	// bash 5.3's bare `unset` checks nothing — `unset 1x`, `unset "a b"` and
	// `unset -- -` are all quiet — while `unset -v 1x` refuses. bash 3.2
	// refused all of them, so the panel's two bash columns differ here.
	s.UnsetNameOperands = interp.AnythingIsAName
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	s.JobsListFinishedJobs = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.Yes

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	s.ExportCarriesFunctions = interp.Yes
	s.AnnouncesBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.Yes
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.Yes

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.Yes
	s.TypeEndsOptionsWithDashDash = interp.Yes

	return s
}

// Diagnostics is how bash 5 reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword:  "%[1]s is a shell keyword",
		TypeFunction: "%[1]s is a function",
		TypeNotFound: "type: %[1]s: not found",
		// The target as it was written, not as it expanded.
		AmbiguousRedirect: "%[1]s: ambiguous redirect",
		JobStarted:        "[%[1]d] %[2]d",
		// Measured from a terminal: `[1]+` then two spaces, the state in a
		// 27-wide column, then the command — with the `&` back on it while
		// the job runs and gone once it has ended.
		ParamNullOrNotSet: "parameter null or not set",
		JobLine:           "[%[1]d]%[2]s  %-27[3]s%[4]s",
		// The same twenty-seven-column state field the listing above uses,
		// with the process id in front of it rather than the job number: one
		// formatter, said in two places.
		KilledCommandNotice:      "%5[1]d %-27[2]s%[3]s",
		JobRunning:               "Running",
		JobStopped:               "Stopped",
		JobDone:                  "Done",
		JobExited:                "Exit %[1]d",
		JobRunningShowsAmpersand: true,
		// No verb at all: the name, then the OS string. Same either way —
		// bash does not distinguish opening from creating.
		CannotOpen: "%[1]s: %[2]s",
		// Measured: bash reports a failed open at the redirect's own line.
		RedirectFailureLine:     interp.LineOfRedirect,
		CannotCreate:            "%[1]s: %[2]s",
		NamesTheInputInLocation: true,
		EchoesTheOffendingLine:  true,
		SelectPrompt:            "#? ",
		Location:                interp.LocationLineWord,
		NotFound:                "%s: command not found",
		UnboundVariable:         "%s: unbound variable",
		UnboundPositional:       "$%s: unbound variable",
		NumericArgument:         "%[1]s: %[2]s: numeric argument required",
		ArithError:              `%[1]s: %[2]s (error token is "%[3]s")`,
		DivisionByZero:          "division by 0",

		// bash reserves its generic arithmetic wording for operands that are
		// not literals, so a bad digit gets a reason of its own.
		DigitTooGreatForBase: "value too great for base",
		TraceQuoting:         interp.QuoteShell,
		TraceForHeader:       interp.TraceForSource,
		// bash names the construct and the line it opened on, and nothing
		// about what would have closed it.
		EvalNaming:                 interp.SourceBeforeLocation,
		SourceFileNaming:           interp.SourceReplacesShell,
		UnterminatedEndsOnNextLine: true,
		ArithOperandExpected:       "arithmetic syntax error: operand expected",
		ArithOperatorExpected:      "arithmetic syntax error in expression",
		ArithBadOperator:           "arithmetic syntax error: invalid arithmetic operator",
		ArithFailureStatus:         1,
		SyntaxUnexpected:           "syntax error near unexpected token `%[1]s'",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus:     1,
		ForName:           "`%[1]s': not a valid identifier",
		Unterminated:      "syntax error: unexpected end of file from `%[1]s' command on line %[2]d",
		SyntaxErrorStatus: 2,
		// Measured: `.` of a file it cannot open reports 1 and carries on,
		// where a missing operand is 2 — two numbers for what reads like one
		// failure, which is why they are two fields.
		DotCannotOpen:       "%[1]s: %[2]s",
		DotCannotOpenStatus: 1,
		// bash names neither the builtin nor the operation: just the command
		// and the reason, the same shape it uses for `.`.
		CannotExecute: "%[1]s: %[2]s",
		// bash names the builtin only when the command was not found at all.
		ExecNotFound: "exec: %[1]s: not found",
		// bash is the only one that says which *kind* of operator it wanted.
		TestUnaryExpected:    "%[2]s: %[1]s: unary operator expected",
		TestBinaryExpected:   "%[2]s: %[1]s: binary operator expected",
		TestIntegerExpected:  "%[2]s: %[1]s: integer expected",
		TestTooManyArguments: "%[2]s: too many arguments",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing `]'",
		// `kill` puts the process in parentheses and the reason after a dash,
		// which is the only wording in the panel a script could not confuse
		// with a message about a signal name.
		GetoptsBadOption:       "illegal option -- %[1]s",
		GetoptsMissingArgument: "option requires an argument -- %[1]s",
		// bash names itself and no line here, where it gives a line to
		// everything else it says.
		GetoptsNamesNoLine:        true,
		CdCannotChange:            "cd: %[1]s: %[2]s",
		CdHomeNotSet:              "cd: HOME not set",
		CdOldpwdNotSet:            "cd: OLDPWD not set",
		PrintfBadNumber:           "printf: %[1]s: invalid number",
		PrintfBadVerb:             "printf: `%[1]s': invalid format character",
		PrintfBadOption:           "printf: %[1]s: invalid option",
		PrintfBadOptionShowsUsage: true,
		LetNoExpression:           "let: expression expected",
		UlimitBadOption:           "ulimit: -%[1]s: invalid option",
		UlimitBadNumber:           "ulimit: %[1]s: invalid number",
		BuiltinBadOption:          "%[1]s: %[2]s: invalid option",
		// One wording for all three, and the operand quoted back exactly as
		// given: `export 1x=v` says `1x=v', not `1x'.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: `%[2]s': not a valid identifier",
			"readonly": "%[1]s: `%[2]s': not a valid identifier",
			"unset":    "%[1]s: `%[2]s': not a valid identifier",
		},
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinUsage: map[string]string{
			"export":   "export: usage: export [-fn] [name[=value] ...] or export -p [-f]",
			"readonly": "readonly: usage: readonly [-aAf] [name[=value] ...] or readonly -p",
			"unset":    "unset: usage: unset [-f] [-v] [-n] [name ...]",
		},
		PrintfUsage:            "printf: usage: printf [-v var] format [arguments]",
		UmaskBadMask:           "umask: %[1]s: octal number out of range",
		AliasNotFound:          "%[1]s: %[2]s: not found",
		UnaliasNotFound:        "%[1]s: %[2]s: not found",
		AliasListPrefix:        "alias ",
		UnaliasUsage:           "unalias: usage: unalias [-a] name [name ...]",
		UnaliasUsageUnprefixed: true,
		// bash names the character and says which kind it wanted.
		UmaskBadSymbolicMode:     "umask: `%[2]s': invalid symbolic mode character",
		UmaskBadSymbolicOperator: "umask: `%[2]s': invalid symbolic mode operator",
		UmaskBadOption:           "umask: %[1]s: invalid option",
		UmaskUsage:               "umask: usage: umask [-p] [-S] [mode]",
		UmaskUsageUnprefixed:     true,
		// The usage carries no location, as `kill`'s does not.
		PrintfUsageUnprefixed:     true,
		TrapBadSignal:             "trap: %[1]s: invalid signal specification",
		KillNoSuchProcess:         "kill: (%[1]s) - No such process",
		KillNotPermitted:          "kill: (%[1]s) - Operation not permitted",
		KillInvalidSignal:         "kill: %[1]s: invalid signal specification",
		KillIllegalOption:         "kill: %[1]s: invalid signal specification",
		KillNotAPid:               "kill: `%[1]s': not a pid or valid job spec",
		KillMissingSignalArgument: "kill: %[1]s: option requires an argument",
		KillUsage: "kill: usage: kill [-s sigspec | -n signum | -sigspec] pid | jobspec ... " +
			"or kill -l [sigspec]",
		// The usage is the one bash diagnostic with no location in front of
		// it — every other message here carries "bash: line N:".
		KillUsageUnprefixed: true,
		KillUsageStatus:     2,
		TimesDecimals:       3,
		// A path that is not there is the OS reason and does not name the
		// builtin; a bare name off PATH does the reverse.
		PathNotFound: "%[1]s: No such file or directory",
		// bash names the path it tried, absolute, where the other three
		// report the operand as written.
		NamesResolvedPath: true,
		// Two lines, which is bash rather than a mistake: it prints the
		// complaint and then a usage line, and only the first carries the
		// shell's own prefix.
		DotNoOperand: ".: filename argument required\n" +
			".: usage: . [-p path] filename [arguments]",
		DotNoOperandStatus: 2,
	}
}

// Apply adds what bash has and the substrate does not.
//
// `source` is a synonym for `.`, and is a dialect's answer rather than the
// core's because dash does not have it at all — `command -v source` reports
// "not found" there. It is the same function under a second name rather than a
// second implementation, which is the only way the two cannot drift apart.
func Apply(r *interp.Runner) {
	// The `set -o` names this shell has and the others do not all have,
	// measured by asking each of the four to turn every name off. This one
	// has the most, and five of them belong to it alone.
	r.AddSetOptions(
		"braceexpand",
		"errtrace",
		"functrace",
		"hashall",
		"histexpand",
		"history",
		"interactive-comments",
		"keyword",
		"onecmd",
		"physical",
		"posix",
		"privileged",
	)
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("PIPESTATUS")
	// Parameters bash provides and the others do not all have. Which
	// variables a shell supplies is the same kind of question as which
	// builtins it has, so it is answered here rather than as an axis.
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	// Where the script is, which is a stack rather than a value — see
	// callstack.go for why it cannot be stored.
	registerCallStack(r)
	// A function carried to a child through the environment, under the name
	// bash gives it. The other three do not carry functions at all.
	r.SetFunctionExport("BASH_FUNC_", "%%")
	// And how they are laid out, which is this shell's taste rather than
	// anything the printer should know.
	r.SetFunctionLayout(FunctionLayout(), ExportedFunctionLayout())
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// `declare` is `typeset` under a second name rather than a second
	// implementation. ksh93 has only the older name and dash has neither, so
	// which names exist is a dialect's answer and not an axis.
	if typeset, ok := r.Builtin("typeset"); ok {
		r.Register("declare", typeset)
		// And the assignment rule follows the name: `declare x=*` stores the
		// character here, where in a shell without the name it would be an
		// ordinary command with an ordinary globbed argument.
		r.SetDeclaring("declare")
	}
}
