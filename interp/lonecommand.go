// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// loneCommandNamed reports whether a forked body is nothing but one simple
// command whose command word is written name: unquoted literal text, with any
// assignments and redirections of its own, reached through any number of
// braces that carry no redirection.
//
// The shape BusyBox ash reads on the far side of a fork to decide whether the
// child keeps a piece of the parent's state it otherwise clears — see
// Semantics.ALoneTrapCommandKeepsTrapListing for `trap` and
// SubshellJobsKeptForALoneJobsCommand for `jobs`. It is the *written* word and
// not the command that runs: measured 2026-10-03 in the pinned image,
// `trap | cat`, `x=1 trap | cat`, `trap 2>/dev/null | cat`, `{ trap; } | cat`
// and `echo "$(trap)"` list the parent's traps, while `$t | cat` with
// t=trap, `"trap" | cat`, `\trap | cat`, `command trap | cat`, `f | cat`
// with f calling trap, `{ echo a; trap; } | cat`, `{ trap; } 2>&1 | cat`
// and `$(trap; echo z)` list nothing.
func loneCommandNamed(stmts []*syntax.Stmt, name string) bool {
	if len(stmts) != 1 {
		return false
	}
	st := stmts[0]
	if st.Background {
		return false
	}
	p, ok := st.Expr.(*syntax.Pipeline)
	if !ok || p.Negated || len(p.Cmds) != 1 {
		return false
	}
	return loneCommandIn(p.Cmds[0], name)
}

// loneCommandIn is loneCommandNamed for one command: a pipeline element.
func loneCommandIn(cmd syntax.Command, name string) bool {
	switch c := cmd.(type) {
	case *syntax.Group:
		return len(c.Redirs) == 0 && loneCommandNamed(c.List, name)
	case *syntax.SimpleCmd:
		if len(c.Args) == 0 || len(c.Precommands) != 0 {
			return false
		}
		w := c.Args[0]
		return plainLiteralWord(w) && w.Literal() == name
	}
	return false
}
