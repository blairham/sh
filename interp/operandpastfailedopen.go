// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// applyArrayOperandsPastAFailedOpen stores a declaration utility's
// array-literal operands even though the command's redirection failed and the
// utility never ran.
//
// One column does this and it is the array literal alone. Measured
// 2026-09-19 over a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	typeset a=(VAL) 2>/nope/x
//	echo "[${a[0]}]"
//
//	bash 5.3.20, bash 3.2.57   the open's complaint, then `[VAL]`
//	zsh 5.9.2                  the complaint, then `[]`
//	ksh93u+                    the complaint, and the shell stops
//	dash, BusyBox ash          no array literal, so no question
//
// **The control is what makes this narrow.** A *scalar* operand is not
// applied in bash either — `export s=$(echo VAL) 2>/nope/x` leaves `s` empty
// there — so this is not "bash performs a declaration's assignments before it
// opens anything". It is the array literal, and nothing else.
//
// ksh93's row is settled elsewhere and cannot reach here: a failed
// redirection on a declaration utility ends that shell, so there is no next
// command to observe the name from. That is
// Semantics.RedirectErrorOnSpecialBuiltinFatal, asked just above.
//
// The elements were already expanded ahead of the open — that is #3806, which
// this shell now does too, and it is why `typeset a=($(echo VAL; echo SIDE
// >&2)) 2>/nope/x` shows `SIDE` in every column that has array literals. What
// this adds is the *store* running anyway, which is the half bash has and we
// did not.
func (r *Runner) applyArrayOperandsPastAFailedOpen(ctx context.Context, c *syntax.SimpleCmd) {
	for _, a := range c.Assigns {
		if !a.Operand {
			continue
		}
		e := r.expandedArrayOperand(a)
		if e == nil {
			// A scalar operand, which no column applies on this path. Asked
			// per operand rather than per command because one command can
			// carry both, and bash keeps them apart.
			continue
		}
		if !r.ask(r.sem().ArrayOperandIsStoredPastAFailedOpen,
			"a declaration's array-literal operand stored after its redirection failed") {
			return
		}
		r.withPreparedValue(ctx, e)
	}
}
