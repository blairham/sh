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
func listingRows(candidates []Candidate, width int) []string {
	var out []string
	for _, block := range listingBlocks(candidates) {
		rows := block.rows()
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
		out = append(out, columns(rows, width)...)
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
func (b block) rows() []string {
	kept := make([]Candidate, 0, len(b.candidates))
	for _, c := range b.candidates {
		if c.Display == "" {
			// Nothing of its own to draw, so the word is the row — which is
			// what a completer answering with plain words leaves behind, and
			// what every listing here was before a candidate could carry one.
			c.Display = c.Word
		}
		if c.Display == "" {
			// Neither: a candidate that exists so its block does. See
			// Candidate, and the message a completion system draws over a
			// block with nothing in it.
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
