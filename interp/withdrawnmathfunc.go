// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A **withdrawn math function** is one this shell has and a dialect's module
// selection has taken out of the table arithmetic looks in. The name resolves
// to nothing, and `$(( sqrt(4) ))` is `unknown function: sqrt` — the same
// sentence a name nobody registered gets.
//
// The `f:` third of what [Runner.SetBuiltinWithdrawn] and
// [Runner.SetParameterWithdrawn] do for the other two feature kinds, and the
// arithmetic seam differs from both in the one way the shell does: a
// withdrawn *builtin* is looked up on PATH and told `command not found`, a
// withdrawn *parameter* says nothing at all, and a withdrawn math function
// refuses by name inside the expression that called it. Measured against zsh
// 5.9.2, 2026-09-28, in a fresh shell that has not loaded `zsh/mathfunc`:
//
//	$(( sqrt(4) ))      unknown function: sqrt    status 1
//	$(( nosuchmf(1) ))  unknown function: nosuchmf
//
// — the same line for a name the module would bring and a name nothing has,
// which is what says the withdrawal is the right shape and not a third
// diagnostic.
//
// The implementation is kept rather than discarded, for the reason the
// builtin half records: the selection moves in both directions — `+f:sqrt`
// and a plain `zmodload zsh/mathfunc` each put it back — and a dialect that
// had to re-register would need a second table of feature names to functions,
// which is the first thing to drift from the registrations it copies.
//
// **[Runner.KnownMathFunction] goes on saying yes for a withdrawn name**, and
// that is load-bearing rather than tidy: a module gate asks it to decide
// whether this shell *has* a feature, and a withdrawn function is one the
// shell has and is not currently offering. Without the split, withdrawing the
// forty-seven at startup would make `zmodload zsh/mathfunc` refuse with
// `not implemented yet` — the gate firing at the very names the module is
// about to bring, which is the trap #4922 records for parameters and #5029
// for builtins.
//
// The state is per-Runner and is cloned into a subshell with the table it was
// taken out of, so a selection made inside `( … )` is the subshell's.

// SetMathFunctionWithdrawn takes a name out of the table arithmetic looks in,
// or puts it back. The implementation is kept either way.
func (r *Runner) SetMathFunctionWithdrawn(name string, withdrawn bool) {
	if !withdrawn {
		delete(r.withdrawnMathFuncs, name)
		return
	}
	if r.withdrawnMathFuncs == nil {
		r.withdrawnMathFuncs = map[string]bool{}
	}
	r.withdrawnMathFuncs[name] = true
}

// MathFunctionWithdrawn reports whether a name has been taken out of the
// table.
func (r *Runner) MathFunctionWithdrawn(name string) bool {
	return r.withdrawnMathFuncs[name]
}
