// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
// The builtins table, the functions table (`-f`) and the declaration words of
// the reserved-word table (`-r`) are what these implement. The other three
// are refused as not implemented rather than silently doing
// nothing, and the builtins listing is of *this* shell's builtins, which are
// not zsh's — nothing can make those the same set, and pretending otherwise
// would be the silent wrong answer this project exists to avoid.

// enableBuiltin builds `enable` or `disable`; they differ only in which way
// they switch a name and which list they print when given nothing.
func enableBuiltin(name string, on bool) interp.Builtin {
	return func(r *interp.Runner, _ context.Context, args []string) int {
		rest, table, code := hashTableOptions(r, name, args)
		if code != 0 {
			return code
		}
		if table == 'f' {
			return switchFunctions(r, name, on, rest)
		}
		if table == 'r' {
			return switchReservedWords(r, name, on, rest)
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
func hashTableOptions(r *interp.Runner, builtin string, args []string) ([]string, byte, int) {
	// One step rather than a loop: every option here names the table and is
	// the whole of what this builtin was asked, so nothing is ever read
	// twice. `--` hands back what follows it, and the rest return.
	if len(args) > 0 {
		a := args[0]
		if a == "" || a[0] != '-' {
			return args, 0, 0
		}
		if a == "--" {
			return args[1:], 0, 0
		}
		if a == "-" {
			// A lone `-`, eaten in this dialect and an operand elsewhere.
			// The guard above used to be `len(a) < 2`, which handed it to the
			// operands unasked — so `enable - -f x` named the dash as a table
			// element and wrote a complaint the reference never writes
			// (#5040). See interp.Runner.ReadALoneDash.
			switch r.ReadALoneDash() {
			case interp.LoneDashUnanswered:
				return nil, 0, 2
			case interp.LoneDashEndsTheOptions:
				return args[1:], 0, 0
			}
			return args, 0, 0
		}
		switch a {
		case "-f":
			return args[1:], 'f', 0
		case "-r":
			return args[1:], 'r', 0
		case "-a", "-m", "-s":
			// A table this shell does not keep. Said out loud rather than
			// passed over: `disable -a foo` that quietly did nothing would
			// leave the alias in place and report success.
			r.Diagnosef("%s: %s is not implemented yet\n", builtin, a)
			return nil, 0, 2
		}
		r.Diagnosef("%s: bad option: %s\n", builtin, a)
		return nil, 0, 1
	}
	return args, 0, 0
}

// switchFunctions is `enable -f` and `disable -f`: the functions table, which
// a script fills itself, switched a name at a time.
//
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`), with `zq() { print zq; }`:
//
//	disable -f zq       0, and `zq` is then `command not found` at 127
//	disable -f zq       again, 0 — already off is no change
//	enable -f zq        0 — on whether or not it was off
//	disable -f nosuch   no such hash table element: nosuch, 1
//	disable -f          the switched-off definitions, as `functions` lists
//	enable -f           the live ones, the same way
//	disable -f 'z*'     no such hash table element: z*, 1 — not a pattern
//
// What "off" is lives in the core: see interp.Runner.SetFunctionWithdrawn.
func switchFunctions(r *interp.Runner, builtin string, on bool, names []string) int {
	if len(names) == 0 {
		list, text := r.ListedFuncNames(), r.FunctionText
		if !on {
			list, text = r.WithdrawnFunctionNames(), r.WithdrawnFunctionText
		}
		for _, n := range list {
			if def, ok := text(n); ok {
				_, _ = fmt.Fprintln(r.Out(), strings.TrimSuffix(def, "\n"))
			}
		}
		return 0
	}
	status := 0
	for _, n := range names {
		if !r.SetFunctionWithdrawn(n, !on) {
			r.Diagnosef("%s: no such hash table element: %s\n", builtin, n)
			status = 1
		}
	}
	return status
}

// switchReservedWords is `enable -r` and `disable -r`: the reserved-word
// table, a name at a time — listed in sorted order, as `enable -r` lists it
// there.
//
// Only the declaration reserved words can be switched here — see
// interp.Runner.SetReservedWordEnabled for what off means and the rows
// measured. The rest of the table is the grammar's own and is refused by
// name rather than quietly left on: `disable -r if` that did nothing would
// report success over an `if` that still parses. Measured on zsh 5.9.2,
// 2026-10-02: `disable -r nosuch` is `no such hash table element: nosuch`
// at 1, and `disable -r` with no names lists the switched-off words.
func switchReservedWords(r *interp.Runner, builtin string, on bool, names []string) int {
	if len(names) == 0 {
		for _, n := range slices.Sorted(slices.Values(zshReservedWords)) {
			if r.ReservedWordDisabled(n) != on {
				_, _ = fmt.Fprintln(r.Out(), n)
			}
		}
		return 0
	}
	status := 0
	for _, n := range names {
		if r.SetReservedWordEnabled(n, on) {
			continue
		}
		if zshReserves(n) {
			r.Diagnosef("%s: -r %s is not implemented yet\n", builtin, n)
			status = 2
			continue
		}
		r.Diagnosef("%s: no such hash table element: %s\n", builtin, n)
		status = 1
	}
	return status
}

// registerEnable puts both in place, replacing the core's `enable`.
func registerEnable(r *interp.Runner) {
	r.Register("enable", enableBuiltin("enable", true))
	r.Register("disable", enableBuiltin("disable", false))
}
