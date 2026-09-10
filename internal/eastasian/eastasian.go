// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package eastasian answers whether a code point is East Asian Width W or F,
// which is the one thing two readers of it have to agree about.
//
// The data is here rather than beside either reader because there are two.
// The line editor counts cells to know where the cursor is, and the prompt
// language counts cells to answer `%(l.…)` — "at least n columns have been
// printed on this line" — and those two counts have to be the same or a
// prompt whose width the shell computes and a prompt the shell draws part
// company. A second copy of the ranges is how that happens quietly.
//
// What the two readers do *around* this differs, and deliberately: a control
// character occupies no cell of the editor's line and counts as one column of
// zsh's prompt length, measured. So the shared thing is the wide table and
// not the whole of a width.
package eastasian

import "sort"

// Wide reports whether the code point is East Asian Width W or F, which a
// terminal draws in two cells.
//
// Ambiguous is not among them: it is one cell in a Western locale and two in
// an East Asian one, which makes it a property of the terminal rather than of
// the character. See the generated table beside this file.
func Wide(r rune) bool {
	i := sort.Search(len(wideRanges), func(i int) bool { return r <= wideRanges[i][1] })
	return i < len(wideRanges) && r >= wideRanges[i][0]
}
