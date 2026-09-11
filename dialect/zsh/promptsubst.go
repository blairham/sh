// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `setopt PROMPT_SUBST`: whether a prompt's `${…}`, `$(…)` and `$((…))` are
// expanded when it is drawn.
//
// Off in a fresh shell, which is this dialect alone — the other three in the
// panel expand a prompt always and have no name for the question. Measured
// 2026-09-10 through a pseudo-terminal with `PROMPT='[hello-${V}-$((1+2))] '`
// and `V=WORLD`: with the option set the prompt draws `[hello-WORLD-3]`, and
// without it the fourteen characters as written.
//
// # It is asked at every draw, and that is the point
//
// The state is the one `setopt`, `unsetopt` and `$options` already share, so
// a script that turns it on mid-session is obeyed at the next prompt rather
// than at the next login. That is not a nicety: a prompt theme sets the
// option in its own setup, long after this shell has read its startup files,
// and a shell that decided the answer at startup would answer no for the
// rest of the session.
//
// What that cost is worth recording, because nothing reported it. A theme's
// whole `PROMPT` is parameter expansions — `${_p9k__raw_msg-}${(e)_p9k_t[7]}…`
// — which mean nothing unexpanded, so the session drew that text, verbatim,
// as its prompt. No diagnostic: the option was set, remembered and reported
// correctly by every listing, and simply consulted by nothing.
func promptSubstIsOn(r *interp.Runner) bool {
	if r == nil {
		// No shell, so nothing has set the option. A caller with no runner
		// is asking what a fresh one would answer.
		return false
	}
	i, ok := zshOptionIndex["promptsubst"]
	if !ok {
		return false
	}
	return zshOptions[i].get(r)
}
