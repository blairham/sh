// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"
	"strconv"

	"github.com/blairham/sh/interp"
)

// registerHist adds `hist`, the builtin this shell's `fc` alias names.
//
// With a history to answer from it is the substrate's `fc` under this name.
// With none — every script, which keeps no history — it refuses the range it
// would have reached, and that range is measured. ksh93u+ on 2026-10-03, from
// -c with an empty history file, every row `hist: A-B: invalid range` at 1:
//
//	hist -l        1-0        hist -l 5      5-0
//	hist -l 1 2    1-1        hist -l 2 1    2-1
//	hist -l -3     1-0        hist           0-0
//	hist -s        0-0        hist 3         3-0
//	hist -s 1      1-0        hist -e true   0-0
//
// which is a shell whose next command would be number 1: a listing starts
// sixteen back from it and stops one short of it, an edit or a re-run starts
// and stops one short of it, a written first is taken as written, a written
// last is held to the next number, and a count back from the end is held to
// 1 (corpus row fc/with-no-history).
func registerHist(r *interp.Runner) {
	r.Register("hist", func(rr *interp.Runner, ctx context.Context, args []string) int {
		if len(rr.HistoryEntries()) > 0 {
			if fc, ok := rr.Builtin("fc"); ok {
				return rr.RunBuiltinAs("hist", "fc", fc, ctx, args)
			}
		}
		listing := false
		i := 0
		for ; i < len(args); i++ {
			a := args[i]
			if a == "--" {
				i++
				break
			}
			if len(a) < 2 || a[0] != '-' || isCount(a[1:]) {
				break
			}
			for j := 1; j < len(a); j++ {
				switch a[j] {
				case 'l':
					listing = true
				case 'n', 'r', 's':
				case 'e', 'N':
					if j == len(a)-1 {
						i++
					}
					j = len(a)
				default:
					// Refused the way every builtin's option is, usage
					// line and all.
					_, _, _, code := rr.BuiltinOptions("hist", []string{a}, "lnrse:N:")
					return code
				}
			}
		}
		operands := args[i:]
		const next = 1
		first, last := next-1, next-1
		if listing {
			first = max(next-16, 1)
		}
		if len(operands) > 0 {
			first = histEvent(operands[0], next)
		}
		if len(operands) > 1 {
			last = min(histEvent(operands[1], next), next)
		}
		rr.Diagnosef("hist: %d-%d: invalid range\n", first, last)
		return 1
	})
}

// histEvent reads an operand as an event number: as written, or counted back
// from the next command and held to 1. A word that is no number names no
// event, which an empty history reads as the first.
func histEvent(word string, next int) int {
	n, err := strconv.Atoi(word)
	if err != nil {
		return 1
	}
	if n < 0 {
		return max(next+n, 1)
	}
	return n
}

// isCount reports whether a word after its dash is digits, which makes it a
// count back rather than an option.
func isCount(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
