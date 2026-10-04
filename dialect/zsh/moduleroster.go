// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$modules` is this shell's own module roster: every module it knows about,
// to the word for what has happened to it.
//
// A view over the set `zmodload` already keeps, which is the route out of the
// absent roster that `$reswords`, `$history` and the three job tables took
// before it: the fact was there and the publication was missing. See
// zmodloadLoaded, which is the same set `zmodload` with no operands writes
// and is what keeps the parameter and the builtin from coming to disagree.
//
// # The word, and the one this shell cannot say
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script file, with
// `zsh/parameter` and then `zsh/datetime` loaded:
//
//	zsh/main        loaded
//	zsh/parameter   loaded
//	zsh/datetime    loaded
//	zsh/zle         autoloaded
//	zsh/zutil       autoloaded      … and eleven more
//
// and `zmodload -u zsh/datetime` takes its row out again rather than moving
// it to the other word. So the table has two populations: what a script has
// loaded, and what that *installation* registered for autoloading with
// `zmodload -a` before the shell ever ran.
//
// **Only the first is written here**, and the second is left out for the
// reason builtInTies leaves `FPATH`'s default out: those fourteen names are
// one machine's `/etc/zshrc` and one zsh build's module directory, and
// writing them down would hand a script another installation's roster as
// though this shell had it. What a *script* registers with `zmodload -a` is
// a different matter — it is the script's own — and those modules are
// written as `autoloaded` from the same set the builtin keeps (see
// zmodloadautoload.go) rather than from a list in this file.
//
// So a fresh shell answers one row, `zsh/main loaded`, which is what
// zmodloadAlwaysLoaded already says about itself through every other surface
// the builtin has.
func zshModulesView(r *interp.Runner) interp.AssocArray {
	loaded := zmodloadLoaded(r)
	out := make(interp.AssocArray, len(loaded))
	// A module a registration names and nobody has loaded is the second
	// word. Measured 2026-10-03 on zsh 5.9.2 under `-f`: after `zmodload -a
	// zsh/nosuch mb`, `$modules[zsh/nosuch]` is `autoloaded`. Written first
	// so that a module both loaded and named is `loaded`.
	for _, m := range zmodloadAutoloadedModules(r) {
		out[m] = interp.Scalar(zshModuleAutoloadedWord)
	}
	for _, m := range loaded {
		out[m] = interp.Scalar(zshModuleLoadedWord)
	}
	return out
}

// zshModuleLoadedWord is the state word a module a script has loaded carries.
const zshModuleLoadedWord = "loaded"

// zshModuleAutoloadedWord is the word a module carries when a `zmodload -a`
// registration names it and it is not loaded. Only a script's registrations
// reach it — see zshModulesView for the installation's own, which do not.
const zshModuleAutoloadedWord = "autoloaded"

// registerModuleRoster installs `$modules`.
//
// Readonly and hidden, the pair every produced table here carries: measured,
// `${(t)modules}` is `association-readonly-hide-hideval-special`, and
// `modules=(a b c)` is `read-only variable: modules` at 1 — which is the
// freeze registerAbsentParameters was already putting on the name and which
// has to survive the name becoming a view (#1604).
func registerModuleRoster(r *interp.Runner) {
	r.SetDynamicAssoc("modules", zshModulesView)
	r.MarkReadonly("modules")
	// Silent to `-p`, as zsh/parameter's frozen tables are; see
	// interp.Runner.SetSilentToPrint.
	r.SetSilentToPrint("modules")
	hideModuleParameter(r, "modules")
}
