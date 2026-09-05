// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A logical line ends where the input it consumed ends, and a here-document's
// body and delimiter are part of what a command consumed.
//
// The distinction is invisible in the tree — the body arrives on the redirect
// either way — and visible only in the position, which is what a caller
// walking the *physical* lines of the input reads. Without it a command
// spanning three lines of script reports one, because the newline that
// triggers the read sits at the end of the first and the body is consumed
// behind it.
func TestALineEndsAtItsHereDocumentDelimiter(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []int
		why  string
	}{
		{
			name: "no here-document",
			src:  "echo one\necho two\n",
			want: []int{1, 2},
			why:  "each line ends on its own line, which is the position of the newline that ended it",
		},
		{
			name: "the delimiter is the last line",
			src:  "cat <<END\nbody\nEND\necho after\n",
			want: []int{3, 4},
			why:  "the command occupies three lines and the one after it is a line of its own",
		},
		{
			name: "the last delimiter closes the line",
			src:  "cat <<A <<B\na\nA\nb\nB\necho after\n",
			want: []int{5, 6},
			why:  "two bodies are read in operator order and the second one is what ends the command",
		},
		{
			name: "an empty body",
			src:  "cat <<END\nEND\necho after\n",
			want: []int{2, 3},
			why:  "a body with no lines still has a delimiter, and the delimiter is a line",
		},
		{
			name: "a second here-document that never starts",
			src:  "cat <<A <<B\na\nA\n",
			want: []int{3},
			why:  "A closed on line 3 and B found nothing after it, so the furthest the command reached is still line 3 rather than the operator's own line",
		},
		{
			name: "the input ends before the delimiter does",
			src:  "cat <<END\nbody\n",
			want: []int{2},
			why:  "the last line there was is the last line the command occupied",
		},
		{
			name: "a here-string is not a here-document",
			src:  "cat <<<word\necho after\n",
			want: []int{1, 2},
			why:  "nothing is read after the newline, so the line ends where it is written",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
			var got []int
			for {
				f, ok := p.NextLine()
				if !ok {
					break
				}
				got = append(got, f.Last.Line)
			}
			if len(got) != len(c.want) {
				t.Fatalf("ends = %v, want %v — %s", got, c.want, c.why)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("line %d ends at %d, want %d — %s", i+1, got[i], c.want[i], c.why)
				}
			}
		})
	}
}
