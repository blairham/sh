// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The parameters that do not exist until their module is loaded.
//
// The **third** state a module parameter can be in, and the one this shell
// had no way to spell. Thirty-eight of them are registered at startup and
// wait for the script's first reference — that is the deferral, and
// deferredparameters.go is its roster. These seven are not registered at all:
// their modules declare no autoloadable parameter, so in the shell being
// modeled the name is simply not there until something loads the module.
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script file with `env -i
// PATH=/usr/bin:/bin TERM=dumb` and a scratch `HOME`, as the first statement
// of the script so that nothing has referred to any of them:
//
//	                        zsh 5.9.2                      before
//	${+mapfile}             0                              1
//	${(t)langinfo}          (empty)                        association-…
//	typeset -p sysparams    no such variable: sysparams    typeset -Ar sysparams
//	                        at 1                           at 0
//
// and a name nothing registers at all — `${+neverheardof}` — answers exactly
// the same three ways in both shells, which is what says the reference is
// describing a name it has not got rather than a name it is hiding.
//
// # Why this is a registration moved and not a deferral widened
//
// A deferred name is *registered and waiting*: `typeset -p funcstack` is
// silent at 0 there, which is neither `no such variable` nor a row. These
// seven answer `no such variable` at 1 — the answer an invented name gets —
// so deferring them would have made `typeset -p langinfo` silent at 0, which
// is neither shell's answer. #4917 left them out of the deferral roster
// deliberately and said so; this is that gap closed rather than that roster
// widened (#4922).
//
// # The two that are not in the issue's five, and why they are here
//
// `EPOCHSECONDS` and `EPOCHREALTIME` were to be *measured* rather than
// assumed to follow `epochtime`, and they do follow it: `typeset -p
// EPOCHSECONDS` is `no such variable: EPOCHSECONDS` at 1 in a fresh shell
// there and `integer-readonly-hide-hideval-special` after `zmodload
// zsh/datetime`, exactly as `epochtime` does. So the roster is per module
// after all for this module, which is a measurement and not the reason they
// are grouped.
//
// # What is deliberately **not** modeled
//
// That shell gates the module's **builtins** the same way — `strftime` is
// `command not found` until `zmodload zsh/datetime`, and `zsystem` until
// `zsh/system`. This shell registers every module builtin at startup and goes
// on doing so. The two are separable: a missing builtin is told by name at
// its own call site, which is the whole of zmodload.go's rule for why a
// module short of one still loads, and a parameter had no such line — which
// is why the parameter half is the half that costs a script something. The
// builtin half is a row of its own and is left where it is rather than
// changed in passing.
//
// An **unload** is the other row left alone: `zmodload -u zsh/datetime` takes
// the parameters away again there, where they stay here. That is the same
// question read backwards and it is not what this issue measured.
var zshGatedParameters = map[string][]string{
	"zsh/langinfo": {"langinfo"},
	"zsh/mapfile":  {"mapfile"},
	"zsh/system":   {"sysparams", "errnos"},
	"zsh/datetime": {"epochtime", "EPOCHSECONDS", "EPOCHREALTIME"},
}

// zshGatedParameterInstallers is the other half of the table above: what to
// run when the module arrives.
//
// Keyed by module rather than by name, because the registration is per module
// in the code even where the roster is per name in the measurement — one
// `zmodload zsh/system` brings `sysparams` and `errnos` together and there is
// no spelling that brings one without the other.
//
// A map of functions rather than a `switch`, so that the two tables are
// checked against each other: TestEveryGatedModuleHasAnInstaller says a
// module named above with nothing to run is a module whose parameters would
// never arrive, which is silent — `${+langinfo}` would simply stay 0 and the
// `zmodload` would still report success.
var zshGatedParameterInstallers = map[string]func(*interp.Runner){
	"zsh/langinfo": registerLangInfoParameter,
	"zsh/mapfile":  registerMapfileParameter,
	"zsh/system":   registerSystemParameters,
	"zsh/datetime": registerDatetimeParameters,
}

// zshGatedParameterModule is the module a name waits for, or the empty string
// for a name that is not gated.
//
// It is what keeps the gate from closing on itself. `zmodload` loads a module
// only when this shell has every feature it names — see zmodloadHasFeature —
// and a parameter that is not registered yet is exactly what that test reads
// as missing. So `zmodload zsh/langinfo` would have refused with `langinfo is
// not implemented yet`, which is the module rule firing at a name the module
// is about to bring. The feature is *there*; it has not arrived yet, and this
// says so.
func zshGatedParameterModule(name string) string {
	for module, names := range zshGatedParameters {
		for _, n := range names {
			if n == name {
				return module
			}
		}
	}
	return ""
}

// installGatedParameters brings a module's parameters into being, and is what
// a load does beyond saying it loaded.
//
// Idempotent, because a second `zmodload zsh/datetime` is not an error in
// either shell and re-running a registration must not be one either: every
// call below is a write to a table keyed by the name.
func installGatedParameters(r *interp.Runner, module string) {
	if install := zshGatedParameterInstallers[module]; install != nil {
		install(r)
	}
}
