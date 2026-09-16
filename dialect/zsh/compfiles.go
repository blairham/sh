// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `compfiles`: the four things `_path_files` asks of `zsh/computil`, and the
// one of the eight that is entirely an **optimisation**.
//
// The manual is unusually clear about that: it "is used by the _path_files
// function to optimize complex recursive filename generation (globbing)", and
// its three jobs are building glob patterns that fold in the path already
// handled, testing directories for the `ignore-parents` style, and dropping
// matches when one component is exactly what is on the line.
//
// So there is a correct answer that does none of it, and this is it:
//
//   - `-p` and `-P` build no pattern and leave the caller's alone, at status
//     0. Measured on zsh 5.9.2, 2026-09-15: `compfiles -p tmp1 accex '' ' '
//     '' fake '*'` is 0 and leaves `tmp1` empty, which is the same answer.
//   - `-i` and `-r` answer 1, which is "nothing was ignored" and "nothing was
//     removed". Measured in the trace of `make ` reaching `_path_files`:
//     `compfiles -r tmp1 ''` is 1 there, and `_path_files` carries on.
//
// The cost is that a `_files` in a shipped completion does the globbing
// itself rather than having it folded, which is slower and not wrong. The
// gain is that nothing here pretends to a pruning it did not do — a
// `compfiles -r` that answered 0 without removing anything would tell
// `_path_files` its matches had been narrowed when they had not.
//
// That is the honest half of #3039 this file does not finish, and it is
// written down in the issue rather than left to be discovered.

func compfilesBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if _, _, ok := computilFrom(r, ctx); !ok {
		return 1
	}
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	switch args[0] {
	case "-p", "-P":
		return 0
	case "-i", "-r":
		return 1
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}
