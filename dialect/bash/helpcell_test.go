// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "testing"

// A listing cell's last column belongs to the marker.
//
// Measured 2026-09-21 on bash 5.3.20 by laying the listing out at COLUMNS 74
// through 84 and reading the character positions out of the bytes, with a
// 37-character synopsis landing in the right column at each of them:
//
//	width 39   written out, nothing after it
//	width 38   written out and a `>` after it — 38 characters, none lost
//	width 37   its first 36 characters and a `>`
//	width 36   its first 35 characters and a `>`
//
// So the question the cell asks is whether the text reaches the **second to
// last** column, not whether it overflows: a synopsis exactly as long as its
// cell is marked, and so is one a single character shorter. Reading it as
// "cut to the width" spelled one synopsis out where the real shell marks it —
// one wrong line in every listing, and the last line of a suite file's two
// listings that nothing else could account for (#2298).
//
// The table is written against widths rather than against COLUMNS on purpose:
// the geometry above it — which column is how wide — is writeHelpListing's,
// and this is the one rule inside a cell.
func TestACellsLastColumnIsTheMarker(t *testing.T) {
	const text = "source [-p path] filename [arguments]" // 37 characters

	for _, tc := range []struct {
		width int
		want  string
	}{
		{39, text},
		{38, text + ">"},
		{37, "source [-p path] filename [arguments>"},
		{36, "source [-p path] filename [argument>"},
		// A cell wider than the text by more than one is the ordinary case,
		// and it is here so the rows above cannot pass for a helpCell that
		// marks everything.
		{80, text},
		// And the degenerate widths, which COLUMNS=8 really reaches: the
		// marker is most of the cell and the cell is still never wider than
		// its column. Measured at COLUMNS=8, where the left column is two
		// characters and the real shell writes `!>` for `! PIPELINE`.
		{2, "s>"},
		{1, ">"},
		{0, ">"},
	} {
		if got := helpCell(text, tc.width); got != tc.want {
			t.Errorf("helpCell(width %d) = %q, want %q", tc.width, got, tc.want)
		}
	}

	// No cell is ever wider than its column, at any width, for any synopsis
	// this shell has. That is the invariant the padding in writeHelpListing
	// depends on — a cell one character over turns into a negative repeat
	// count and a panic.
	for _, synopses := range []map[string]string{helpKeywordSynopses(), helpOtherSynopses()} {
		for name, synopsis := range synopses {
			for width := 1; width <= 60; width++ {
				if got := helpCell(synopsis, width); len(got) > width {
					t.Errorf("%s at width %d is %d characters: %q", name, width, len(got), got)
				}
			}
		}
	}
}
