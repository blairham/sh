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
	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		Location:          interp.LocationColonLine,
		SyntaxErrorStatus: 2,
	}
}
