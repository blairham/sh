// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// commandWordWasWritten reports whether a command's own word was written out
// rather than produced by an expansion.
//
// One dialect reads it, and only for `command`: reached through a parameter
// or a substitution, that word loses the power to run anything and reports
// instead. See Semantics.ExpandedCommandOnlyReports, where the rows are.
//
// **Quoting does not take the writing away.** `"command" echo hi` and
// `\command echo hi` both run there, and `$c`, `${c}`, `"$c"` and
// `$(echo command)` all do not — so the question is whether a *substitution*
// stands in the word, not whether the text is bare. A test on the source
// spelling would have called the quoted forms expanded and been wrong in two
// rows out of seven.
//
// Asked of the first word alone. A later word cannot be the command's name,
// and an assignment prefix in front of it is not a word of this list at all.
func commandWordWasWritten(c *syntax.SimpleCmd) bool {
	if c == nil || len(c.Args) == 0 {
		// No word to have been written — a command that is only assignments
		// or only redirections reaches no builtin, so the answer is never
		// read. Written rather than expanded is the safe side of a value
		// nothing asks for: it is the reading every other dialect has.
		return true
	}
	for _, span := range c.Args[0].Spans {
		if span.Kind != syntax.Literal {
			return false
		}
	}
	return true
}
