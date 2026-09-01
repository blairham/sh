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
	d.ParamIndirection = true
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
	return d
}

// Semantics is what ksh93 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.SetFTurnsOffGlobbing = interp.Yes
	s.ArithIntegerOperatorRefusesFloat = interp.Yes
	s.ArrayScalarIsTheWholeArray = interp.No
	s.SelectLayout = interp.SelectMenuVertical
	s.SelectPromptNeedsTerminal = interp.Yes
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectEofPrintsNewline = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
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
	// Alone in the panel: `PATH=` finds nothing here, where dash, bash and
	// zsh still search the current directory.
	s.EmptyPathIsTheCurrentDirectory = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.Yes
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
		ArithFailureStatus:          1,
		ArithInfinity:               "inf",
		ArithNotANumber:             "nan",
		ArithFloatDigits:            15,
		ArithFloatKeepsPoint:        false,
		SelectPrompt:                "#? ",
		Location:                    interp.LocationNone,
		TraceQuoting:                interp.QuoteDollar,
		ScriptLocation:              interp.LocationLineWord,
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
		CdHomeNotSet:          "cd: bad directory",
		CdOldpwdNotSet:        "cd: bad directory",
		PrintfBadVerb:         "printf: %[1]s: unknown format specifier",
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
		TestUnaryExpected:         "test: %[1]s: unknown operator",
		TestBinaryExpected:        "test: %[1]s: unknown operator",
		TestIntegerExpected:       "test: %[1]s: integer expected",
		TestTooManyArguments:      "test: too many arguments",
		TestOperandExpected:       "test: argument expected",
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
