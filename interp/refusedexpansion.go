// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// refusedAtExpansion writes a parse failure the parser handed forward instead
// of raising, and stops the shell where a refused word stops it. It reports
// whether the word was answered here.
//
// The sentence is the one the parse would have written — the same kind, the
// same token and the same quoted word tail, through the same
// Diagnostics.ParseFailure the front end words a refused script with — because
// the failure is the same failure and only its *moment* moved. Rebuilding it
// here from the node would be a second wording of one rule, which is how the
// substitution's refusal came to place a body its own runner placed correctly.
//
// The status is the syntax one and not a fatal expansion's: the shell is
// refusing a word it could not read, and measured, the column that defers it
// ends at the number its parse failures end at rather than at the number a
// failed expansion leaves.
//
// See syntax.ParamExpr.RefusedAtExpansion and
// syntax.Dialect.FlagGroupRefusedAtExpansion for the rows and for the control
// that says the deferral is the flag group's alone.
func (r *Runner) refusedAtExpansion(e *syntax.ParamExpr) bool {
	if e == nil || e.RefusedAtExpansion == nil {
		return false
	}
	r.diagf("%s\n", r.diag().ParseFailure(e.RefusedAtExpansion))
	// The stop first and the number after it, because fatalQuiet writes the
	// dialect's generic fatal status over whatever it finds: the shell is
	// refusing a *word it could not read*, so it ends at the number its
	// parse failures end at. Measured, `ksh -c 'echo one; echo "${(f)x}";
	// echo two'` writes `one` and exits 3, where a fatal error there is 1.
	r.fatalQuiet()
	r.status = r.diag().SyntaxStatus()
	return true
}
