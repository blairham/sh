// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// The boundary between two adjacent elements of an unquoted list is itself a
// delimiter of the splitting rule here — it counts as IFS whitespace — so a
// separator written beside it joins that one delimiter instead of writing a
// field of its own. This shell and dash against bash, ksh93 and zsh; see
// Semantics.UnquotedListBoundaryIsIFSWhitespace for the whole grid and the
// readings it rules out.
//
// Measured against BusyBox 1.37.0 inside the alpine digest `internal/oracle.Panel` pins, on 2026-09-26 — in the container, never derived from dash. The field count is asserted beside the fields because the
// two readings put the same characters in a different number of words: a lost
// boundary reads back through `"$@"` as the text that went in, and the wrong
// answer is a plausible argument list at status 0.
func TestTheListBoundaryIsAnIFSDelimiter(t *testing.T) {
	const probe = `w(){ printf '%d' $#; printf '[%s]' "$@"; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		// The rows the issue was filed on.
		{"a separator element at the end", `IFS=:; set -- b ':'; w x$@y`, `2[xb][y]`},
		{"a separator element in the middle", `IFS=:; set -- b ':' c; w x$@y`, `2[xb][cy]`},
		{"two of them", `IFS=:; set -- a ':' b ':'; w x$@y`, `3[xa][b][y]`},
		{"an element beginning with one", `IFS=:; set -- b ':b'; w x$@y`, `2[xb][by]`},
		{"with nothing beside the expansion", `IFS=:; set -- b ':'; w $@`, `1[b]`},
		{"text in front only", `IFS=:; set -- 2 ':'; w x$@`, `1[x2]`},
		{"text behind only", `IFS=:; set -- a ':'; w $@y`, `2[a][y]`},

		// **The rows that say the boundary is IFS whitespace and not the
		// element's separator being swallowed.** One delimiter takes at most
		// one non-whitespace separator, so where two of them stand on either
		// side of a boundary the second starts a delimiter of its own and the
		// empty field between them stands. A rule keyed on the element —
		// "a separator at an element's edge merges into the boundary" —
		// answers both of these two fields and agrees with this shell on
		// every row above.
		{"a separator each side of a boundary", `IFS=:; set -- 'b:' ':c'; w x$@y`, `3[xb][][cy]`},
		{"two elements that are each a separator", `IFS=:; set -- ':' ':'; w x$@y`, `3[x][][y]`},
		{"three of them", `IFS=:; set -- ':' ':' ':'; w x$@y`, `4[x][][][y]`},
		{"two separators in one element", `IFS=:; set -- b '::'; w x$@y`, `3[xb][][y]`},

		// And the rows that say it is a delimiter at all. The same
		// characters reach the word; what moves is whether the middle one is
		// a boundary or a separator somebody wrote.
		{"a boundary on its own", `IFS=:; set -- a b; w x$@y`, `2[xa][by]`},
		{"the same characters, one element", `IFS=:; set -- ab; w x$@y`, `1[xaby]`},
		{"a boundary beside a separator", `IFS=:; set -- b ':'; w x$@y`, `2[xb][y]`},
		{"the same characters, no boundary", `IFS=:; set -- 'b::'; w x$@y`, `3[xb][][y]`},

		// An empty element beside a boundary, where the opening run decides:
		// a separator inside it has already written the empty field that
		// joins the text in front, and recording an edge as well is that
		// field twice.
		{"an empty element then a separator element", `IFS=:; set -- "" ':' b; w x$@y`, `2[x][by]`},
		{"an empty element then a word ending in one", `IFS=:; set -- "" 'b:'; w x$@y`, `3[x][b][y]`},
		{"a separator element at the very end", `IFS=:; set -- b ':' ""; w x$@y`, `2[xb][y]`},

		// An IFS holding both kinds of separator.
		{"both kinds in IFS", `IFS=' :'; set -- b ':'; w x$@y`, `2[xb][y]`},
		{"both kinds, a blank element", `IFS=' :'; set -- b ' '; w x$@y`, `2[xb][y]`},

		// The star spelling, which is the same path.
		{"the star spelling", `IFS=:; set -- b ':'; w x$*y`, `2[xb][y]`},

		// **The controls.** Under a whitespace IFS the two readings coincide,
		// which is why none of this is reachable by a script that leaves IFS
		// alone; an IFS set to nothing splits nothing; quoting keeps one
		// field per element. None of these can move whatever the boundary
		// does.
		{"a blank element, whitespace IFS", `set -- b ' '; w x$@y`, `2[xb][y]`},
		{"a blank element in the middle", `set -- b ' ' c; w x$@y`, `2[xb][cy]`},
		{"two blank elements", `set -- a ' ' b ' '; w x$@y`, `3[xa][b][y]`},
		{"an IFS set to nothing", `IFS=; set -- b ':'; w x$@y`, `2[xb][:y]`},
		{"quoted", `IFS=:; set -- b ':'; w "x$@y"`, `2[xb][:y]`},
		{"the quoted star", `IFS=:; set -- b ':'; w "x$*y"`, `1[xb::y]`},
		// A null the *splitter* wrote is a field and is kept, where the
		// boundary is a boundary and never a field.
		{"a leading separator writes a field", `IFS=:; set -- ':b' c; w $@`, `3[][b][c]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, probe+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
