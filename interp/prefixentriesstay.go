// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// What a command's assignment prefix leaves behind when something in the
// command fails and the failure ends the shell: the entries it had already
// assigned, or what the shell held before them. See
// Semantics.PrefixEntriesStayWhenAFailureEndsTheShell for the panel (#7004).
//
// The entries are put back by whichever walk or route holds them — the walk
// ahead of the redirections, the walk ahead of a declaration's operand, the
// trace walks, and the builtin's, the function's and the program's own
// stores — and each of them asks this on its way out, rather than the shell
// re-applying anything at its end. A failure that does not end the shell is
// no reason to leave anything, so it is the prefix's ordinary end there.

// prefixEntriesStay reports whether the entries a command's prefix has
// assigned so far stay where they are, rather than being put back, because a
// failure in the command is ending the shell.
//
// kind is the command, which decides whether the failure is contained in it
// rather than ending anything — see Runner.prefixFailureIsThisCommandsAlone.
// Asked only once something has failed, so an ordinary prefix reads nothing.
func (r *Runner) prefixEntriesStay(kind prefixCommand) bool {
	if !r.expandErr && !r.givingUpAlready() {
		return false
	}
	if r.Interactive {
		// A shell that carries on past the failure gives the entries back:
		// measured 2026-10-30, `v=old`, then `v=new w=${q?boom} :`, then
		// `echo $v` at an interactive zsh 5.9.2 is `v=old`.
		return false
	}
	if r.prefixFailureIsThisCommandsAlone(kind) {
		return false
	}
	// Read rather than asked: this is a path that has already written one
	// complaint, and an unanswered vector keeps the answer this shell gave
	// before the question was put — see Runner.failedExpansion for the same
	// reasoning.
	return r.sem().PrefixEntriesStayWhenAFailureEndsTheShell == Yes
}

// leaveThePrefixEntries puts a program's prefix entries back under their names
// as the walk expanded them, for the route that had already released its hold
// on them when the refusal that ends the shell was made. Each is `name=value`,
// as the child was to be handed it.
func (r *Runner) leaveThePrefixEntries(entries []string) {
	for _, e := range entries {
		name, value, _ := strings.Cut(e, "=")
		r.arraysDel(name)
		r.dropAssocTable(name)
		r.attrClear(name, attrRemoved)
		r.varsPut(name, value)
	}
}
