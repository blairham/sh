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
	// `.` with no filename at all is not an error here: dash does nothing and
	// reports success, where the other three complain. Everything else about
	// `.` and `eval` is the POSIX answer, which dash keeps and the others have
	// each moved away from.
	s.DotWithNoOperandIsAnError = interp.No
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
		ExecFailed:    "exec: %[1]s: %[2]s",
		ExecNotFound:  "exec: %[1]s: not found",
		// dash hands the path to execve rather than checking first, so a
		// directory comes back as a permission error.
		ExecDirectoryReason: "Permission denied",
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(_ *interp.Runner) {}
