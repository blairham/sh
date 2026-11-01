// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration's array-literal operand is placed before the literal behind it
// expands where the axis says so, and after the utility where it does not. The
// axis is read rather than asked, so a vector that leaves it open runs the
// declaration the way it always did. See
// Semantics.DeclarationArrayOperandsAreStoredInTurn (#6798).
func TestALiteralOperandIsPlacedInTurnWhereTheAxisSaysSo(t *testing.T) {
	t.Parallel()
	src := "a=(0)\ntypeset -a a=(1) b=(\"${a[0]}\")\necho \"${b[0]}\"\n"
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"in turn", Yes, "1\n"},
		{"after the utility", No, "0\n"},
		{"left open", Unspecified, "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, nil, func(r *Runner) {
				giveUpLineSemantics(r)
				r.Semantics.DeclarationArrayOperandsAreStoredInTurn = tc.answer
			})
			if out != tc.want || st != 0 {
				t.Errorf("output %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}
