// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Brace expansion is one pass over the word: text a group *produced* is not
// scanned for braces again, even where it now spells one.
//
// `{{1,2,3}..4}` is the worked example. The outer group is not a list and not
// a range, so the scan enters it and finds `{1,2,3}`; substituting each
// alternative leaves `{1..4}`, `{2..4}` and `{3..4}`, whose braces were never
// written in the file — the `{1` came from one place and the `..4}` from
// another. A second pass reads them as ranges and answers `1 2 3 4 2 3 4 3 4`.
//
// Measured 2026-09-22, and it is not an axis: bash 5.3.20 and zsh 5.9.2 both
// answer with the unexpanded ranges, and so does every column that expands
// braces at all.
func TestBraceExpansionDoesNotRescanWhatItProduced(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a group in a range's low end", `echo {{1,2,3}..4}`, "{1..4} {2..4} {3..4}"},
		{"a group in its high end", `echo {6..{7,8,9}}`, "{6..7} {6..8} {6..9}"},
		{"a group at both ends", `echo {{1,2}..{7,8}}`, "{1..7} {1..8} {2..7} {2..8}"},

		// The rows that say the first pass still reaches everything the file
		// wrote. A group nested inside an alternative is a list, a group
		// behind the closing brace is another factor of the product, and the
		// two together are still one pass.
		{"a group inside an alternative", `echo {a,{b,c}}`, "a b c"},
		{"a group behind the closing brace", `echo {a,b}{c,d}`, "ac ad bc bd"},
		{"and both at once", `echo {a,{b,c}}{d,e}`, "ad ae bd be cd ce"},
		{"a range still counts", `echo {1..3}`, "1 2 3"},
		{"and a range beside a list", `echo {1..2}{x,y}`, "1x 1y 2x 2y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The scan entering a group that did not expand is an axis of
			// its own, and it is bash's and zsh's answer that puts the
			// first three rows in reach at all — a core vector refuses by
			// name rather than guessing.
			out, st := run(t, tc.src+"\n", func(r *Runner) {
				s := *r.Semantics
				s.BraceRescanEntersFailedGroup = Yes
				r.Semantics = &s
			})
			if st != 0 {
				t.Fatalf("status = %d, want 0; out = %q", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
