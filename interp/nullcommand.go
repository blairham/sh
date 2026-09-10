// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// nullCommand is the command word a command that is only redirections runs,
// in a dialect that has the hook: see Semantics.NullCommandVariable.
//
// Three answers rather than two. `hooked` false is a dialect without the hook
// at all, where such a command opens its files, runs nothing and succeeds —
// the core's behavior and five of the six panel columns'. `hooked` true with
// an empty name is the hook's own refusal, which is a different shell from
// the first: the command is reported and reports 1.
//
// Measured 2026-09-10 on zsh 5.9.2 with two marker functions, one behind each
// parameter, because that is the only probe that separates the routes — left
// at their defaults both of them print the file and the reading cannot be
// falsified.
func (r *Runner) nullCommand(c *syntax.SimpleCmd) (name string, hooked bool) {
	if r.sem().NullCommandVariable == "" || len(c.Redirs) == 0 {
		return "", false
	}
	// An assignment prefix takes the command off this route: `x=1 <f` sets
	// `x`, opens the file and runs nothing, with the hook pointed at a marker
	// that never prints. The same clause disqualifies the `$(<file)` form,
	// and for the same reason — a command with a prefix is a command.
	if len(c.Assigns) != 0 {
		return "", false
	}
	if v := r.sem().ReadNullCommandVariable; v != "" && readNullCommand(c) {
		if name := firstValue(r.getVar(v)); name != "" {
			return name, true
		}
		// Emptied or unset, which falls back rather than refusing: a script
		// that clears the reader gets the writer's command instead.
	}
	return firstValue(r.getVar(r.sem().NullCommandVariable)), true
}

// readNullCommand reports whether the command's redirections are the single
// plain input file redirection that takes the reading parameter's route.
//
// One redirection and one operator. The descriptor number is *not* part of it
// — `3<f` reads the marker as surely as `<f` does — which is where this test
// parts company with the one the `$(<file)` form applies to a body that looks
// identical: that form is standard input alone. Everything else measured onto
// the writing parameter's route: a second redirection, `<>`, `<<`, `<<<`,
// `<&`, and any output redirection beside the input one.
func readNullCommand(c *syntax.SimpleCmd) bool {
	if len(c.Redirs) != 1 {
		return false
	}
	rd := c.Redirs[0]
	return rd.Op == syntax.TokLess && rd.Heredoc == nil
}

// firstValue drops the "was it set" half of a variable read: unset and set to
// nothing are the same answer to both of these parameters, which is measured
// — `unset NULLCMD` and `NULLCMD=` both refuse the command in the same words.
func firstValue(v string, _ bool) string { return v }
