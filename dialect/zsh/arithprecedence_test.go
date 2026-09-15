// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell's arithmetic operators do not bind in C's order —
// syntax.Dialect.ArithPrecedence, and the one column of the panel that
// answers it the other way.
//
// Measured 2026-09-15 on zsh 5.9.2, `env -i` with a scratch HOME, over a
// script file, against bash 5.3.15, ksh93u+ and dash, which agree with each
// other. The rows below are that measurement. Parenthesized, every column
// agrees, which is the control that says this is precedence and not a broken
// operator — it is in the syntax package's own suite, where it belongs to the
// flag rather than to this preset.
func TestThisPresetBindsTheShiftsAndTheBitwiseOperatorsTighter(t *testing.T) {
	if got, want := zsh.Dialect().ArithPrecedence,
		syntax.ArithPrecedenceShiftsAndBitwiseBindTighter; got != want {
		t.Errorf("ArithPrecedence = %v, want %v", got, want)
	}
	for _, tc := range []struct{ src, want string }{
		{`printf '%s\n' "$(( 1 << 2 + 1 ))"`, "5\n"},
		{`printf '%s\n' "$(( 1 + 2 << 1 ))"`, "5\n"},
		{`printf '%s\n' "$(( 1 << 2 * 2 ))"`, "8\n"},
		{`printf '%s\n' "$(( 16 >> 1 + 1 ))"`, "9\n"},
		{`printf '%s\n' "$(( 1 < 2 & 1 ))"`, "0\n"},
		{`printf '%s\n' "$(( 6 | 1 + 1 ))"`, "8\n"},
		// The exponent rows, which is where `**` moves with them: it is
		// looser than the bitwise operators here.
		{`printf '%s\n' "$(( 2 ** 1 | 3 ))"`, "8\n"},
		{`printf '%s\n' "$(( 2 | 1 ** 3 ))"`, "27\n"},
		// The control, so the rows above read as precedence.
		{`printf '%s\n' "$(( 1 << (2 + 1) )) $(( (1 + 2) << 1 )) $(( (1 < 2) & 1 ))"`, "8 6 1\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// `c_precedences` is this shell's own name for the rest of the panel's order,
// which is the strongest evidence that the order above is a decision rather
// than an accident — and it is a **run-time** option, so it moves an
// expression the parser has already read.
//
// Measured 2026-09-15 on zsh 5.9.2: the option changes what `$(( 1 << 2 + 1
// ))` answers on a later line of the same `-c` string, and changes it inside
// a function whose body was read before the option was touched. Both shapes
// are rows here, because a shell that re-read only the line in front of it
// would pass the first and fail the second.
func TestCPrecedencesPutsCSOrderBack(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"on, off, and on again",
			`printf 'native %s %s\n' "$(( 1 << 2 + 1 ))" "$(( 1 < 2 & 1 ))"
setopt c_precedences
printf 'c      %s %s\n' "$(( 1 << 2 + 1 ))" "$(( 1 < 2 & 1 ))"
unsetopt c_precedences
printf 'back   %s %s\n' "$(( 1 << 2 + 1 ))" "$(( 1 < 2 & 1 ))"`,
			"native 5 0\nc      8 1\nback   5 0\n",
		},
		{
			"a function body read before the option was touched",
			`f() { printf '%s\n' "$(( 1 << 2 + 1 ))"; }
f
setopt c_precedences
f`,
			"5\n8\n",
		},
		{
			"and the listings say which order is in force",
			`[[ -o c_precedences ]]; printf 'off %s\n' "$?"
setopt c_precedences
[[ -o c_precedences ]]; printf 'on  %s\n' "$?"`,
			"off 1\non  0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}
