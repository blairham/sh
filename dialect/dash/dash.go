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
	return syntax.POSIX()
}

// Semantics is what dash means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.EchoInterpretsEscapes = interp.Yes
	s.LengthOfSpecialIsCount = interp.No
	s.UnterminatedBracket = interp.BracketNoMatch
	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location:          interp.LocationColonLine,
		SyntaxError:       "Syntax error: %s",
		BadSubstitution:   "Bad substitution",
		ReadonlyVariable:  "%s: is read only",
		InvalidNumber:     "Illegal number: %s",
		NumericArgument:   "%[1]s: Illegal number: %[2]s",
		ArithError:        "arithmetic expression: %[2]s: \"%[1]s\"",
		CannotOpen:        "cannot open %s: %s",
		ShiftTooMany:      "shift: can't shift that many",
		SyntaxErrorStatus: 2,
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(_ *interp.Runner) {}
