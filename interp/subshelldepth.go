// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// SubshellDepth is how many subshell boundaries lie between this runner and
// the shell that was started. Zero in the shell itself.
//
// A **count** where [Runner.InSubshell] is a flag, and the two are not the
// same question: the flag is already true one boundary in, so it cannot tell
// a subshell of a subshell from a subshell. A dialect with a parameter for
// the depth needs the difference — measured 2026-09-27 on zsh 5.9.2,
// `-f` from a script file:
//
//	top level                          0
//	( … )                              1
//	( ( … ) )                          2
//	$( … ) and the backquoted form     1
//	<( … )                             1
//	( … ) &                            1
//	a function body                    0
//	{ … }                              0
//	eval                               0
//
// The last three are why the count is kept on the clone rather than on any
// construct's own entry: a function call, a brace group and an `eval` all run
// in this shell, and only what copies the runner is a boundary.
//
// **The pipeline is the case worth measuring rather than assuming**, and it
// is where the shell being modeled parts from an engine that copies every
// stage. Measured the same day, same route: a stage that is not the last
// answers 1, and the **last stage answers 0** — `true | print $ZSH_SUBSHELL`
// is 0 and `print $ZSH_SUBSHELL | cat` is 1, in the reference. An engine that
// clones every stage counts the last one as a boundary, so a dialect naming
// this must say which of the two it wants; see the parameter's own file.
func (r *Runner) SubshellDepth() int { return r.subshellDepth }

// theForkIsTheParentheses reports whether the statement a fork was made to
// run is a bare `( … )`, so those parentheses **are** that fork rather than a
// boundary inside it.
//
// The shell being modeled forks once for `( … ) &` and once for
// `( … ) | cat`, and the count says so. Measured 2026-09-27 on zsh 5.9.2,
// `-f` from a script file, the depth read inside the innermost body:
//
//	( … ) &                    1     the parentheses are the fork
//	( true; … ) &              1     and still are, with commands before it
//	{ ( … ) } &                2     the braces are the body; the `(` forks
//	f() { ( … ); }; f &        2     so does a function body
//	( … ) | cat                1     a stage is a fork and the `(` is it
//	{ ( … ) } | cat            2
//	$( ( … ) )                 2     and a substitution does **not** collapse
//
// The last row is why this is asked at two sites rather than in clone():
// three constructs fork and only two of them let the parentheses stand for
// the fork, so a rule in the one place every copy is made would have to be
// right about all three and is measurably not. It is the syntactic shape and
// not "nothing has run yet" — row two has commands before the read and is
// still one — which is also what keeps row three at two, the braces being a
// different command from the parentheses inside them.
//
// This engine's forks are cloned Runners rather than processes, so without it
// the three collapsing rows each counted twice: the same reconstruction of a
// process boundary Runner.forkedForABackgroundJob is, arrived at from the
// count's side.
func theForkIsTheParentheses(st *syntax.Stmt) bool {
	// One pipeline of one command, with no `!` in front of it: anything else
	// — an and-or chain, a real pipeline, a `time` clause — is a body the
	// fork runs rather than a command the fork *is*.
	p, single := st.Expr.(*syntax.Pipeline)
	if !single || p.Negated || len(p.Cmds) != 1 {
		return false
	}
	return commandIsParenthesized(p.Cmds[0])
}

// commandIsParenthesized is the same question asked of a pipeline element,
// which already has its command in hand.
func commandIsParenthesized(cmd syntax.Command) bool {
	_, parenthesized := cmd.(*syntax.Subshell)
	return parenthesized
}
