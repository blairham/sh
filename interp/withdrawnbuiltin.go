// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A **withdrawn builtin** is one this shell has and a dialect's module
// selection has taken out of the table. The name resolves to nothing: it is
// looked up on PATH like any other word, and a script calling it is told
// `command not found`.
//
// It is the builtin half of what [Runner.SetAbsentParameter] does for a
// parameter, and it is a third state beside registered and disabled rather
// than a spelling of either. Measured against zsh 5.9.2, 2026-09-10, with
// `zmodload zsh/zutil; zmodload -F zsh/zutil -b:zparseopts`:
//
//	zparseopts       command not found: zparseopts   127
//	whence zparseopts                                1
//	type zparseopts  zparseopts not found            1
//	disable          (nothing listed)                0
//	enable zparseopts  no such hash table element    1
//
// So `enable -n` is the wrong seam and not merely an inexact one: a disabled
// builtin is a name that shell still knows about, lists and will put back,
// and every one of the five lines above says this is not that. The function
// is kept rather than discarded, because the selection moves in both
// directions — `+b:zparseopts` and a plain `zmodload` of the module each put
// it back — and a dialect that had to re-register would need a second table
// of feature names to builtins, which is the first thing to drift from the
// registrations it copies.
//
// The state is per-Runner and is cloned into a subshell with the rest of the
// tables, which is what a selection made inside `( … )` needs: measured, the
// deselection holds in the subshell and the parent still has the builtin
// afterwards.

// SetBuiltinWithdrawn takes a name out of the builtin table, or puts it back.
// The builtin itself is kept either way.
func (r *Runner) SetBuiltinWithdrawn(name string, withdrawn bool) {
	if !withdrawn {
		delete(r.withdrawnBuiltins, name)
		return
	}
	if r.withdrawnBuiltins == nil {
		r.withdrawnBuiltins = map[string]bool{}
	}
	r.withdrawnBuiltins[name] = true
}

// BuiltinWithdrawn reports whether a name has been taken out of the table.
func (r *Runner) BuiltinWithdrawn(name string) bool {
	return r.withdrawnBuiltins[name]
}
