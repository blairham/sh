// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// rosterKeepsThisPrefix is Semantics.BuiltinsKeepingAnAssignmentPrefix asked
// about *this* call rather than about the name alone.
//
// The roster is a list of names one shell keeps a prefix in front of while
// answering No to the specialness question — `alias`, `hash`, `builtin` and
// `exec` there. For three of them the name is the whole of it. For the
// fourth it is not, and the difference is the redirection form.
//
// Measured 2026-09-29 on zsh 5.9.2, `( x=43; x=v CMD; print "x=$x" > result )`
// with the value read back outside the subshell so that no probe writes
// through a descriptor the command may have moved:
//
//	x=v exec              x=v    the roster keeps it
//	x=v exec >out         x=43   and here it does not
//	x=v exec 2>out        x=43   any redirection, not just the first
//	x=v exec 3>out        x=43
//	x=v exec <in          x=43   reading as well as writing
//	x=v builtin           x=v    the contrast: a redirection changes
//	x=v builtin 2>out     x=v    nothing for a name that is not the form
//	x=v builtin :         x=43   what moves `builtin` is a command word
//	x=v alias foo=bar     x=v    and operands change nothing at all here
//	x=v hash 2>out        x=v
//
// **It is keyed on the redirection form and not on the word.** `exec` is the
// one member of the modifier family whose no-command form is defined — see
// PrecommandRedirectionForm — and `exec >out` is that builtin doing its
// other job: the redirection outlives the command, so the command is not the
// do-nothing case the roster is about. `builtin >out` is a redirection with
// no command in the same shell, and the rows above are why this asks the
// modifier table rather than comparing against a name.
//
// The three rows that hold the name fixed and move only the operands —
// `exec` against `exec >out`, and `builtin` against `builtin 2>out` — are
// what say the redirection is the key and not "a roster entry with anything
// written after it".
func (r *Runner) rosterKeepsThisPrefix(argv []string, redirs []*syntax.Redirect) bool {
	name := r.prefixRosterName(argv)
	if !r.builtinKeepsAnAssignmentPrefix(name) {
		return false
	}
	if len(redirs) == 0 {
		return true
	}
	return r.precommands[name] != PrecommandRedirectionForm
}
