// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The `echoti` builtin: one capability of the terminal's description, written
// out.
//
// The `zsh/terminfo` module names a parameter and a builtin — `zmodload -lF
// zsh/terminfo` is `+b:echoti` and `+p:terminfo` — and until #2142 only the
// parameter was here. That was the right call while nothing reached it and
// stopped being one with #2123: powerlevel10k's instant-prompt teardown hides
// the cursor with `echoti civis`, so a real startup printed
// `_p9k_clear_instant_prompt:9: command not found: echoti` at the first prompt
// and the cursor stayed visible through the repaint it was called for.
//
// **The answer is `$terminfo`'s, so there is one table and not two.** This
// reads the same capabilityTables the parameter reads, which is what keeps a
// capability from being present to `${+terminfo[x]}` and absent to `echoti x`.
//
// # What was measured
//
// zsh 5.9.2, `TERM=xterm-256color`, 2026-09-12, each capability written
// through `cat -v` and its status taken separately:
//
//	echoti civis    ^[[?25l          no newline
//	echoti cuu1     ^[[A             no newline
//	echoti colors   256\n
//	echoti lines    24\n
//	echoti am       yes\n
//	echoti xenl     yes\n
//	echoti hs       no\n
//	echoti Se       ^[[0 q           an extended name, and it answers
//	echoti ncv      no such terminfo capability: ncv     st=1
//	echoti bogus    no such terminfo capability: bogus   st=1
//	echoti          not enough arguments                 st=1
//
// which is three rules and one boundary:
//
//   - A **string** capability is bytes for the terminal and goes out as they
//     stand, with no newline after them. A **number** and a **boolean** are
//     answers for a person and are written as a word with a newline. That is
//     the whole of why repl.TerminalCapability carries a Kind: both readings
//     of the parameter answer with a string, and `yes` is a plausible value
//     for either.
//   - **Every boolean name answers and an absent one is `no`.** `hs` is not
//     in `xterm-256color` and is `no` rather than an error, where the absent
//     *number* `ncv` is an error. That is terminfo's own shape — a boolean is
//     false when the description does not store it — and it is already what
//     this shell's table holds, so nothing here has to know it.
//   - **A name the description has no answer for is a refusal**, by name, at
//     status 1. Not silence and not an empty line: a script testing a
//     capability reads the status.
//
// # The module, and why nothing here loads it
//
// `echoti` answers **before** `zmodload zsh/terminfo` — measured, `zsh -f -c
// 'echoti civis'` writes the sequence at status 0 — so it is registered like
// any other builtin rather than installed by the module. What the module owns
// is the ability to take it away: `zmodload -F zsh/terminfo -b:echoti` leaves
// `command not found: echoti` at 127, which is zmodload.go's existing feature
// seam and needs nothing from this file.
//
// # Parameters are refused rather than guessed
//
// A capability whose sequence takes arguments keeps terminfo's own parameter
// language in the table — `cup` is `\e[%i%p1%d;%p2%dH` — and zsh computes it
// when arguments follow: `echoti cup 3 4` is `\e[4;5H`, `echoti cup 3` is
// `\e[4;1H`, and `echoti cup` with none writes the language itself, unchanged.
//
// So the no-argument case *is* the table's value and is exact, and the
// argument case is a small stack language — `%p`, `%{n}`, `%i`, the
// arithmetic, `%?…%t…%e…%;` — that nothing measured on a real startup reaches.
// It is refused by name here rather than approximated, which is the same
// choice terminfo.go made about the builtin as a whole and for the same
// reason: a wrong sequence sent to a terminal is a corrupted screen with
// nothing said, where a refusal names its line.
func registerEchoti(r *interp.Runner, tables *capabilityTables) {
	r.Register("echoti", func(r *interp.Runner, _ context.Context, args []string) int {
		return echotiBuiltin(r, tables, args)
	})
}

func echotiBuiltin(r *interp.Runner, tables *capabilityTables, args []string) int {
	if len(args) == 0 {
		// zsh's own wording, which names no letter because this builtin has
		// none — see zle.go, whose usage complaints do name one.
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	name := args[0]
	byTerminfo, _, kinds := tables.load(r)
	value, ok := byTerminfo[name]
	if !ok {
		r.Diagnosef("no such terminfo capability: %s\n", name)
		return 1
	}
	if len(args) > 1 {
		// See the file comment: the table holds terminfo's parameter
		// language and computing it is a language of its own.
		r.Diagnosef("%s: computing a capability's parameters is not implemented yet\n", name)
		return 1
	}
	if kinds[name] == repl.StringCapability {
		// Bytes for the terminal, and nothing added to them.
		_, _ = fmt.Fprint(r.Out(), value)
		return 0
	}
	_, _ = fmt.Fprintln(r.Out(), value)
	return 0
}
