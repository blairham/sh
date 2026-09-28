// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The names whose parameter here is **not** the environment's entry of that
// name.
//
// A name the environment hands in and this shell then supplies a value for
// kept the `export` the environment brought, where the reference takes it
// off: `env IFS=ZZ` listed `scalar-export-special` here and
// `scalar-special` there (#4940). The value was already right in every row —
// that is [interp.Runner.SetSpecial] and the `environmentMaySupply: false`
// row in startupvalues.go — and what survived was the attribute, because a
// name the environment supplied was exported *by having been supplied* and
// nothing took that back.
//
// # The rule is not the one the issue proposed, and the grid says so
//
// #4940 read the split as `special` deciding it, off four rows that are all
// `special`. Swept over every one of the 123 names a fresh `zsh -f` lists,
// one shell per cell, `env -i PATH=/usr/bin:/bin` plus the single name set to
// `ZZ`, the reference drops the export from the 24 names below and **keeps**
// it on `$PPID`, `$RANDOM`, `$SECONDS`, `$SHLVL`, `$LINENO`, `$status` and
// `$ARGC`, which are `special` too and whose own value wins just as plainly.
// So neither `special` nor "the shell's value won" is what decides it, and a
// rule read off the four rows in the issue would have taken the export off
// seven names that carry it.
//
// This is therefore a **table** and not a derivation, which is the same shape
// and the same reason as `environmentMaySupply` beside it.
//
// # The sweep
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so the two columns
// are two programs. Every name below read `${(t)NAME}` with no export word in
// the reference and with one here, and the base word — the same read with
// nothing in the environment — already agreed on every row, which is what
// makes the export the whole of the difference.
//
// The module tables are deliberately **not** here even though they answer
// this read differently too: what parts them is the base word, an
// `association` here against a plain `scalar` in a reference that has not
// loaded the module, and that is #4909's job rather than this one's.
//
// # What it costs a child
//
// The entry goes on to a child as it arrived, which is measured and is not
// what taking an export off normally does — see
// [interp.Runner.MarkEnvironmentEntryNotAdopted], where the six rows are.
// That half is worth more than the listing word here: `$IFS` holds a NUL in
// this shell, as it does in the reference, and exporting it made **every**
// command fail to start under `env IFS=x` with `environment variable
// contains NUL`.
func markTheEntriesThisShellDoesNotAdopt(r *interp.Runner) {
	for _, name := range entriesNotAdopted {
		r.MarkEnvironmentEntryNotAdopted(name)
	}
}

// entriesNotAdopted is the sweep's answer, in the order a listing writes it.
//
// Grouped by what each name is rather than alphabetically, because the groups
// are the only structure there is — the rule is per name.
var entriesNotAdopted = [...]string{
	// The identities. `$UID` and `$EUID` are the row #4912's
	// `environmentMaySupply: false` was written for, and `$GID` and `$EGID`
	// sit beside them.
	"EGID", "EUID", "GID", "UID",
	"USERNAME",
	// The field separator, whose value holds a NUL in both shells.
	"IFS",
	// The history characters, under both spellings — one parameter, and the
	// sweep answered the same for each.
	"HISTCHARS", "histchars",
	// `getopts`' place in the operands, which the shell keeps itself.
	"OPTIND",
	// The terminal input hack and the two `try` block records.
	"KEYBOARD_HACK", "TRY_BLOCK_ERROR", "TRY_BLOCK_INTERRUPT",
	// The last argument of the command before this one.
	"_",
	// The positional parameters under the name zsh writes them with.
	"argv",
	// The array halves of the ties, and the one scalar half whose upper-case
	// spelling is not exported either. `$PATH` is the control beside this
	// group and is **not** here: `env PATH=ZZ` is `scalar-tied-export-special`
	// in the reference, so a rule about tied names would have been wrong.
	"cdpath", "fignore", "fpath", "mailpath", "manpath",
	"module_path", "MODULE_PATH", "path", "psvar",
	// The signal roster.
	"signals",
}
