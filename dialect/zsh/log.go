// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `log`, which reports the logins and logouts of the users `$watch` names.
//
// It exists here for a reason that is not the feature: macOS's `/etc/zshrc`
// holds one line whose whole job is to get this builtin out of the way of
// `/usr/bin/log`, and a shell that reads the system-wide startup files —
// which this one does since #1717 — meets it before the first prompt. With no
// `log` in the table, `disable log` is `no such hash table element: log` at 1
// on every interactive start (#2325).
//
// ## Measured 2026-09-12, zsh 5.9.2 under `-f`
//
//	log                        nothing, 0        `$watch` unset, which is the default
//	watch=(); log              nothing, 0
//	watch=(nosuchuser); log    nothing, 0        nobody by that name is on
//	watch=(all); log           bhamilton has logged on console from .
//	log a b                    zsh:log:1: too many arguments, 1
//	log -x                     the same — it takes no options at all
//	log ""                     the same — an empty word is an operand
//	whence -w log              log: builtin
//	disable log; whence -w log log: command
//
// Three of those five shapes are a builtin that takes no operands and reports
// nothing; the fourth is the feature and the fifth is the refusal.
//
// ## Why the feature is not here
//
// Reporting the watched users needs `$watch`, `$watchfmt`, `$LOGCHECK` and a
// reader for the system's login records — a utmp-shaped file whose layout is
// the platform's rather than the shell's. That is a feature, and
// dialect/zsh/enable.go's rule applies to it as squarely as to a hash table
// this shell does not keep: a `log` that answered nothing whatever `$watch`
// said would be the silent wrong answer, because a script that set `$watch`
// and got silence would read it as *nobody is logged on*.
//
// So the two answers are told apart. With `$watch` empty or unset — which is
// every startup file that only wants the name out of the way — this is the
// measured silence at 0. With `$watch` naming somebody it is refused by name,
// the way every other unimplemented facility here is, so a script can tell a
// shell that lacks the feature from an empty login table.
func logBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	if len(args) > 0 {
		// Any operand, including an option-shaped one and an empty word.
		// There are no options: measured, `log -x` is this same sentence
		// rather than a bad option.
		r.Diagnosef("log: too many arguments\n")
		return 1
	}
	if watched, ok := r.GetArray("watch"); !ok || len(watched) == 0 {
		return 0
	}
	r.Diagnosef("log: reporting the users $watch names is not implemented yet\n")
	return 2
}
