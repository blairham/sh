// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A substitution written inside a bare subscript belongs to the subscript.
//
// The subscript of `$a[…]` ends at its own `]`, and what ends the *word* is
// the quoting's question — which is why a blank inside it gives the brackets
// back. A substitution's `(` was answering that question too, so
// `x=$a[$((n))]` gave the subscript up and came back as the whole array with
// `[3]` behind it. The quoted spelling went through a different scan and was
// right all along, which is why nothing had noticed.
//
// Measured on zsh 5.9.2, 2026-09-11, with `a=(one two three); n=3`:
//
//	x=$a[$((n))]            three      here: one two three[3]
//	x=$a[$(echo 3)]         three      here: one two three[3]
//	x=$a[$n]                three      here: three
//	"$a[$((n))]"            three      here: three
//
// powerlevel10k reads its saved prompt line out of an associative array as
// `$_p9k__prompt_char_saved[$_p9k__prompt_side$_p9k__segment_index$((!_p9k__status))]`,
// so every prompt drew the literal text `[left31]` where the prompt character
// belonged (#2048).
func TestASubscriptHoldingASubstitutionIsStillASubscript(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "an arithmetic substitution",
			src:  `a=(one two three); n=3; x=$a[$((n))]; print -r -- "[$x]"`,
			want: "[three]\n",
		},
		{
			name: "a command substitution",
			src:  `a=(one two three); x=$a[$(echo 3)]; print -r -- "[$x]"`,
			want: "[three]\n",
		},
		{
			name: "a backquoted substitution",
			src:  "a=(one two three); x=$a[`echo 3`]; print -r -- \"[$x]\"",
			want: "[three]\n",
		},
		{
			// The word still ends where it ended: the text after the
			// subscript is the same word and not a second one.
			name: "text after the subscript",
			src:  `a=(one two three); x=$a[$(echo 1)]x; print -r -- "[$x]"`,
			want: "[onex]\n",
		},
		{
			// An associative array reached with a key built out of three
			// expansions, which is the shape the prompt theme writes.
			name: "an associative key built out of expansions",
			src:  `typeset -A m; m[left31]=SAVED; s=left; i=3; n=0; x=$m[$s$i$((!n))]; print -r -- "[$x]"`,
			want: "[SAVED]\n",
		},
		{
			// A substitution suspends the word-end test and not the bracket
			// count: a `[` written inside it still has to be closed before
			// the subscript can.
			name: "a balanced bracket inside the substitution",
			src:  `a=(one two three); x=$a[$(echo 2; : [ ])]; print -r -- "[$x]"`,
			want: "[two]\n",
		},
		{
			// The control: a plain expansion in the subscript was never
			// affected, so a test written only on that shape would have
			// passed throughout.
			name: "a plain expansion in the subscript",
			src:  `a=(one two three); n=3; x=$a[$n]; print -r -- "[$x]"`,
			want: "[three]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if st != 0 {
				t.Fatalf("status = %d, out %q", st, out)
			}
			if out != tc.want {
				t.Errorf("%s\noutput = %q\n  want   %q", tc.src, out, tc.want)
			}
		})
	}
}
