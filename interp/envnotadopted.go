// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// MarkEnvironmentEntryNotAdopted says the shell's parameter of this name is
// **not** the environment's entry of it.
//
// A name the environment supplies is normally the parameter: the value comes
// in, the export attribute comes with it, and a child sees whatever the shell
// has by the time it starts one. A handful of names in one dialect are not
// that, and they are not that in a way no single existing state could say —
// which is the third state [Runner.isExported] was missing.
//
// # The three things it says at once
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` calls it *not a Go
// executable* where it calls ours `github.com/blairham/sh/cmd/zsh`, so the two
// columns are two programs — with a script run `-f` under
// `env -i PATH=/usr/bin:/bin IFS=ZZ UID=QQ` and `/usr/bin/env` as the child:
//
//	${(t)IFS}                scalar-special       and not scalar-export-special
//	${IFS}                   the shell's own      and not ZZ
//	the child's environment  IFS=ZZ               the entry, as it arrived
//
// The first is what a listing writes, the second is what
// [Runner.SetSpecial]'s `environmentMaySupply: false` row already answered,
// and the third is the one that says this is not simply "the export came
// off": `export -n IFS` would take the name out of a child's environment
// altogether, and here the entry is still there and still holds `ZZ`.
//
// # What moves it and what does not
//
// The same run, one script per row:
//
//	as inherited               IFS=ZZ  UID=QQ
//	after IFS=x                IFS=ZZ  UID=QQ
//	after unset IFS            IFS=ZZ  UID=QQ
//	after IFS=x; export IFS    IFS=x
//	after export IFS           IFS=<the shell's own>
//	after export UID           UID=501
//
// So an assignment does not reach the entry and neither does `unset` — the
// two states that normally decide what a child is handed — and an explicit
// `export` is the one thing that makes the shell's own value supersede it.
// `${(t)IFS}` follows the same split: `scalar-special` after an assignment
// and after an `unset` with a reassignment, `scalar-export-special` only
// after the `export`.
//
// It is the *name* that is marked and not a value, and it is marked whether
// or not the environment supplied anything: nothing about it applies until
// there is an entry to leave alone.
func (r *Runner) MarkEnvironmentEntryNotAdopted(name string) {
	if r.envNotAdopted == nil {
		r.envNotAdopted = map[string]bool{}
	}
	r.envNotAdopted[name] = true
}
