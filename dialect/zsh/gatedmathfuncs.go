// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The math functions that do not exist until `zsh/mathfunc` is loaded.
//
// The **`f:` third** of the same gate, and the last of the three: #4922 did
// the parameters, #5029 the builtins, and this was left standing by both —
// `zmodloadEnforce` says in so many words that "a `c:` or `f:` feature is
// left alone", which was true of the selection and became untrue of startup
// the day the other two were gated.
//
// Measured 2026-09-28 on `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable*
// for it — `-f` from a script file under `env -i PATH=/usr/bin:/bin
// TERM=dumb` with a scratch `HOME` and stdin at `/dev/null`:
//
//	                        zsh 5.9.2                 before
//	$(( sqrt(4) ))          unknown function: sqrt    2.
//	$(( floor(4) ))         unknown function: floor   4.
//	$(( abs(4) ))           unknown function: abs     4
//
// # The roster is the module and is not listed again here
//
// All forty-seven, read off mathFuncModule — the table that registers them —
// so a forty-eighth is gated by having been written rather than by being
// remembered in two places. That is the same derivation
// zmodloadWithdrawnAtStart makes for the builtin half from
// zshWithdrawnPlainNames.
//
// # Nothing installs them back but the load path that was already there
//
// zmodloadEnforce is the other writer of the withdrawn state, and a plain
// `zmodload` widens the selection to every feature the module names, so the
// functions come back by the same road `zmodload -F zsh/mathfunc +f:sqrt`
// brings one back — and a *narrowed* load puts back only what it named,
// which is #5045's transition rule and which an installer keyed on the module
// could not see.
func withdrawGatedMathFuncs(r *interp.Runner) {
	for _, f := range mathFuncModule {
		r.SetMathFunctionWithdrawn(f.name, true)
	}
}

// zshGatedMathFuncModule is the module the names above wait for, kept beside
// them so that the release path and the enforcement read one answer.
const zshGatedMathFuncModule = "zsh/mathfunc"

// releaseGatedMathFuncs is withdrawGatedMathFuncs read backwards, and is what
// an unload does for the third kind.
//
// Measured 2026-09-28, the same conditions: `zmodload zsh/mathfunc; zmodload
// -u zsh/mathfunc; print $(( sqrt(4) ))` is `unknown function: sqrt` at 1 in
// zsh 5.9.2 and was `2.` here. Withdrawn rather than unregistered, for the
// reason the other two halves are — see
// interp.Runner.SetMathFunctionWithdrawn, which keeps the implementation.
//
// Only this module's names, asked of the module rather than of the roster, so
// an unload of anything else leaves them exactly as it found them.
func releaseGatedMathFuncs(r *interp.Runner, module string) {
	if module != zshGatedMathFuncModule {
		return
	}
	withdrawGatedMathFuncs(r)
}
