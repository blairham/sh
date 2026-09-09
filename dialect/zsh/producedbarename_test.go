// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAProducedArrayReadsAsTheWholeListHere is this dialect's half of #1600,
// and it is the opposite answer to dialect/bash's for the same fix.
//
// A produced array read without a subscript was invisible: the value empty,
// the length 0, and `${+name}` reporting a parameter this dialect registers
// as absent. What it should answer is the axis — zsh reads a bare array name
// as the whole list where bash reads its first element — so these rows say
// "both elements" where bash's say "the first".
//
// `epochtime` is the subject because it is a produced array with two elements
// that this dialect already had before any of this, so a row here cannot pass
// by accident of the parameter being new. Its *values* move every second,
// which is why every assertion counts fields rather than comparing text.
func TestAProducedArrayReadsAsTheWholeListHere(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// Two fields, not one: the bare name is the list joined, and the
			// first-element reading would leave a single field here.
			name: "the bare name is every element",
			src:  `zmodload zsh/datetime; v=$epochtime; print -r -- "${#${(z)v}}"`,
			want: "2\n",
		},
		{
			// zsh's `${#a}` on an array is the element count, and it agrees
			// with the subscripted spelling. Both answered 0.
			name: "the bare length is the element count",
			src:  `zmodload zsh/datetime; print -r -- "${#epochtime} ${#epochtime[@]}"`,
			want: "2 2\n",
		},
		{
			name: "a default is not taken",
			src:  `zmodload zsh/datetime; [[ ${epochtime:-DEFAULT} == DEFAULT ]] && print -r -- taken || print -r -- kept`,
			want: "kept\n",
		},
		{
			name: "the parameter exists",
			src:  `zmodload zsh/datetime; print -r -- "${+epochtime}"`,
			want: "1\n",
		},
		{
			// The empty producer, which is where the two dialects part: the
			// whole-list reading has a value for it — the empty join — so
			// the name is set, and bash's base-element reading has no
			// element zero and answers unset. Measured, zsh 5.9.2 answers 1.
			name: "an empty produced array still exists",
			src:  `zmodload zsh/parameter; print -r -- "${+dis_reswords} ${#dis_reswords[@]} [${dis_reswords+SET}]"`,
			want: "1 0 [SET]\n",
		},
		{
			// And the same for a produced *table*, which was wrong here for
			// a reason of its own: the empty case answered before the axis
			// was asked. `dis_functions` is one this dialect registers.
			name: "an empty produced association still exists",
			src:  `zmodload zsh/parameter; print -r -- "${+dis_functions} [${dis_functions+SET}]"`,
			want: "1 [SET]\n",
		},
		{
			// A stored empty table, which had nothing to do with producing
			// anything and was wrong in exactly the same place.
			name: "an empty declared association still exists",
			src:  `typeset -A h; print -r -- "${+h} [${h+SET}] [$h]"`,
			want: "1 [SET] []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
