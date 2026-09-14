// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// hideModuleParameter gives one of this shell's *module* parameters both
// hiding attributes, because measurement says a module parameter never
// carries one without the other.
//
// The two are genuinely two — a parameter given only `typeset -H` describes
// as `hideval` and not as `hide` (#2042), which is why
// [interp.Runner.MarkHidden] and [interp.Runner.MarkHideInScope] are separate
// seams and must stay separate. What is measured here is narrower: that for a
// parameter a *module* provides, the pair is always a pair. Every module
// parameter this dialect registers was read back from zsh 5.9.2 on
// 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`, `ZDOTDIR`
// and `HISTFILE`, over a script file with the modules loaded:
//
//	${(t)builtins}       association-readonly-hide-hideval-special
//	${(t)langinfo}       association-hide-hideval-special
//	${(t)epochtime}      array-readonly-hide-hideval-special
//	${(t)EPOCHSECONDS}   integer-readonly-hide-hideval-special
//
// and all thirty of them carry `hide-hideval`, readonly or not. The shell's
// own specials — `RANDOM`, `SECONDS`, `ARGC`, `LINENO`, `COLUMNS`, `PROMPT`,
// `argv` — carry neither, so this is the module half of the question and not
// "every special parameter".
//
// Calling only `MarkHidden`, which is what every site here did, described
// eighteen module parameters with `hideval` and without `hide` — telling a
// script switching on the word about the wrong letter, which is the failure
// #2042 was filed for, reappearing at the registration sites after being
// fixed in the description.
//
// **Both halves of the letter are here.** `hide` is a word in `${(t)…}` and
// it is also a behavior: a local declaration of the name is an ordinary
// parameter rather than a second view of the module's table. #2552 fixed the
// word and left the behavior, and #2586 wired the behavior — the producer is
// suspended for as long as a hidden shadow stands over the name, so
// `f() { local parameters; echo "$parameters"; }` prints an empty line and
// `local EPOCHSECONDS=5` reads 5, as zsh 5.9.2 does. See
// [interp.Runner.MarkHideInScope] and interp/hideinscope.go, where the rows
// are.
func hideModuleParameter(r *interp.Runner, name string) {
	r.MarkHidden(name)
	r.MarkHideInScope(name)
}
