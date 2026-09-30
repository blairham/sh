// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

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
// An **unload** is the same question read backwards, and it is answered now:
// `zmodload -u zsh/datetime` takes the parameters away again, which is
// releaseGatedParameters below (#5025).
var zshGatedParameters = map[string][]string{
	"zsh/langinfo": {"langinfo"},
	"zsh/mapfile":  {"mapfile"},
	"zsh/system":   {"sysparams", "errnos"},
	"zsh/datetime": {"epochtime", "EPOCHSECONDS", "EPOCHREALTIME"},
	// And the module with a **second** way in, which is why it is here and
	// not only in watchpair.go: a reference to either half of
	// `$WATCH`/`$watch` loads `zsh/watch`, and the module brings these two
	// with it. See registerWatchModuleParameters (#4998).
	"zsh/watch": {"WATCHFMT", "LOGCHECK"},
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
	"zsh/watch":    registerWatchModuleParameters,
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

// installGatedParameters brings into being the module's parameters **that the
// selection holds**, and is what a load does beyond saying it loaded.
//
// Idempotent, because a second `zmodload zsh/datetime` is not an error in
// either shell and re-running a registration must not be one either: every
// call below is a write to a table keyed by the name.
//
// # The installer is keyed on the module and the selection is per feature
//
// That mismatch is the whole of #5043. `registerSystemParameters` brings
// `sysparams` and `errnos` together and `registerDatetimeParameters` brings
// all three of its own — there is no spelling that brings one without the
// other — so a narrowed `-F` installed the module's whole roster. Measured
// 2026-09-28 on `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable*
// for it — `-f` from a script file under `env -i PATH=/usr/bin:/bin
// TERM=dumb` with a scratch `HOME`:
//
//	                                        zsh 5.9.2   before
//	-F zsh/system p:sysparams
//	  ${+sysparams} ${+errnos}              1 0         1 1
//	-F zsh/datetime b:strftime
//	  ${+EPOCHSECONDS} ${+epochtime} …      0 0 0       1 1 1
//
// The second row is the sharper one: the selection names a **builtin** and no
// parameter at all, and all three of the module's parameters arrived.
//
// So the roster is made to agree with the selection here, right after the
// installer runs. zmodloadEnforce says the same sentence about the same state
// and cannot say it for these names, because it runs **before** the install
// on both load routes — and a withdrawal recorded in front of a registration
// captures nothing, which is what left the name reading as *taken* and made
// `zmodload -F zsh/datetime b:strftime; zmodload zsh/datetime` refuse the
// widening with `Can't add module parameter`. One root, three rows.
//
// **Nothing here puts back a withdrawal an unload made**, deliberately, and
// it is the same economy gatedbuiltins.go records for the builtin half:
// zmodloadEnforce is the other writer of the withdrawn state and a plain
// `zmodload` widens the selection to every feature the module names, so a
// load after an unload comes back by the road `-F +p:name` already took. What
// is written below is narrower than that and is not a second writer of it: it
// is the same answer for the names that road cannot reach, asked at the only
// moment they exist.
func installGatedParameters(r *interp.Runner, module string) (dropped []string) {
	install := zshGatedParameterInstallers[module]
	if install == nil {
		return nil
	}
	// Which of this module's names the **script** already owns, read before
	// the installer runs because the installer is what would hide them.
	//
	// A module registers its whole roster in one call, so a name the script
	// holds is registered over and then let go again — see
	// interp.Runner.DropProducedParameter for why that is the shape and for
	// the measurement. The alternative, skipping the installer altogether,
	// takes the module's *other* names down with it: measured, a `local
	// EPOCHSECONDS` and then `zmodload zsh/datetime` still produces
	// `epochtime` and `EPOCHREALTIME` in the reference, and skipping left
	// both unset.
	var taken []string
	for _, name := range gatedParametersTheModuleOwns(module) {
		if a, held := r.ParameterAttributes(name); held && !a.Provided {
			taken = append(taken, name)
		}
	}
	install(r)
	on := make(map[string]bool)
	for _, feature := range zmodloadEnabled(r, module) {
		on[feature] = true
	}
	for _, name := range gatedParametersTheModuleOwns(module) {
		// **Cleared before it is written**, and the two lines are not one
		// line twice. A withdrawal zmodloadEnforce recorded ran *in front of*
		// the registration above, so what it captured was an empty set of
		// producers — and the installer then wrote real ones straight over a
		// name the record still calls withdrawn. Writing `true` again would
		// find the record already there and return, leaving the producers
		// standing, which is the whole of why setting the state here did
		// nothing until the clear was in front of it.
		//
		// Clearing is safe over a live registration:
		// interp.Runner.SetParameterWithdrawn puts back only what it took,
		// and it took nothing.
		r.SetParameterWithdrawn(name, false)
		r.SetParameterWithdrawn(name, !on["p:"+name])
	}
	// **After the bookkeeping above, not before it.** Clearing a withdrawal
	// puts back what it took, so a drop in front of that line was undone by
	// it — measured, the name came back produced one statement later and the
	// row read exactly as it had before the fix.
	for _, name := range taken {
		// The load does not take this name, now or later: once the function
		// holding the local returns, the reference has the name unset rather
		// than produced.
		r.DropProducedParameter(name)
	}
	// Reported so the caller does not refer to a name this just let go: a
	// reference materializes it again, and the load would end up owning the
	// name after all — one statement later than before. Measured, the
	// reference has the name **unset** once the function holding the local
	// returns.
	return taken
}

// gatedParametersTheModuleOwns is the names an unload takes away and a load
// brings back: the module's gated parameters that the module also **declares**
// as features of its own.
//
// **Both halves are needed and `zsh/watch` is what says so.** Measured
// 2026-09-28 on `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — `-f` from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb`
// with a scratch `HOME`, each module loaded and immediately unloaded:
//
//	                    ${+…} after      ${(t)…} while loaded
//	langinfo            0                association-hide-hideval-special
//	mapfile             0                association-hide-hideval-special
//	sysparams errnos    0                association-readonly-hide-hideval-…
//	epochtime           0                array-readonly-hide-hideval-special
//	EPOCHSECONDS        0                integer-readonly-hide-hideval-special
//	EPOCHREALTIME       0                (the same)
//	WATCHFMT LOGCHECK   **1**            scalar / integer
//	watch WATCH         **1**            array-special / scalar-special
//
// So a row asserting "everything the module brought goes" would be wrong for
// `zsh/watch`, and what decides is not "is it in use" — nothing referred to
// any of these — but **whose name it is**. `WATCHFMT` and `LOGCHECK` are
// ordinary parameters the module assigns a default to, and `watch` and
// `WATCH` are the shell's own specials, present in a fresh shell at
// `${+watch}` of 1 before any module is loaded. Neither kind is the module's
// to take back; the seven with `hide-hideval-special` are.
//
// Read off the two tables rather than listed a third time, which is what
// makes `zsh/watch` fall out rather than be carved out: it is in
// zshGatedParameters for `WATCHFMT` and `LOGCHECK`, it declares `p:WATCH` and
// `p:watch` in zmodloadFeatures, and the two lists are **disjoint** — so the
// intersection is empty and nothing is withdrawn.
func gatedParametersTheModuleOwns(module string) []string {
	gated := zshGatedParameters[module]
	if len(gated) == 0 {
		return nil
	}
	var out []string
	for _, feature := range zmodloadFeatures[module] {
		name, ok := strings.CutPrefix(feature, "p:")
		if !ok {
			continue
		}
		for _, g := range gated {
			if g == name {
				out = append(out, name)
			}
		}
	}
	return out
}

// releaseGatedParameters is installGatedParameters read backwards, and is
// what an unload does beyond saying it unloaded.
//
// Withdrawn rather than unregistered, for the reason the builtin half is:
// [interp.Runner.SetParameterWithdrawn] keeps what it took, so a later load
// needs nothing to have been remembered elsewhere. The name then reads as one
// this shell has not got — `${+langinfo}` of 0 and `typeset -p langinfo` is
// `no such variable: langinfo` at 1, which is exactly a fresh shell's answer
// and exactly the reference's after the unload.
func releaseGatedParameters(r *interp.Runner, module string) {
	for _, name := range gatedParametersTheModuleOwns(module) {
		r.SetParameterWithdrawn(name, true)
	}
}
