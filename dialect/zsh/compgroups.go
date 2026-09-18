// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `compgroups`: the `group-order` style's internals, which is the order the
// blocks of a listing are drawn in.
//
// The manual: it "only takes its arguments as names of completion groups and
// creates the groups for it (all six types: sorted and unsorted, both without
// removing duplicates, with removing all duplicates and with removing
// consecutive duplicates)". Creating a group ahead of the `compadd` that
// fills it is how the style works: the order the names are given in is the
// order the blocks come out in, whatever order they were added in.
//
// This was a no-op until #3232, and honestly so — repl drew one listing of
// replacement words, so there were no blocks to order. There are now, and the
// declaration is kept and read at the end of the completion.
//
// Measured on zsh 5.9.2, 2026-09-18 through a pseudo-terminal, with a widget
// of my own so that what is measured is this builtin:
//
//	compgroups second first
//	compadd -J first  -X FIRST  -- alpha
//	compadd -J second -X SECOND -- beta
//
// draws `SECOND` over `beta` and then `FIRST` over `alpha`. So the six types
// the manual lists are one thing as far as a listing is concerned — a name,
// and where it sits — and duplicate removal is `compadd`'s, which already
// drops a word it has been given twice.
//
// A name this completion never adds to costs nothing: a block with no
// candidates in it is not drawn, which is the same answer zsh gives.

func compgroupsBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if !compArity(r, args, 1, -1) {
		return 1
	}
	cs, _, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	// Appended rather than replaced: a second call names more blocks, and the
	// first call's names keep their places in front of them.
	cs.groupOrder = append(cs.groupOrder, args...)
	return 0
}
