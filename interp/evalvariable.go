// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// EvalVariable runs command text a front end read out of a shell variable, as
// `eval` runs its arguments, and names that variable in any diagnostic the
// text produces.
//
// The call a *front end* needs and a script never does, which is what
// [Runner.CallFunction] is beside it — and the two are the two halves of the
// same seam rather than one of them twice. A hook whose value is a function
// *name* is called; a hook whose value is command *text* is evaluated, and
// there is no way to reach the second through the first: building `eval
// "$VAR"` and parsing it would run the text through the grammar a second time,
// so a value holding a lone `$` or an unbalanced quote would be a syntax error
// in the wrapper rather than in the text, reported against a line nobody wrote.
//
// **The variable's name is what a diagnostic says, not `eval`.** Measured on
// 2026-09-07, bash 5.3.15 and 3.2.57 through a pseudo-terminal, with
// `PROMPT_COMMAND='echo unbalanced ((('`:
//
//	bash: PROMPT_COMMAND: line 3: syntax error near unexpected token `('
//	bash: PROMPT_COMMAND: line 3: `echo unbalanced ((('
//
// which is where the name has to come from: the text is in a variable and the
// person's only way back to it is that variable's name. The same failure said
// `eval` would send them looking for a builtin they never ran. Both shells
// left the variable set and reported it again at every prompt after, and the
// session carried on — so this reports and returns, and refuses nothing.
//
// The status is the text's, exactly as `eval`'s is, and every caller of this
// is at a site that must not let it reach the next command — see
// [Runner.FireChain], which saves `$?` and puts it back around each item of a
// chain.
func (r *Runner) EvalVariable(ctx context.Context, name, text string) int {
	// The context the text runs under, for the length of the run, for the
	// reason CallFunction sets one: a hook fires between two chunks rather
	// than inside one, so there is no RunPart in progress to have set this.
	saved := r.ctx
	r.ctx = ctx
	defer func() { r.ctx = saved }()
	return r.runSourced(ctx, text, sourced{
		eval:         true,
		named:        name,
		label:        name,
		syntaxStatus: r.diag().SyntaxStatus(),
	})
}
