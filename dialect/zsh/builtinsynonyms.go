// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The builtins zsh lists that are another builtin under a second name, or one
// small enough to be its own sentence (#5265).
//
// Measured 2026-10-01 on zsh 5.9.2 under `-f` from a script file with `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, where every one of these is `builtin` to
// `whence -w` and in `$builtins`, and each was `none` here:
//
//	bye 3                 exits 3, as `exit 3`
//	logout 2              exits 2 from a login shell; from any other, `logout:
//	                      not login shell` and the shell exits 1 — `||` does
//	                      not catch it
//	chdir /nope           `zsh:chdir:1: no such file or directory: /nope`, as
//	                      `cd` under its own name
//	history               `fc -l`, and complains as `fc`: `zsh:fc:1: no such
//	                      event: 1`
//	r                     `fc -e -`, complaining as `fc`
//	pushln a b; getln x   `print -nz` and `read -zr`: x is `a b`
//	ttyctl -f / -u        freezes and unfreezes; bare, `tty is frozen` or `tty
//	                      is not frozen`
//	echotc md             `$termcap[md]`, padding off; `co` writes 80 and a
//	                      newline, a boolean `yes` or `no`; an unknown code is
//	                      `no such capability: xx` at 1, and nothing at 1
//	                      where there is no terminal at all
//	compcall              `can only be called from completion function`, 1
//
// `noglob` and `-` are listed too, and are precommand modifiers this shell
// reads before any lookup — `"noglob" echo a*` and `x=noglob; $x echo a*`
// already write `a*` — so here they are names in the table and nothing more.
// `zregexparse` stays where declined.go put it, deliberately.
func registerBuiltinSynonyms(r *interp.Runner) {
	delegate := func(name, as, from string, front ...string) {
		r.Register(name, func(rr *interp.Runner, ctx context.Context, args []string) int {
			b, ok := rr.Builtin(as)
			if !ok {
				return 127
			}
			return rr.RunBuiltinAs(name, from, b, ctx, append(append([]string(nil), front...), args...))
		})
	}
	delegate("bye", "exit", "exit")
	delegate("chdir", "cd", "cd")
	delegate("pushln", "print", "print", "-nz")
	delegate("getln", "read", "read", "-zr")
	// These two complain as the builtin they are, measured: `zsh:fc:1:`.
	for name, front := range map[string][]string{"history": {"-l"}, "r": {"-e", "-"}} {
		r.Register(name, func(rr *interp.Runner, ctx context.Context, args []string) int {
			fc, ok := rr.Builtin("fc")
			if !ok {
				return 127
			}
			return rr.RunBuiltinAs("fc", "", fc, ctx, append(append([]string(nil), front...), args...))
		})
	}
	r.Register("logout", logoutBuiltin)
	r.Register("ttyctl", ttyctlBuiltin)
	r.Register("compcall", func(rr *interp.Runner, _ context.Context, _ []string) int {
		rr.Diagnosef("can only be called from completion function\n")
		return 1
	})
	for _, name := range []string{"noglob", "-"} {
		// Reached only where the modifier was not read as one, which the
		// expansion does for every spelling measured; the names are here so
		// that the table says what zsh's does.
		r.Register(name, func(rr *interp.Runner, ctx context.Context, args []string) int {
			if len(args) == 0 {
				return 0
			}
			b, ok := rr.Builtin("builtin")
			if !ok {
				return 127
			}
			return b(rr, ctx, args)
		})
	}
}

// logoutBuiltin is `exit` for a login shell and a refusal that still exits for
// any other. See registerBuiltinSynonyms for the measurement.
func logoutBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	exit, ok := r.Builtin("exit")
	if !ok {
		return 127
	}
	if !r.LoginShell {
		r.Diagnosef("not login shell\n")
		return r.RunBuiltinAs("logout", "exit", exit, ctx, []string{"1"})
	}
	return r.RunBuiltinAs("logout", "exit", exit, ctx, args)
}

// ttyctlBuiltin keeps the bit `ttyctl` reports, on the Runner where the front
// end reads it. Frozen means a change a command makes to the terminal's
// settings is undone when the command ends. Unfrozen means it is kept, as
// zsh keeps it (#6105). See interp.Runner.TerminalFrozen.
func ttyctlBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	rest, letters, _, code := r.BuiltinOptions("ttyctl", args, "fu")
	if code != 0 {
		return code
	}
	_ = rest
	for _, c := range letters {
		switch c {
		case 'f':
			r.TerminalFrozen = true
		case 'u':
			r.TerminalFrozen = false
		}
	}
	if letters == "" {
		if r.TerminalFrozen {
			_, _ = fmt.Fprintln(r.Out(), "tty is frozen")
		} else {
			_, _ = fmt.Fprintln(r.Out(), "tty is not frozen")
		}
	}
	return 0
}

// registerEchotc installs `echotc`, `echoti` by termcap code.
func registerEchotc(r *interp.Runner, tables *capabilityTables) {
	r.Register("echotc", func(rr *interp.Runner, _ context.Context, args []string) int {
		if len(args) == 0 {
			rr.Diagnosef("not enough arguments\n")
			return 1
		}
		if len(tables.termcapTable(rr)) == 0 {
			// No terminal at all: measured, `echotc zz` with TERM unset is
			// silent at 1.
			return 1
		}
		value, kind, ok := tables.termcapEntry(rr, args[0])
		if !ok {
			rr.Diagnosef("no such capability: %s\n", args[0])
			return 1
		}
		switch kind {
		case repl.StringCapability:
			if need := highestParameter(value); len(args)-1 < need {
				// The arguments a parameterized string reads have to be
				// there: measured, `echotc cm` and `echotc cm 1` under xterm
				// are both `not enough arguments`, where echoti writes the
				// language.
				rr.Diagnosef("not enough arguments\n")
				return 1
			} else if len(args)-1 > need {
				// And no more than it reads: `echotc md 3` and `echotc cm 1 2
				// 3` are `too many arguments`.
				rr.Diagnosef("too many arguments\n")
				return 1
			}
			if len(args) > 1 {
				params := make([]int, 0, len(args)-1)
				for _, a := range args[1:] {
					n, _ := strconv.Atoi(a)
					params = append(params, n)
				}
				value = repl.ParameterizedString(value, params)
			}
			_, _ = fmt.Fprint(rr.Out(), repl.WithoutPadding(value))
		default:
			_, _ = fmt.Fprintln(rr.Out(), value)
		}
		return 0
	})
}

// highestParameter is the highest `%pN` a capability string reads, or 0.
func highestParameter(s string) int {
	n := 0
	for i := 0; i+2 < len(s); i++ {
		if s[i] == '%' && s[i+1] == 'p' && s[i+2] >= '1' && s[i+2] <= '9' {
			if d := int(s[i+2] - '0'); d > n {
				n = d
			}
		}
	}
	return n
}
