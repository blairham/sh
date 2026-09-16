// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `compgroups`: the `group-order` style's internals, which is a listing
// arrangement this editor has not got.
//
// The manual: it "only takes its arguments as names of completion groups and
// creates the groups for it (all six types: sorted and unsorted, both without
// removing duplicates, with removing all duplicates and with removing
// consecutive duplicates)". A group is a heading in zsh's listing and a
// sorting boundary; repl draws one listing of replacement words and has
// neither.
//
// So the groups are created and there is nowhere for them to be, which is the
// same answer `compadd`'s `-J` and `-V` get and is given for compctl.go's
// reason: a builtin that refused every call naming a group would stop
// completions that are otherwise entirely servable, and refusing here would
// stop every completion whose context sets `group-order`.
//
// It is named in #3039 as one of the eight that is *registered* rather than
// *written*, so that `zmodload -F zsh/computil b:compgroups` does not claim
// more than is here.

func compgroupsBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if _, _, ok := computilFrom(r, ctx); !ok {
		return 1
	}
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	return 0
}
