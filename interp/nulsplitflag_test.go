// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// The `(0)` flag: split the result at NUL bytes. Every expectation here is an
// oracle measurement recorded in docs/spec/grammar/parameter-expansion.md;
// the tests name the grammar flag and never a shell.
//
// It is the separator split with a separator no argument can hold — measured,
// `${(@s.\0.)v}` splits on the two characters `\` and `0` — so this letter is
// the only way to ask for it, and the values below are built with `printf`,
// which is the way to put a byte in a variable that no source line can hold.

func TestNulSplitFlagFields(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{
			"a value split at its NULs",
			`v="$(printf 'a\0b\0c')"; printf "[%s]" ${(@0)v}`,
			"[a][b][c]",
		},
		{
			"the fields flag is about quoting and not about the split",
			`v="$(printf 'a\0b\0c')"; printf "[%s]" ${(0)v}`,
			"[a][b][c]",
		},
		{
			"a value with no NUL is one field",
			`v=abc; printf "[%s]" ${(@0)v}`,
			"[abc]",
		},
		{
			"an interior empty field is dropped unquoted",
			`w="$(printf 'x\0\0y')"; printf "[%s]" ${(@0)w}`,
			"[x][y]",
		},
		{
			"and kept in quotes with the fields flag",
			`w="$(printf 'x\0\0y')"; printf "[%s]" "${(@0)w}"`,
			"[x][][y]",
		},
		{
			"a trailing NUL leaves a trailing empty field",
			`t="$(printf 'p\0')"; printf "[%s]" "${(@0)t}"`,
			"[p][]",
		},
		{
			"a leading NUL leaves a leading one",
			`v="$(printf '\0q')"; printf "[%s]" "${(@0)v}"`,
			"[][q]",
		},
		{
			"a value that is one NUL is two empty fields in quotes",
			`v="$(printf '\0')"; printf "[%s]" "${(@0)v}"`,
			"[][]",
		},
		{
			"and none at all without them",
			`v="$(printf '\0')"; printf "[%s]" ${(@0)v}`,
			"[]",
		},
		{
			"an empty value is one empty field in quotes",
			`v=""; printf "[%s]" "${(@0)v}"`,
			"[]",
		},
		// The edge rule every separator split here already follows: quoted
		// and without the fields flag, the empty field at each end survives
		// and the interior ones do not.
		{
			"a quoted split keeps the field at each edge",
			`t="$(printf 'p\0')"; printf "[%s]" "${(0)t}"`,
			"[p][]",
		},
		{
			"and drops the interior one",
			`w="$(printf 'x\0\0y')"; printf "[%s]" "${(0)w}"`,
			"[x][y]",
		},
		// An array is joined before a separator split, and the fields flag
		// is what turns that join off — the rule the letter splits share.
		{
			"an array is joined before the split",
			`a=("$(printf 'a\0b')" "$(printf 'c\0d')"); printf "[%s]" ${(0)a}`,
			"[a][b c][d]",
		},
		{
			"unless the fields flag skips the join",
			`a=("$(printf 'a\0b')" "$(printf 'c\0d')"); printf "[%s]" ${(@0)a}`,
			"[a][b][c][d]",
		},
		{
			"a join separator asked for by name puts it back",
			`a=("$(printf 'a\0b')" "$(printf 'c\0d')"); printf "[%s]" ${(@j:-:0)a}`,
			"[a][b-c][d]",
		},
		// The length is asked of the value ahead of the split, and the word
		// counts against the same separator the split uses.
		{
			"a length is the value's own",
			`v="$(printf 'a\0b\0c')"; printf "[%s]" "${(0)#v}"`,
			"[5]",
		},
		{
			"a word count collapses a run of separators",
			`z="$(printf 'a\0\0b')"; printf "[%s]" "${(0w)#z}" "${(0W)#z}"`,
			"[2][3]",
		},
		// The letter written last decides, which is what makes this a third
		// member of the set rather than a special case in front of it.
		{
			"the last split letter written decides",
			`n=$'a\nb'; printf "[%s]" ${(@f0)n} "|" ${(@0f)n}`,
			"[a\nb][|][a][b]",
		},
		{
			"and the same the other way round",
			`m=$'a\nb:c'; printf "[%s]" ${(@fs.:.)m} "|" ${(@s.:.f)m}`,
			"[a\nb][c][|][a][b:c]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}
