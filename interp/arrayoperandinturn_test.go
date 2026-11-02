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

// A declaration's array-literal operand is expanded where it is written,
// between the words either side of it, whatever the axis above answers: every
// shell with the construct does, and the axis says only whether the literal is
// placed before the next word expands. Measured 2026-10-30 from script files
// against GNU bash 5.3.20, bash 3.2.57, ksh93u+ and zsh 5.9.2. The literals
// were expanded after every other word where the axis said No, so `s` came out
// ahead of `A`, and a literal that failed did so after the words behind it had
// run (#7075).
func TestALiteralOperandIsExpandedWhereItIsWritten(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ name, src, want string }{
		{
			"a literal before a scalar",
			"typeset a=($(echo A >&2)) y=$(echo s >&2)\n",
			"A\ns\n",
		},
		{
			"a literal between two scalars",
			"typeset y=$(echo s >&2) a=($(echo A >&2)) z=$(echo t >&2)\n",
			"s\nA\nt\n",
		},
		{
			"a literal that fails stops the words behind it",
			"typeset y=$(echo s >&2) a=(1 $((1/0)) $(echo A >&2)) z=$(echo t >&2)\necho next\n",
			"s\nsh: division by zero\nnext\n",
		},
	} {
		for _, answer := range []Answer{Yes, No, Unspecified} {
			t.Run(row.name+"/"+answerName(answer), func(t *testing.T) {
				t.Parallel()
				out, st := runGrammar(t, row.src, nil, func(r *Runner) {
					giveUpLineSemantics(r)
					r.Semantics.DeclarationArrayOperandsAreStoredInTurn = answer
				})
				if out != row.want || st != 0 {
					t.Errorf("output %q status %d, want %q", out, st, row.want)
				}
			})
		}
	}
}
