// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${a:^b}` and `${a:^^b}` interleave a parameter with the array the operand
// names. Every row was run against zsh 5.9.2 before it was written down, and
// the set was then diffed against that binary.
//
// The dialect has the grammar, so the rows live here — see
// elementselect_test.go for why that is the rule. Without the flag the colon
// opens a substring and `^b` goes to arithmetic, which refuses it: that
// refusal was the last error line of an interactive start on a real
// configuration, from `FG=( ${codes:^fg} )` in Oh My Zsh's spectrum library
// (#2112).
func TestTheArrayZipOperatorsAreThisDialects(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the line this was made for",
			`codes=(1 2); fg=(A B); printf "[%s]" ${codes:^fg}`,
			"[1][A][2][B]",
		},
		{"equal lengths", `a=(1 2 3); b=(x y z); printf "[%s]" ${a:^b}`, "[1][x][2][y][3][z]"},
		// `:^` stops when the shorter operand runs out; `:^^` cycles it.
		{"a shorter left stops it", `a=(1 2); b=(x y z w); printf "[%s]" ${a:^b}`, "[1][x][2][y]"},
		{"and cycling carries on", `a=(1 2); b=(x y z w); printf "[%s]" ${a:^^b}`, "[1][x][2][y][1][z][2][w]"},
		{"a single element cycles", `a=(1); b=(x y z); printf "[%s]" ${a:^^b}`, "[1][x][1][y][1][z]"},
		// An empty array on the right has nothing to pair with, so `:^` keeps
		// nothing and `:^^` has nothing to cycle and leaves the left whole.
		{"and cycling leaves the left", `a=(1 2 3); b=(); printf "[%s]" ${a:^^b}`, "[1][2][3]"},
		// A scalar is the one-element list it already is, on either side.
		{"a scalar on the left", `s=one; b=(x y); printf "[%s]" ${s:^b}`, "[one][x]"},
		{"a scalar on the right", `a=(1 2); s=zz; printf "[%s]" ${a:^s}`, "[1][zz]"},
		// A name nothing is stored under is no second array at all, and the
		// left comes back untouched — quietly, with no complaint about the
		// name. That is measured, not derived: it is the opposite of what the
		// set operators next door do with the same absence.
		{"an unset right-hand name", `a=(1 2); printf "[%s]" ${a:^nosuch}`, "[1][2]"},
		// Flags compose with it, applying to what the zip produced.
		{"a join flag applies after", `a=(1 2); b=(x y); printf "[%s]" ${(j:-:)a:^b}`, "[1-x-2-y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// How many fields the zip produced, which `printf '[%s]'` cannot answer: it
// prints `[]` for no arguments at all, so an empty answer and a single empty
// field read identically through it. That probe reported the quoted empty
// case as already correct while this shell was producing no field where zsh
// produces one — the whole of the difference is in the count.
//
// Quoted, the **left** operand is joined to one word before the zip sees it,
// so `"${a:^b}"` on `(1 2 3)` and `(x y z)` is the two fields `1 2 3` and
// `x`. The answer stays a list: quoting picks the operand's shape, not the
// result's.
func TestAQuotedZipJoinsTheLeftAndStaysAList(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the left joins, and the answer is still two fields",
			`a=(1 2 3); b=(x y z); set -- "${a:^b}"; print -r -- $#; printf "<%s>" "$@"`,
			"2\n<1 2 3><x>",
		},
		{
			// Unquoted there is no field at all; quoted there is one, empty.
			// The same guarantee the element filter records, reached through
			// a different operator.
			"an empty right keeps no field unquoted",
			`a=(1 2 3); b=(); set -- ${a:^b}; print -r -- $#`,
			"0\n",
		},
		{
			"and one empty field quoted",
			`a=(1 2 3); b=(); set -- "${a:^b}"; print -r -- $#; printf "<%s>" "$@"`,
			"1\n<>",
		},
		{
			// Set-and-empty is not never-set. An empty array quoted is one
			// empty word and zips with the first element of the other; a name
			// that was never set is no word to zip with at all.
			"a set-but-empty left is one empty word",
			`a=(); b=(x y); set -- "${a:^b}"; print -r -- $#; printf "<%s>" "$@"`,
			"2\n<><x>",
		},
		{
			"where a name never set is none",
			`unset a; b=(x y); set -- "${a:^b}"; print -r -- $#; printf "<%s>" "$@"`,
			"1\n<>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
