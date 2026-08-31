// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what zsh parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	d.FunctionKeywordParens = true
	return d
}

// Semantics is what zsh means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
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
	s.ArithFloat = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.ArrayBaseIsZero = interp.No
	s.DollarZeroInFunctionIsFunctionName = interp.Yes
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	s.UnterminatedBracket = interp.BracketBadPattern
	s.ExitTrapIsFunctionLocal = interp.Yes
	s.SignalHandlerSeesEarlierStatus = interp.Yes
	s.ExitArgument = interp.ExitArgLenient
	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location:          interp.LocationTightLine,
		BadSubstitution:   "bad substitution",
		BadPattern:        "bad pattern: %s",
		ReadonlyVariable:  "read-only variable: %s",
		TraceQuoting:      interp.QuoteShell,
		TraceStyle:        interp.TraceNameLine,
		TraceForHeader:    interp.TraceForAssign,
		SyntaxErrorStatus: 1,
		// zsh alone answers "a syntax error" differently depending on where it
		// read the text: 1 from -c, 126 from a file `.` opened.
		SourcedSyntaxErrorStatus: 126,
		DotCannotOpen:            ".: no such file or directory: %[1]s",
		DotCannotOpenStatus:      127,
		DotNoOperand:             ".: not enough arguments",
		DotNoOperandStatus:       1,
	}
}

// Apply adds what zsh has and the substrate does not.
//
// `source` is a synonym for `.`, which dash does not have at all — so the name
// is a dialect's answer, and it is the same function under a second name
// rather than a second implementation.
func Apply(r *interp.Runner) {
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
}
