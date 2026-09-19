// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// ConditionAnswer answers one conditional operator that this package's
// grammar has and whose meaning belongs to a dialect.
//
// operands are the operand words with their quoting already resolved into the
// matcher's language — the same string [Runner.MatchPattern] takes — because
// a completion condition's operand is a *pattern*: measured on zsh 5.9.2
// inside a `zle -C` widget with `PREFIX=foobar`, `[[ -prefix *o ]]` is 0 and
// `[[ -prefix '*o' ]]` is 1, so the quoting decides whether the word is a
// pattern or a literal exactly as it does for `==`.
//
// The second result is whether this operator could be answered *here at all*.
// False is not "the condition is false": it is the refusal
// [Diagnostics.CompletionConditionOutsideCompletion] carries, which the
// caller writes and which ends the shell. A completion condition reached
// outside a completion is that case, and it is the only one.
type ConditionAnswer func(r *Runner, ctx context.Context, op string, operands []string) (bool, bool)

// SetConditionAnswer installs the answer to one conditional operator, named
// the way it is written inside `[[ ]]` — `-prefix`, with its leading dash.
//
// The seam the completion-context conditions need, and the shape
// [Runner.Register] already has for a builtin: the grammar is the parser's
// and the meaning is the dialect's, so the operator parses wherever
// [syntax.Dialect.CompletionConditions] is on and answers wherever a dialect
// has said what it means. Nothing registered is the refusal, which is what a
// shell with the grammar and no completion system gives.
func (r *Runner) SetConditionAnswer(op string, ask ConditionAnswer) {
	if r.conditionAnswers == nil {
		r.conditionAnswers = map[string]ConditionAnswer{}
	}
	r.conditionAnswers[op] = ask
}

// KnownCondition reports whether this shell can answer the conditional
// operator named — again with its leading dash.
//
// For `zmodload`, whose feature list names a module's conditions as `c:prefix`
// and which has to say whether this shell has one. A condition has no word
// that runs it, so a script is never told about a missing one the way it is
// told about a missing builtin, and that is why the question has to be asked
// at load time at all (#3042).
func (r *Runner) KnownCondition(op string) bool {
	_, ok := r.conditionAnswers[op]
	return ok
}
