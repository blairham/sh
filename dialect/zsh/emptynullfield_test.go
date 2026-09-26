// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An empty field an unquoted expansion produced is removed only where nothing
// in the word joined text to it — the core rule, in interp/emptynullfield.go,
// which every shell in the panel agrees on. What is this preset's is the two
// routes only this grammar has: a **flag group**, whose result is a list
// whether or not an array was named, and the distribution `RC_EXPAND_PARAM`
// and `${^spec}` ask for.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), run
// `-f`. The field count is asserted beside the fields: a boundary that was
// lost reads back as the same characters.
func TestAFlagGroupsEmptyFieldKeepsItsBoundary(t *testing.T) {
	const probe = `w(){ printf '%d' $#; printf '[%s]' "$@"; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		// A group that keeps the fields, over an array holding an empty
		// element: the same answer the unflagged spelling gives.
		{"the at flag", `b=('' 2); w x${(@)b}y`, `2[x][2y]`},
		{"an ordering flag", `b=('' 2); w x${(o)b}y`, `2[x][2y]`},
		{"a case flag", `b=('' 2); w x${(U)b}y`, `2[x][2y]`},
		{"at the end", `b=(2 ''); w x${(o)b}y`, `2[x][2y]`},

		// And a group whose *own* split makes the empty field. `(s)` and
		// `(f)` are the two that do, and the null they leave at an edge is
		// where the text beside the expansion stops — which is the half that
		// does not follow from the array rows above, since no element was
		// empty in any of them.
		{"a split flag's leading null", `v='::b'; w x${(s.:.)v}y`, `2[x][by]`},
		{"a split flag's trailing null", `v='a::'; w x${(s.:.)v}y`, `2[xa][y]`},
		{"a split flag's interior null", `v='a::b'; w x${(s.:.)v}y`, `2[xa][by]`},
		{"the lines flag", `v=$'a\n\nb'; w x${(f)v}y`, `2[xa][by]`},

		// The controls. A group that joins comes to one word whatever the
		// elements were, and a quoted group keeps every field it has — so
		// neither row can move whatever the removal does.
		{"a join flag", `b=('' 2); w x${(j.,.)b}y`, `1[x,2y]`},
		{"quoted", `b=('' 2); w "x${(@)b}y"`, `2[x][2y]`},
		{"with nothing beside it", `b=('' 2); w ${(o)b}`, `1[2]`},

		// A context that keeps no fields joins what comes back, so there the
		// empty element is the separator rather than a word — and the flag
		// group has to answer that the way the unflagged spelling does.
		{"an assignment's value", `b=('' 2); x=${(o)b}; printf '[%s]' "$x"`, `[ 2]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), probe+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The distribution is the other route, and it does not remove the field: the
// word is produced once per element, so the copy an empty element makes is
// the word with nothing added to it — a field wherever the word had any text
// of its own, and nothing where it had none.
//
// Both spellings, because `RC_EXPAND_PARAM` is the option that gives every
// unflagged expansion what `${^spec}` already had (#4549) and the rows have
// to agree.
func TestDistributionOverAnEmptyElementKeepsTheWord(t *testing.T) {
	const probe = `w(){ printf '%d' $#; printf '[%s]' "$@"; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		{"the flag, text on both sides", `b=('' 2); w x${^b}y`, `2[xy][x2y]`},
		{"the option, text on both sides", `setopt rcexpandparam; b=('' 2); w x${b}y`, `2[xy][x2y]`},
		{"the flag, one empty element", `b=(''); w x${^b}y`, `1[xy]`},
		{"the flag, nothing but empties", `b=('' ''); w x${^b}y`, `2[xy][xy]`},
		{"the option, nothing but empties", `setopt rcexpandparam; b=('' ''); w x${b}y`, `2[xy][xy]`},
		// With no text of its own the word each empty element makes is
		// empty, and that is the field the removal is about.
		{"the flag, no text at all", `b=('' 2); w ${^b}`, `1[2]`},
		{"the option, no text at all", `setopt rcexpandparam; b=('' 2); w ${b}`, `1[2]`},
		{"the option, text behind only", `setopt rcexpandparam; b=('' 2); w ${b}y`, `2[y][2y]`},
		{"the option, text in front only", `setopt rcexpandparam; b=(1 ''); w x${b}`, `2[x1][x]`},
		// The control: quoted, the distribution does not run at all.
		{"quoted", `b=('' 2); w "x${^b}y"`, `1[x 2y]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), probe+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
