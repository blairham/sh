// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `argv` is this shell's name for the positional parameters.
//
// **Every assertion that matters here is a read, a mutation and a read
// again**, because a name filled in once and never updated passes any test
// that sets the parameters and then looks: the snapshot was taken after the
// only mutation. The rows below move `$@` and `argv` in both directions and
// read the *other* one each time, which no snapshot at any instant satisfies.
//
// Measured against zsh 5.9.2, 2026-09-10; the corpus row is
// `param/argv-is-the-positional-parameters`.
func TestArgvIsThePositionalParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"reading it reads the parameters",
			`set -- 'a b' c; printf "[%s]" "$argv" "${#argv}" "${argv[1]}" "${argv[2]}"`,
			"[a b c][2][a b][c]",
		},
		{
			"the bare name joins in quotes and keeps its fields without them",
			`set -- 'a b' c; printf "[%s]" "$argv"; printf "[%s]" $argv`,
			"[a b c][a b][c]",
		},
		{
			"a whole-array subscript keeps them in quotes as well",
			`set -- 'a b' c; printf "[%s]" "${argv[@]}"; printf "[%s]" "${argv[*]}"`,
			"[a b][c][a b c]",
		},
		{
			"moving the parameters moves it",
			`set -- x y; printf "[%s]" "$argv"; set -- z; printf "[%s]" "$argv" "${#argv}"`,
			"[x y][z][1]",
		},
		{
			"writing one element writes that parameter",
			`set -- 'a b' c; argv[1]=zz; printf "[%s]" "$1" "$2" "$#"`,
			"[zz][c][2]",
		},
		{
			"writing it whole replaces the parameters",
			`set -- a b; argv=(n1 n2 n3); printf "[%s]" "$1" "$2" "$3" "$#"`,
			"[n1][n2][n3][3]",
		},
		{
			"appending to it appends to them",
			`set -- a b c; argv+=(d); printf "[%s]" "$#" "$4"`,
			"[4][d]",
		},
		{
			"writing past the end extends them",
			`set -- a b; argv[5]=e; printf "[%s]" "$#" "$5"`,
			"[5][e]",
		},
		{
			"unsetting it leaves none",
			`set -- a b c; unset argv; printf "[%s]" "$#" "$argv" "$1"`,
			"[0][][]",
		},
		{
			"inside a function it is that frame's parameters",
			`set -- outer; f() { printf "[%s]" "${argv[1]}" "$#"; }; f q w; printf "[%s]" "$1" "$#"`,
			"[q][2][outer][1]",
		},
		{
			"and writing it inside a function does not reach the caller's",
			`set -- outer; g() { argv=(p q); printf "[%s]" "$1" "$#"; }; g z; printf "[%s]" "$1" "$#"`,
			"[p][2][outer][1]",
		},
		{
			"a stray global is not what a later read finds",
			`set -- 'a b' c; argv[1]=zz; f() { printf "[%s]" "${argv[1]}"; }; f q`,
			"[q]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
