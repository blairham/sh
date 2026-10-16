// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// bareRedirectionsOwner is whose process the redirections of a command with
// no command word are made in: this shell's, except where the list carries a
// `{name}` redirection and the dialect makes such a list in a child of its
// own, which is the parentheses' owner — the same rows, measured, as `( : )`
// with that list on it. See Semantics.FdVariableMakesABareListASubshell
// (#6630).
//
// Asked only where the list has a `{name}` in it, which is the only shape the
// panel splits on: `>f`, `x=1 >f` and `4<&9-` with no command are this
// shell's in every column.
func (r *Runner) bareRedirectionsOwner(rs []*syntax.Redirect) redirOwner {
	for _, rd := range rs {
		if rd.N == nil {
			continue
		}
		if lit := rd.N.Literal(); len(lit) > 2 && lit[0] == '{' {
			if r.ask(r.sem().FdVariableMakesABareListASubshell,
				"a `{name}` redirection with no command word being made in a child of its own") {
				return redirOwnerASubshell
			}
			break
		}
	}
	return redirOwnerThisShell
}
