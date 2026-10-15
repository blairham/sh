// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// zshTables is how many names Apply registers into each of the runner's
// per-name tables, handed to [interp.Runner.ReserveTables] before it
// registers any, so that each table is made once at the size it ends at
// instead of doubling its way there at every start (#6491).
//
// Measured after Apply on a fresh runner, the largest of the four emulations
// a shell can start in: 121 parameters, 94 builtins, 80 scope-fixed names,
// 79 kind-fixed, 48 math functions with 47 of them withdrawn, 42 of the
// shell's own, 40 deferred, 36 declarations, 30 withdrawn builtins and 24
// environment entries not adopted. Each is rounded up so that adding a name
// does not need this edited; TestApplyReservesTheTablesItFills says which
// field to raise when one is outgrown, and which to lower when a table has
// shrunk well under it.
var zshTables = interp.TableSizes{
	Parameters:             160,
	Builtins:               128,
	KindFixed:              96,
	ScopeFixed:             96,
	ShellOwn:               56,
	Deferred:               48,
	MathFunctions:          64,
	WithdrawnMathFunctions: 64,
	WithdrawnBuiltins:      40,
	NotAdopted:             32,
	Declarations:           48,
}
