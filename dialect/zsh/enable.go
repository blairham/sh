// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"

	"github.com/blairham/sh/interp"
)

// `enable` and `disable` here are not the `enable` the other dialect has.
//
// bash's is one builtin with `-n` to switch a name off. zsh's are a pair, and
// what they act on is a *hash table* chosen by the option: builtins by
// default, and aliases, functions, patterns, reserved words or suffix aliases
// with `-a`, `-f`, `-m`, `-r` and `-s`. `-n` is not an option there at all.
//
// So the core's `enable` is unregistered for this dialect and these are
// registered in its place, rather than one builtin trying to be both — which
// would accept `-n`, which zsh refuses, and refuse `-f`, which zsh takes.
//
// The builtins table is what these implement. The other five are refused as
// not implemented rather than silently doing nothing, and the listing is of
// *this* shell's builtins, which are not zsh's — nothing can make those the
// same set, and pretending otherwise would be the silent wrong answer this
// project exists to avoid.

// enableBuiltin builds `enable` or `disable`; they differ only in which way
// they switch a name and which list they print when given nothing.
func enableBuiltin(name string, on bool) interp.Builtin {
	return func(r *interp.Runner, _ context.Context, args []string) int {
		rest, code := hashTableOptions(r, name, args)
		if code != 0 {
			return code
		}
		if len(rest) == 0 {
			return listNames(r, on)
		}
		status := 0
		for _, a := range rest {
			// A withdrawn name is not in the table to switch either way: a
			// module selection took it out, and until the selection puts it
			// back it is no more a hash table element than a name this shell
			// never had. Measured on zsh 5.9.2, 2026-09-12, after `zmodload
			// -F zsh/zutil -b:zparseopts`, both `enable zparseopts` and
			// `disable zparseopts` are `no such hash table element` at 1.
			//
			// KnownBuiltin goes on saying yes for it on purpose — `+b:` has
			// to be able to put it back, and zmodloadHasFeature leans on
			// that — so the two questions are asked separately here rather
			// than folded into one.
			if !r.KnownBuiltin(a) || r.BuiltinWithdrawn(a) {
				r.Diagnosef("%s: no such hash table element: %s\n", name, a)
				status = 1
				continue
			}
			r.SetBuiltinEnabled(a, on)
		}
		return status
	}
}

// listNames prints the names in the table, one per line and bare — no
// `enable ` in front of them, which is the other dialect's shape.
func listNames(r *interp.Runner, enabled bool) int {
	names := r.DisabledBuiltins()
	if enabled {
		names = r.BuiltinNames()
	}
	for _, n := range names {
		_, _ = fmt.Fprintln(r.Out(), n)
	}
	return 0
}

// hashTableOptions reads the leading options, which name a table rather than
// modify an action.
func hashTableOptions(r *interp.Runner, builtin string, args []string) ([]string, int) {
	// One step rather than a loop: every option here names the table and is
	// the whole of what this builtin was asked, so nothing is ever read
	// twice. `--` hands back what follows it, and the rest return.
	if len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			return args, 0
		}
		if a == "--" {
			return args[1:], 0
		}
		switch a {
		case "-a", "-f", "-m", "-r", "-s":
			// A table this shell does not keep. Said out loud rather than
			// passed over: `disable -a foo` that quietly did nothing would
			// leave the alias in place and report success.
			r.Diagnosef("%s: %s is not implemented yet\n", builtin, a)
			return nil, 2
		}
		r.Diagnosef("%s: bad option: %s\n", builtin, a)
		return nil, 1
	}
	return args, 0
}

// registerEnable puts both in place, replacing the core's `enable`.
func registerEnable(r *interp.Runner) {
	r.Register("enable", enableBuiltin("enable", true))
	r.Register("disable", enableBuiltin("disable", false))
}
