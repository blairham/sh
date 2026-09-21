// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The substrate's own prelude, which today is one function: `prompt`.
//
// # Why there is a second prelude at all
//
// `Shell.Prelude` is the **dialect** written as shell, and each dialect
// package owns its own. The prompt theme engine is not a dialect's — it is a
// capability of the substrate, the whole point of #1323 being that a script
// run under `dash` gets the same prompt as one run under `zsh`. Putting the
// function in each dialect's prelude would be five copies of one text, which
// is the "fifth copy" failure AGENTS.md records about the front end itself.
// So it is one text, here, sourced in the same window as the dialect's.
//
// # Why it is only sourced for an interactive session
//
// A prelude function is *presented as a builtin* — that is #1117's one
// table, and it is what makes `prompt` visible to `type` and `command -v`
// exactly as `pushd` is. That divergence is accepted (see
// interp/promptengine.go) and it is still worth keeping as small as it can
// honestly be: a theme exists only where there is a prompt, so a script run
// by `./dash script.sh` reaches no `prompt` and answers about the name
// exactly what it answered before this file existed.
//
// # Written in the core language
//
// It is parsed with the session's own `syntax.Dialect`, so it must be a
// program every dialect reads. There is deliberately almost nothing in it:
// the subcommands and the usage line are matched in Go, because a `case`
// here and a `switch` there would be two lists to keep in step and the drift
// would be a word the function accepts and the engine does not. What the
// function buys is the *name* — a word a person types, presented the way
// `pushd` is — and the location, since a function the prelude defined is the
// shell speaking and its complaints are located where the person typed them.
const promptPrelude = `
prompt() {
	` + interp.PromptEngineName + ` "$@"
}
`

// sourcePromptPrelude installs the substrate's prelude on a runner that has a
// prompt engine behind it.
//
// Nothing is defined where there is no engine. A `prompt` function that
// answered `promptengine: command not found` would be worse than no function
// at all: it would claim a name this shell does not really have, and the
// lookup answers that word only while a prelude function is running, so the
// failure would be visible nowhere else.
func (sh Shell) sourcePromptPrelude(r *interp.Runner, name string) int {
	if !r.HasPromptEngine() {
		return 0
	}
	r.SourcingPrelude(true)
	defer r.SourcingPrelude(false)
	f, err := syntax.Parse(promptPrelude, sh.Dialect)
	if err == nil {
		err = r.RunPart(context.Background(), f)
		// The same reason Shell.source forgets it: the prelude is the
		// shell's plumbing and not a command the person ran, so what it
		// leaves in `$_` is not an answer about anything they typed.
		r.ForgetLastArgument()
	}
	if err != nil {
		sh.errf("%s: prompt prelude: %v\n", name, err)
		return usageStatus
	}
	return 0
}
