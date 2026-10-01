// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
// # Parameters
//
// A capability whose sequence takes arguments keeps terminfo's own parameter
// language in the table — `cup` is `\e[%i%p1%d;%p2%dH` — and zsh computes it
// when arguments follow: `echoti cup 3 4` is `\e[4;5H`, `echoti cup 3` is
// `\e[4;1H`, and `echoti cup` with none writes the language itself, unchanged.
// The computing is tparm.go, implemented from terminfo(5) and measured
// against zsh across four terminal types (#5150). It was refused by name
// before that, on the reasoning that a wrong sequence sent to a terminal is a
// corrupted screen with nothing said.
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
	byTerminfo, kinds := tables.terminfoTable(r)
	value, ok := byTerminfo[name]
	if !ok {
		r.Diagnosef("no such terminfo capability: %s\n", name)
		return 1
	}
	if len(args) > 1 {
		// The table holds terminfo's parameter language, computed here with
		// the arguments as numbers. See tparm.
		params := make([]int, 0, len(args)-1)
		for _, a := range args[1:] {
			n, _ := strconv.Atoi(a)
			params = append(params, n)
		}
		_, _ = fmt.Fprint(r.Out(), withoutPadding(tparm(value.Str, params)))
		return 0
	}
	if kinds[name] == repl.StringCapability {
		// Bytes for the terminal, less the padding a terminal never reads.
		_, _ = fmt.Fprint(r.Out(), withoutPadding(value.Str))
		return 0
	}
	_, _ = fmt.Fprintln(r.Out(), value.Str)
	return 0
}

// withoutPadding takes out terminfo's padding specifications, `$<n>` with an
// optional `*` or `/` after the number, which are a delay for whatever writes
// the string and not bytes for the terminal. Measured on zsh 5.9.2 under
// TERM=vt100, whose `cup` is `\e[%i%p1%d;%p2%dH$<5>`: `echoti cup 3 4` is
// `\e[4;5H` and a bare `echoti cup` is the language without the `$<5>` (#5150).
func withoutPadding(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '<' {
			if j := strings.IndexByte(s[i:], '>'); j > 0 && paddingSpec(s[i+2:i+j]) {
				i += j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// paddingSpec reports whether the text between `$<` and `>` is a delay: a
// number, possibly with a fraction, and the `*` and `/` terminfo(5) allows.
func paddingSpec(s string) bool {
	digits := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' || c == '*' || c == '/':
		default:
			return false
		}
	}
	return digits
}
