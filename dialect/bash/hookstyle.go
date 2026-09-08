// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/repl"

// HookStyle is what this shell runs between one command and the next.
//
// One name, and it is a *variable* rather than a function: `PROMPT_COMMAND`
// holds command text, which the session evaluates before every prompt. zsh's
// `precmd` holds the name of a function to call, so the two fire at the same
// moment and are not the same mechanism — see repl.HookStyle, which carries
// both fields for that reason. dash and ksh93 have neither.
//
// Measured through a paced pseudo-terminal on 2026-09-07, macOS 25.5, in a
// scratch `HOME` with `HISTFILE` redirected, `/opt/homebrew/bin/bash` 5.3.15
// and `/bin/bash` 3.2.57 alike, and `/opt/homebrew/bin/bash` a third time
// under an argv[0] of `sh`:
//
//   - It runs **before the first prompt**, with nothing typed yet. A prompt
//     built from a command substitution proved the order: the hook's output
//     came out above the prompt's own, every time, so it fires before the
//     prompt is expanded and cannot land in the middle of one.
//   - It runs after every accepted line, including an **empty** one and
//     including a line the parser **refused** — and not at a `PS2`
//     continuation, where a four-line `for` loop ran it once at the end.
//   - The **job notices come first**, exactly as they do for zsh's hook.
//   - It is told the status of the line before it and **cannot change what the
//     next command reads**: after `(exit 5)` the hook saw 5, and the next line
//     typed still read `$?` as 5 — through a hook that failed with `command
//     not found` and through an array whose second element ran `false`.
//   - `exit` inside it ends the session there. `PROMPT_COMMAND='echo A; exit
//     3; echo NOTREACHED'` printed `A` and the shell was gone with status 3.
//   - Unset and empty are silent and run nothing. Text that will not parse is
//     reported **against the variable's name** — `bash: PROMPT_COMMAND: line
//     3: syntax error near unexpected token ...` — the variable is left set,
//     the session carries on, and the same complaint arrives at every prompt
//     after. See interp.Runner.EvalVariable, which is where the naming lives.
//   - The chain is the *value* as the prompt found it: an element that
//     assigned a whole new array to the name did not change what the rest of
//     that prompt's chain ran, and the new value took effect at the next one.
//
// The one place the two builds disagree is the **array**: 5.3.15 with
// `PROMPT_COMMAND=('echo A' 'echo B; false' 'echo C=$?')` ran all three in
// order, and 3.2.57 ran `echo A` alone — it reads the name as a scalar, which
// is element 0. This tree carries one bash and it is 5.3's, so the array form
// is what runs here; the 3.2 reading is recorded and not offered.
//
// Nothing is Unfired. Every hook this shell has is this one, and it runs.
func HookStyle() repl.HookStyle {
	return repl.HookStyle{BeforePromptVariable: "PROMPT_COMMAND"}
}
