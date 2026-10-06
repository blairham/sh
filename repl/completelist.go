// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"sort"
	"strings"
)

// How a listing is arranged: the blocks it is drawn in, what heads each one,
// and which of them are packed into columns.
//
// This is the half of the completion seam that is about *reading* rather than
// about inserting. A completer answers with candidates, and every candidate
// carries the block it belongs to; nothing here knows why two candidates are
// in different blocks, only that a listing that ran them together would be
// answering a question the completer had already answered.
//
// Measured against zsh 5.9.2 on 2026-09-18 through a pseudo-terminal, with a
// completion widget of my own so that what is being measured is the drawing
// and not a shipped function's choices. `compadd -J g1 -X 'first group' --
// delta alpha charlie` followed by `compadd -J g2 -X 'second group' -- zulu
// bravo` draws
//
//	first group
//	alpha    charlie  delta
//	second group
//	bravo        zulu
//
// which is four facts at once, and each is a line of code below:
//
//  1. **The blocks are drawn in the order they were first added**, not sorted
//     against each other and not merged when a later call names an earlier
//     group.
//  2. **The heading sits on a row of its own above its block.**
//  3. **Within a block the rows are sorted**, by the word rather than by what
//     is drawn: the same probe with `-d` display strings of `delta:D alpha:A
//     charlie:C` draws `alpha:A charlie:C delta:D`, so the display went where
//     its word went.
//  4. **Each block is packed on its own.** `compadd -V g2` — the unsorted
//     spelling of the same option — keeps `zulu bravo yankee` in that order,
//     so sorting is a property a block carries rather than something the
//     listing does to everything it is given.
//
// And one more, from `compadd -l`: a block asked for one row per line gets
// exactly that, columns and all skipped. That is what a row carrying a
// sentence needs, and it is why the flag is on the Group and not on the
// listing — zsh draws a described block a row each and an undescribed block
// packed, in the same listing, over the same Tab.

// listingRows is everything a listing prints, in order: each block's heading,
// then its rows, arranged the way the block asked for.
//
// A width of zero is a terminal that will not say how wide it is; see columns.
//
// **The blocks share one grid width.** Measured against zsh 5.9.2 on
// 2026-10-05 through a pseudo-terminal 40 columns wide, a block after `compadd
// -J a alpha beta gamma zeta` lays its matches out across the 28 columns the
// first block's grid spans:
//
//	alpha  beta   gamma  zeta
//	d1       d2       d3            three matches, nine columns each
//	d1     d2     d3     d4         four, seven each
//	delta         eps               two, fourteen each
//
// So every packed block's grid is as wide as the widest of them — its number
// of columns times its cell, where the number of columns is as many as fit
// and no more than it has matches — and each block widens its cells to fill
// that, never narrowing them. Ten matches of `a1` … `a10` fit eight cells of
// five across 40 columns, so their grid is the whole 40 and three matches in
// the next block are thirteen apart, although only five of those columns are
// used. A block drawn one per line takes no part in it: a 34-column `-l` row
// leaves `d1  d2  d3` as it is (#6157).
func listingRows(candidates []Candidate, width int, layout listLayout) []string {
	blocks := listingBlocks(candidates)
	drawn := make([][]string, len(blocks))
	shared := 0
	for i, block := range blocks {
		drawn[i] = block.rows()
		if block.group.OnePerLine || len(drawn[i]) == 0 {
			continue
		}
		if _, widths, ok := packedArrangement(drawn[i], width, block.arrangement(layout)); ok {
			// **A packed block spans its own columns too**, where they are
			// wider than its uniform grid would be — and then no more than
			// two short of the screen. Measured on zsh 5.9.2, 2026-10-05,
			// a packed block followed by an unpacked block of two or three
			// matches:
			//
			//	cols  uniform  packed  second block's matches at
			//	100   51       88      44
			//	100   88       100     49; of three, 32 and 64 (98 split)
			//	101   89       101     of three, 33 and 66 (99 split)
			//	40    28       26      9 and 18 (28 split)
			//
			// so the wider of the two counts, the packed span capped at the
			// screen less two. The cap is the packed span's alone: an
			// unpacked 40-column grid at 40 columns spreads the next block
			// 20 apart.
			span := 0
			for _, w := range widths {
				span += w
			}
			if width > 2 {
				span = min(span, width-2)
			}
			shared = max(shared, span)
		}
		cell, cols := uniformGrid(drawn[i], width)
		if width > 0 && cell > width {
			// A block whose widest match does not fit the screen lends the
			// grid nothing: measured on zsh 5.9.2, a 49-column match among
			// one-letter ones at 40 columns leaves the next block's `y1  y2`
			// as they are, packed or not, where at 100 columns the same
			// block spreads them 49 apart (#6203).
			continue
		}
		shared = max(shared, cell*cols)
	}
	if width > 0 {
		shared = min(shared, width)
	}
	var out []string
	for i, block := range blocks {
		rows := drawn[i]
		if len(rows) == 0 && block.group.Heading == "" {
			continue
		}
		if block.group.Heading != "" {
			out = append(out, strings.Split(block.group.Heading, "\n")...)
		}
		if block.group.OnePerLine {
			out = append(out, rows...)
			continue
		}
		out = append(out, arrange(rows, width, shared, block.arrangement(layout))...)
	}
	return out
}

// listLayout is the two options that change how a block is arranged.
type listLayout struct {
	// packed lets each column be as wide as its own longest match, where
	// that takes fewer rows. See EditorStyle.ListPackedOption.
	packed bool
	// rowsFirst fills across each row rather than down each column. See
	// EditorStyle.ListRowsFirstOption.
	rowsFirst bool
}

// uniformGrid is a block's own grid: the cell every column takes — the
// longest match plus two — and how many columns there are, which is as many
// cells as fit and no more than there are matches. One column where nothing
// fits or the width is unknown.
func uniformGrid(matches []string, width int) (cell, cols int) {
	widest := 0
	for _, m := range matches {
		widest = max(widest, displayWidth(m))
	}
	cell = widest + 2
	cols = 1
	if width > 0 {
		cols = max(1, width/cell)
	}
	return cell, max(1, min(cols, len(matches)))
}

// arrange lays one block out, its cells widened to span shared columns.
//
// Packed, the columns are each as wide as their own longest match, and that
// arrangement is used only where it takes fewer rows. Measured on zsh 5.9.2,
// 40 columns: `aaaaaaaaaaaa b c … j` draws in two rows of a 14-column column
// and four 3-column ones where unpacked it takes five rows of two; ten matches
// `a1` … `a10`, which pack into no fewer rows than they take unpacked, draw
// exactly as unpacked. A packed arrangement fits where every column's width,
// the last one's included, sums to no more than the screen: `b` … `p` and a
// 17-column `xxxxxxxxxxxxxxxxx` fill exactly 40 in two rows.
//
// Filled across the rows, the matches go `a1 a10 a2 … a7` along the first row
// of eight and `a8 a9` along the second — measured, with LIST_ROWS_FIRST set.
// Both options at once is not drawn the way zsh draws it, which packs rows
// into column widths no simpler rule reproduced; that pair is drawn across
// the rows and unpacked here.
func arrange(matches []string, width, shared int, layout listLayout) []string {
	if len(matches) == 0 {
		return nil
	}
	if rows, widths, ok := packedArrangement(matches, width, layout); ok {
		// Spread across the shared grid like an unpacked block, the room
		// left over shared out evenly among the columns (#6203).
		span := 0
		for _, w := range widths {
			span += w
		}
		if extra := (shared - span) / len(widths); extra > 0 {
			for i := range widths {
				widths[i] += extra
			}
		}
		return placeCells(matches, rows, widths, false)
	}
	cell, cols := uniformGrid(matches, width)
	nrows := (len(matches) + cols - 1) / cols
	if spread := shared / cols; spread > cell {
		cell = spread
	}
	used := cols
	if !layout.rowsFirst {
		used = (len(matches) + nrows - 1) / nrows
	}
	widths := make([]int, used)
	for i := range widths {
		widths[i] = cell
	}
	return placeCells(matches, nrows, widths, layout.rowsFirst)
}

// packedArrangement is the packed layout of a block — how many rows, and each
// column's width — where the layout asks for one and it takes fewer rows than
// the uniform grid would.
func packedArrangement(matches []string, width int, layout listLayout) (int, []int, bool) {
	if !layout.packed || layout.rowsFirst || width <= 0 || len(matches) == 0 {
		return 0, nil, false
	}
	_, cols := uniformGrid(matches, width)
	nrows := (len(matches) + cols - 1) / cols
	for r := 1; r < nrows; r++ {
		if widths, ok := packedWidths(matches, r, width); ok {
			return r, widths, true
		}
	}
	return 0, nil, false
}

// arrangement is the layout this block is drawn with: the listing's, packed
// where the block itself asked to be. See Group.Packed.
func (b block) arrangement(layout listLayout) listLayout {
	if b.group.Packed {
		layout.packed = true
	}
	return layout
}

// packedWidths is each column's width when the matches are laid down rows to a
// column, and whether that fits the screen.
func packedWidths(matches []string, rows, width int) ([]int, bool) {
	cols := (len(matches) + rows - 1) / rows
	widths := make([]int, cols)
	total := 0
	for c := range cols {
		for r := range rows {
			if i := c*rows + r; i < len(matches) {
				widths[c] = max(widths[c], displayWidth(matches[i])+2)
			}
		}
		total += widths[c]
		if total > width {
			return nil, false
		}
	}
	return widths, true
}

// placeCells writes the rows of one arrangement: nrows of them, a column per width,
// filled down the columns or across the rows.
//
// No padding after the last match on a row: trailing spaces are invisible
// until something copies them. zsh pads every column but the grid's last, and
// what reaches the screen is the same.
func placeCells(matches []string, nrows int, widths []int, rowsFirst bool) []string {
	at := func(r, c int) int {
		if rowsFirst {
			return r*len(widths) + c
		}
		return c*nrows + r
	}
	out := make([]string, 0, nrows)
	for r := range nrows {
		var b strings.Builder
		for c := range widths {
			i := at(r, c)
			if i >= len(matches) {
				break
			}
			b.WriteString(matches[i])
			if c+1 < len(widths) && at(r, c+1) < len(matches) {
				for n := displayWidth(matches[i]); n < widths[c]; n++ {
					b.WriteByte(' ')
				}
			}
		}
		out = append(out, b.String())
	}
	return out
}

// block is one group's candidates, in the order they arrived.
type block struct {
	group      Group
	candidates []Candidate
}

// rows is what this block prints under its heading: each candidate's drawn
// text, sorted unless the group asked to keep its own order, and without the
// candidates that draw nothing.
//
// Sorted by the word and not by the row, which is the measurement above and
// also the only reading that is stable: a display string is padded to the
// widest name in its group, so sorting by it would sort by the padding as
// soon as two groups were laid out differently.
//
// And sorted by **byte**, where zsh under a UTF-8 locale sorts by the
// locale's collation — deliberately, because this shell orders a pathname
// expansion by byte too, and zsh orders the two surfaces alike (#6168). The
// decision and its measurement are in interp/order.go, beside shellOrder: a
// listing collated on its own would put one kind of word in two orders.
func (b block) rows() []string {
	kept := make([]Candidate, 0, len(b.candidates))
	for _, c := range b.candidates {
		if c.Display == "" {
			// Nothing of its own to draw, so the word is the row — which is
			// what a completer answering with plain words leaves behind, and
			// what every listing here was before a candidate could carry one.
			c.Display = c.Word
		}
		if c.Display == "" && !c.Filler {
			// Neither: a candidate that exists so its block does. See
			// Candidate, and the message a completion system draws over a
			// block with nothing in it. A filler is the exception: it is
			// a cell, and leaving it out would move every cell after it.
			continue
		}
		kept = append(kept, c)
	}
	if !b.group.Unsorted {
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].Word < kept[j].Word })
	}
	out := make([]string, len(kept))
	for i, c := range kept {
		out[i] = c.Display
	}
	return out
}

// listingBlocks gathers the candidates by group, keeping the order each group was
// first seen in.
//
// By value equality of the whole Group and not by its Name, which is what
// makes the zero Group one block: a completer that says nothing about groups
// has every candidate in the same one, and that is the listing this package
// drew before there were any. Two calls that disagree about the arrangement
// are two blocks even under one name, because there is no way to draw them as
// one.
func listingBlocks(candidates []Candidate) []block {
	var out []block
	at := map[Group]int{}
	for _, c := range candidates {
		i, seen := at[c.Group]
		if !seen {
			i = len(out)
			at[c.Group] = i
			out = append(out, block{group: c.Group})
		}
		out[i].candidates = append(out[i].candidates, c)
	}
	return out
}
