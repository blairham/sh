// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An element that is nothing but separators splits away to no field and
// leaves its boundary behind — the core rule, in interp/splitawayedge.go,
// which every splitting column agrees on. What is this preset's is that
// `shwordsplit` is the option that puts it in reach at all: with the option
// off, ` ` is a literal and the element is an ordinary field, which is why
// this shell's default answers every row here the other way.
//
// So the rows come in pairs, and the pair is the point: the option on is the
// boundary, the option off is the blank kept as text. A rule that removed the
// blank rather than recording where it was would answer the second row of
// each pair the same as the first.
//
// Measured 2026-09-26 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), run
// `-f`. The field count is asserted beside the fields: a boundary that was
// lost reads back as the same characters.
func TestShwordsplitMakesABlankElementABoundary(t *testing.T) {
	const probe = `w(){ printf '%d' $#; printf '[%s]' "$@"; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		// The option on: the blank element is separators, so it is a
		// boundary and nothing else.
		{"at the front", `setopt shwordsplit; b=(' ' 2); w x${b}y`, `2[x][2y]`},
		{"at the end", `setopt shwordsplit; b=(2 ' '); w x${b}y`, `2[x2][y]`},
		{"the only element", `setopt shwordsplit; b=(' '); w x${b}y`, `2[x][y]`},
		{"one at each end", `setopt shwordsplit; b=(' ' 2 ' '); w x${b}y`, `3[x][2][y]`},
		{"the positional parameters", `setopt shwordsplit; set -- ' ' 2; w x$@y`, `2[x][2y]`},

		// The option off, which is this preset's default and the other half
		// of each pair: the same word, the blank kept as text.
		{"off, at the front", `b=(' ' 2); w x${b}y`, `2[x ][2y]`},
		{"off, at the end", `b=(2 ' '); w x${b}y`, `2[x2][ y]`},
		{"off, the only element", `b=(' '); w x${b}y`, `1[x y]`},

		// A flag group whose own split makes the separators, which is a
		// route of its own and already recorded the edge: `(s)` splitting a
		// scalar reaches the scalar path, where the boundary has been kept
		// since #3373.
		{"a split flag's leading blanks", `v='  b'; w x${(s. .)v}y`, `2[x][by]`},
		{"a split flag's trailing blanks", `v='b  '; w x${(s. .)v}y`, `2[xb][y]`},

		// The controls. Quoted, the blanks are text in every case; and the
		// distribution is a copy of the word per element rather than a list
		// of fields, so neither row can move whatever the boundary does.
		{"quoted", `setopt shwordsplit; b=(' ' 2); w "x${b}y"`, `1[x  2y]`},
		{"the distribution", `b=(' ' 2); w x${^b}y`, `2[x y][x2y]`},
		{"the option, one blank element", `b=(' '); w x${^b}y`, `1[x y]`},
		{"rcexpandparam", `setopt rcexpandparam; b=(' ' 2); w x${b}y`, `2[x y][x2y]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), probe+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
