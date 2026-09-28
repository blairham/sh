// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// SetEngineOwnedPrefix says that every parameter whose name begins with this
// prefix is the engine's own state rather than a parameter of the shell being
// emulated, and that no listing a script can reach may write a row for one.
//
// A dialect with internal state that has to be **copied into a subshell** has
// one obvious place to keep it: the parameter tables, which are cloned for
// exactly that reason. zsh's does — the set of loaded modules, the `zstyle`
// table, the scheduled events, the line editor's buffer and twenty more, all
// under `.zsh.`. The names are unreachable by a *read*: `${.zsh.zmodload}` is
// `bad substitution` here and in zsh 5.9.2 alike, because a leading `.` is not
// an identifier in any dialect this shell has.
//
// **A listing is the other route, and it was open.** Measured 2026-09-28
// against `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0),
// `go version -m` says *not a Go executable* for it — `-f` from a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, after
// `zmodload zsh/langinfo`, a bare `typeset` filtered for the prefix:
//
//	zsh 5.9.2   (nothing)
//	ours        array .zsh.zmodload=( zsh/main zsh/langinfo )
//
// and the same for `typeset -p`, `typeset +`, `typeset -a`, `typeset -m`,
// `set`, `declare` and `${(k)parameters}` — eight routes, one name apiece
// there and none of them in the reference (#5014).
//
// This is the narrow half of what the issue asked for. The wide half is for
// the state to stop being parameters at all, which is a different change to
// twenty-five stores; this one says what a *listing* does, and says it once
// so that the twenty-sixth is covered by having been named.
//
// The prefix is a dialect's own, so the core has none and every other column
// is untouched: ksh93's `.sh.version` and `.sh.level` are **not** this — they
// are parameters a script there reads, produced rather than stored, and the
// listings already leave produced parameters out.
//
// **Three walks ask it**, and they are the three a dialect that names a
// prefix can reach: the whole-table declaration listing
// (Runner.declarableNames, behind `typeset`, `typeset -p`, `typeset -m`,
// `set`, `declare`, `export -p` and `readonly -p`), the parameter-name union
// ([Runner.ParameterNames] and [Runner.ParameterIsNamed], behind zsh's
// `$parameters`), and `unset -m`'s — which is not a listing at all and is the
// one with teeth: `unset -m '.zsh*'` unloaded a module before this, the
// pattern reaching state no script put there.
//
// The two bash-shaped walks — `compgen -v` and `${!p@}` — deliberately do
// **not** ask, because the only dialect that names a prefix has neither
// builtin: `compgen` is `command not found` in zsh and `${!.@}` is `bad
// substitution`, so a guard there would be code no test could reach. A
// dialect that gains both a prefix and one of those has to come here.
func (r *Runner) SetEngineOwnedPrefix(prefix string) { r.engineOwnedPrefix = prefix }

// engineOwns reports whether a name is the engine's own state, which is what
// every whole-table walk asks before it collects a name.
//
// An empty prefix owns nothing. Written that way rather than as "no prefix
// set" because the two are the same answer and one of them is a branch: a
// dialect that never called the setter has no state to hide.
func (r *Runner) engineOwns(name string) bool {
	return r.engineOwnedPrefix != "" && strings.HasPrefix(name, r.engineOwnedPrefix)
}
