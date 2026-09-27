// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The parameters this shell brings into being on a script's first reference,
// and the ones it does not.
//
// The mechanism is [interp.Runner.SetDeferredParameter] and the grid that
// measures what counts as a reference is in interp/deferredparam.go. This
// file is the **roster**: which names the reference defers, measured name by
// name rather than derived from the module each belongs to, because the two
// do not line up and reasoning from the module would have got five names
// wrong.
//
// # How the roster was measured, and why the route matters
//
// A read is the thing under test, so a probe that reads the name to find out
// whether it is there has already answered its own question. The route used
// is `typeset -p name` as the **first statement of a script**, which is no
// reference — proved by asking twice in one shell and getting nothing both
// times — and which tells the three states apart in one line: a row means the
// parameter is there, `no such variable` at 1 means it is not, and nothing at
// 0 means it is registered and waiting.
//
// Measured 2026-09-27 on zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, one shell per name.
// The thirty-six below are the `nothing at 0` answers. Five names this shell
// also registers are the *second* state there and are deliberately not in the
// list — `langinfo`, `mapfile`, `sysparams`, `errnos` and `epochtime` are
// `no such variable` at 1 in a bare reference, because their modules declare
// no autoloadable parameter, so what they want is not a deferral but a
// registration that waits for `zmodload`. That is a different gap and it is
// left where it is rather than papered over with this one.
//
// # It is not the same list as `hideModuleParameter`
//
// Every name here is hidden and most hidden names are here, and they are
// still two statements. `hideModuleParameter` says what a listing writes for
// a name it *has*; this says whether it has it yet. Folding the two would
// have put the five above into the deferred set on the strength of their
// sharing an attribute with the rest, which is the shape of reasoning the
// measurement above exists to replace.
var zshDeferredParameters = []string{
	// `zsh/parameter`'s tables, produced rather than stored.
	"aliases",
	"builtins",
	"commands",
	"dis_aliases",
	"dis_builtins",
	"dis_functions",
	"dis_functions_source",
	"dis_galiases",
	"dis_patchars",
	"dis_reswords",
	"dis_saliases",
	"funcfiletrace",
	"funcsourcetrace",
	"funcstack",
	"functions",
	"functions_source",
	"functrace",
	"galiases",
	"history",
	"historywords",
	"jobdirs",
	"jobstates",
	"jobtexts",
	"modules",
	"nameddirs",
	"options",
	"parameters",
	"patchars",
	"reswords",
	"saliases",
	"userdirs",
	"usergroups",
	// `zsh/zleparameter`'s two, `zsh/sched`'s one and the two terminal
	// capability tables — all of them `nothing at 0` on the same route, and
	// none of them reached by `zmodload zsh/parameter`, which is what says
	// the state is per parameter rather than per module.
	"keymaps",
	"widgets",
	"zsh_scheduled_events",
	"termcap",
	"terminfo",
	// And the directory stack, which is not a module parameter at all and
	// behaves identically: `typeset -p dirstack` is nothing at 0 in a fresh
	// shell, still nothing after a `cd` that fills the stack, and a row once
	// `${#dirstack}` has been read. It is the row that says the mechanism is
	// the shell's own laziness rather than module loading (#4899).
	"dirstack",
}

// registerDeferredParameters tells the core which of this dialect's
// registered parameters are waiting for a first reference.
//
// Called after every module registration rather than beside each one, so the
// roster reads as the measurement it is: a name is in the list because the
// reference was asked about that name, not because of which `register…`
// function happened to install it.
func registerDeferredParameters(r *interp.Runner) {
	for _, name := range zshDeferredParameters {
		r.SetDeferredParameter(name)
	}
}
