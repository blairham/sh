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
	s.UnterminatedBracket = interp.BracketBadPattern
	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location:          interp.LocationTightLine,
		BadSubstitution:   "bad substitution",
		BadPattern:        "bad pattern: %s",
		ReadonlyVariable:  "read-only variable: %s",
		SyntaxErrorStatus: 1,
	}
}

// Apply makes any adjustment that is not a vector value. zsh needs none.
func Apply(_ *interp.Runner) {}
