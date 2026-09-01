// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what zsh parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	d.FunctionKeywordParens = true
	// `}` is reserved wherever a word may stand here, which is what lets a
	// brace group close without a terminator — and what makes `echo }` a
	// syntax error rather than a brace on the output.
	d.CloseBraceAlwaysReserved = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// A bare `(a|b)` inside a pattern word, which makes `@(abc|xyz)` a
	// literal `@` followed by a group here rather than an extended pattern.
	d.PatternAlternation = true
	return d
}

// Semantics is what zsh means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.No
	s.ArithIntegerOperatorRefusesFloat = interp.No
	s.ArrayScalarIsTheWholeArray = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.No
	s.UnsetEndsTheProducedPipelineStatus = interp.Yes
	s.SelectLayout = interp.SelectMenuColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.SelectAssumesUnboundedWidth = interp.Yes
	s.SelectEofEndsPromptLine = interp.Yes
	s.SelectEofIsSuccess = interp.Yes
	s.SelectEofPrintsNewline = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	s.SplitParamExpansion = interp.No
	s.GlobExpansionResults = interp.No
	s.GlobNoMatchIsError = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	s.EchoInterpretsEscapes = interp.Yes
	s.ArithLeadingZeroIsOctal = interp.No
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithNameValueRecurses = interp.Yes
	s.BraceExpansion = interp.Yes
	s.BracketCaretNegates = interp.Yes
	s.EqualsExpansion = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.ArrayBaseIsZero = interp.No
	s.DollarZeroInFunctionIsFunctionName = interp.Yes
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// zsh and ksh93 drop the EXIT trap when an exec fails; dash and bash
	// still run it.
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	s.PipefailOption = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.Yes
	// Alone in refusing an argument to `times`; dash and bash ignore it.
	s.TimesRejectsArguments = interp.Yes
	s.UnterminatedBracket = interp.BracketBadPattern
	s.ExitTrapIsFunctionLocal = interp.Yes
	s.SignalHandlerSeesEarlierStatus = interp.Yes
	s.ExitArgument = interp.ExitArgLenient
	// A status that carries a count rather than a verdict: two dead targets
	// is 2.
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.Yes
	s.KillStatus = interp.KillStatusFailureCount
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	s.PrintfQuote = interp.PrintfQuoteBackslash
	s.GetoptsAssignmentRestartsWord = interp.No
	s.GetoptsClearsOptarg = interp.Yes
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.No
	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// The reason first and the name after it, which is zsh's shape and
		// nobody else's. Lowercased, which LowercaseReason already says.
		// Same either way — zsh does not distinguish opening from creating.
		CannotOpen:               "%[2]s: %[1]s",
		CannotCreate:             "%[2]s: %[1]s",
		NotABuiltin:              "no such builtin: %[1]s",
		CommandStringParsedWhole: true,
		ArithInfinity:            "Inf",
		ArithNotANumber:          "NaN",
		ArithFloatDigits:         17,
		ArithFloatKeepsPoint:     true,
		SelectPrompt:             "?# ",
		Location:                 interp.LocationTightLine,
		BadSubstitution:          "bad substitution",
		BadPattern:               "bad pattern: %s",
		ReadonlyVariable:         "read-only variable: %s",
		TraceQuoting:             interp.QuoteShell,
		TraceStyle:               interp.TraceNameLine,
		TraceForHeader:           interp.TraceForAssign,
		// zsh names the last token it read and nothing else.
		EvalNaming:       interp.SourceReplacesShell,
		SourceFileNaming: interp.SourceReplacesShell,
		EvalSourceName:   "(eval)",
		// zsh does not quote the expression, where the other three do.
		ArithError:            "%[2]s",
		ArithOperandExpected:  "bad math expression: operand expected at end of string",
		ArithOperatorExpected: "bad math expression: operator expected at `%[1]s'",
		SyntaxUnexpected:      "parse error near `%[1]s'",
		ForName:               "parse error near `%[1]s'",
		Unterminated:          "parse error near `%[5]s'",
		SyntaxErrorStatus:     1,
		// zsh alone answers "a syntax error" differently depending on where it
		// read the text: 1 from -c, 126 from a file `.` opened.
		SourcedSyntaxErrorStatus: 126,
		DotCannotOpen:            ".: no such file or directory: %[1]s",
		DotCannotOpenStatus:      127,
		DotNoOperand:             ".: not enough arguments",
		DotNoOperandStatus:       1,
		// zsh leads with the reason, lowercased, and names the command after
		// it — the reverse of the other three.
		// zsh alone says something for a shift it survives; bash says
		// nothing, and the two that speak up here treat it as fatal.
		ShiftTooMany:  "shift count must be <= $#",
		CannotExecute: "%[2]s: %[1]s",
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
		GetoptsBadOption:          "bad option: -%[1]s",
		GetoptsMissingArgument:    "argument expected after -%[1]s option",
		CdCannotChange:            "%[2]s: %[1]s",
		PrintfBadVerb:             "%[2]s: invalid directive",
		PrintfUsage:               "not enough arguments",
		PrintfUsageStatus:         1,
		TrapBadSignal:             "undefined signal: %[1]s",
		KillNoSuchProcess:         "kill %[1]s failed: no such process",
		KillNotPermitted:          "kill %[1]s failed: operation not permitted",
		KillInvalidSignal:         "unknown signal: %[3]s",
		KillIllegalOption:         "unknown signal: %[3]s",
		KillNotAPid:               "illegal pid: %[1]s",
		KillMissingSignalArgument: "%[1]s: argument expected",
		KillUsage:                 "not enough arguments",
		// -L, capitalized, and measured against the zsh the panel resolves:
		// 5.9.2 from Homebrew says -L where Apple's /bin/zsh 5.9 says -l.
		// Probing whichever zsh came first on PATH is how the lowercase one
		// got here.
		KillUnknownSignalHint: "type kill -L for a list of signals",
		// zsh is the one dialect that does not treat "nothing to signal" as a
		// usage error worth a different number from any other failure.
		KillUsageStatus:      1,
		TestUnaryExpected:    "unknown condition: %[1]s",
		TestBinaryExpected:   "condition expected: %[1]s",
		TestIntegerExpected:  "integer expression expected: %[1]s",
		TestTooManyArguments: "too many arguments",
		TestOperandExpected:  "argument expected",
		TestMissingBracket:   "']' expected",
		TimesDecimals:        2,
		TimesArguments:       "times: too many arguments",
		PathNotFound:         "no such file or directory: %[1]s",
		// zsh lowercases every strerror string it quotes, where the other
		// three print the C string as it comes.
		// zsh names the builtin that is speaking between its own name and the
		// line: `zsh:shift:1:`. A rule rather than a handful of cases, and the
		// only shell in the panel that does it.
		NamesBuiltinInLocation: true,
		LowercaseReason:        true,
		DirectoryReason:        "Permission denied",
	}
}

// Apply adds what zsh has and the substrate does not.
//
// `source` is a synonym for `.`, which dash does not have at all — so the name
// is a dialect's answer, and it is the same function under a second name
// rather than a second implementation.
func Apply(r *interp.Runner) {
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("pipestatus")
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
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
