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
	// Not a construct it adds but how it reads one it already has: a name
	// followed by `(` is a function definition here, whether or not the `)`
	// comes next, which is what decides the token a malformed one is blamed
	// on.
	d.FuncDefAtParen = true
	// One operator may stand where a case pattern belongs, opening an arm
	// that matches nothing — measured by running it, not inferred from the
	// error it causes elsewhere.
	d.CasePatternAcceptsOperator = true
	return d
}

// Semantics is what dash means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.EchoInterpretsEscapes = interp.Yes
	s.LengthOfSpecialIsCount = interp.No
	s.UnterminatedBracket = interp.BracketNoMatch
	// `.` with no filename at all is not an error here: dash does nothing and
	// reports success, where the other three complain. Everything else about
	// `.` and `eval` is the POSIX answer, which dash keeps and the others have
	// each moved away from.
	s.DotWithNoOperandIsAnError = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.KillListAcceptsName = interp.No
	s.SIGPrefixAccepted = interp.No
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	s.PrintfQuote = interp.PrintfQuoteAbsent
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.Yes
	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location: interp.LocationColonLine,
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
		ForName:               "Syntax error: Bad for loop variable",
		Unterminated:          "Syntax error: end of file unexpected (expecting \"%[4]s\")",
		SyntaxError:           "Syntax error: %s",
		BadSubstitution:       "Bad substitution",
		ReadonlyVariable:      "%s: is read only",
		InvalidNumber:         "Illegal number: %s",
		NumericArgument:       "%[1]s: Illegal number: %[2]s",
		ArithError:            "arithmetic expression: %[2]s: \"%[1]s\"",
		FileNotFound:          "No such file",
		TestNamesFirstOperand: true,
		CannotOpen:            "cannot open %s: %s",
		ShiftTooMany:          "shift: can't shift that many",
		SyntaxErrorStatus:     2,
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
		PrintfBadVerbStatus:     2,
		PrintfBadVerb:           "printf: %[2]s: invalid directive",
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
		TestUnaryExpected:    "test: %[1]s: unexpected operator",
		TestBinaryExpected:   "test: %[1]s: unexpected operator",
		TestIntegerExpected:  "test: Illegal number: %[1]s",
		TestTooManyArguments: "test: too many arguments",
		TestOperandExpected:  "test: argument expected",
		TestMissingBracket:   "[: missing ]",
		// Six decimal places, the most of any shell in the panel.
		TimesDecimals: 6,
		// dash hands the path to execve rather than checking first, so a
		// directory comes back as a permission error.
		DirectoryReason: "Permission denied",
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(_ *interp.Runner) {}
