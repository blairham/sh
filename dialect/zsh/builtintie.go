// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The ties this shell makes for itself, before a script says anything.
//
// `typeset -T SCALAR array` is the letter a script writes (see
// interp/tiedscalar.go); these are the pairs the shell arrives with. It is
// what makes `$path` an array at all here, and what makes writing it reach
// `PATH` — `path=( /new "${path[@]}" )` is how every rc file in the world
// puts a directory on the search path, and without the tie it wrote an
// ordinary array that nothing read.
//
// Measured 2026-09-06 against zsh 5.9.2 under `env -i` with a scratch HOME
// and no startup files, one pair at a time with `typeset -p`. Eight pairs,
// every one of them joined on `:`:
//
//	export -T PATH path            typeset -T MANPATH manpath
//	typeset -T FPATH fpath         typeset -T MAILPATH mailpath
//	typeset -T CDPATH cdpath       typeset -T MODULE_PATH module_path
//	typeset -T PSVAR psvar         typeset -T FIGNORE fignore
//
// **The export attribute is inherited, never conferred.** `PATH` lists as
// `export -T` when the environment supplied it and as plain `typeset -T`
// when it did not — measured both ways, `env -i` with and without a `PATH`
// — and writing `cdpath` never puts `CDPATH` in a child's environment. So
// nothing here exports anything; the tie carries whatever the scalar
// already was.
//
// What is deliberately **not** here:
//
//   - **`ZSH_EVAL_CONTEXT`/`zsh_eval_context`**, which that shell lists among
//     its ties. It is a *produced* parameter — `typeset -p
//     ZSH_EVAL_CONTEXT` writes nothing there — and a produced parameter is a
//     different mechanism from a tie. It stays absent rather than being tied
//     to a stored value that would then be wrong.
//
//   - **Any default value.** zsh fills `FPATH` with its own function
//     directories and `MODULE_PATH` with its module directory when the
//     environment names neither, and those are that installation's files.
//     Inventing them here would point `autoload` at another shell's
//     function library. So a pair the environment says nothing about starts
//     empty, which is exactly what the same shell does for `CDPATH`.
//
//     `FPATH` has a default now and it is still not written here, which is
//     the same rule seen from the other side: this dialect names the
//     parameter — Semantics.FunctionSearchVariable — and the front end fills
//     it with *this* installation's directories, read off where the running
//     binary was installed. A path in this file would be the machine the
//     line was typed on. The tie is what carries it into `fpath`, which is
//     why the seed happens after this runs (#1250).
func tieTheBuiltInPairs(r *interp.Runner) {
	for _, pair := range [...][2]string{
		{"PATH", "path"},
		{"FPATH", "fpath"},
		{"CDPATH", "cdpath"},
		{"MANPATH", "manpath"},
		{"MAILPATH", "mailpath"},
		{"MODULE_PATH", "module_path"},
		{"PSVAR", "psvar"},
		{"FIGNORE", "fignore"},
	} {
		r.Tie(pair[0], pair[1], ":")
	}
}
