// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// `$ARGC`: how many positional parameters there are, under this shell's own
// name for it.
//
// Measured 2026-09-10 against zsh 5.9.2, bash 5.3, bash 3.2, ksh93 and dash.
// Only zsh has the name — everywhere else `$ARGC` is an ordinary unset
// parameter and reads as empty — so this is a dialect's parameter and not an
// axis: there is nothing for the other four to disagree with.
//
// It is **produced rather than stored**, which is the whole of what this file
// has to get right. `$#` changes with `shift`, with `set --`, and at every
// function call and return, and a number written once would be right until the
// first of those and quietly wrong afterwards — quietly, because a stale count
// still reads as a number and still divides in arithmetic. Reading `len(Params)`
// at the moment of the expansion makes the two spellings one answer by
// construction rather than by bookkeeping.
//
// **Readonly, and that is measured rather than defensive.** zsh answers
// `ARGC=2`, `ARGC+=1` and `unset ARGC` alike with `read-only variable: ARGC`
// and status 1, where `argv=(x y)` beside it is accepted and moves `$@`. The
// mark is also what keeps the producer from being shadowed: a produced scalar
// with no writer takes an assignment into the stored table, a stored value is
// what a read finds first, and the parameter would report a frozen count from
// then on with nothing said.
//
// # Why a shell noticed
//
// #1682. Every powerlevel10k configuration ends with `p10k reload`, and that
// function's dispatch opens with
//
//	if (( !ARGC )); then
//	  print -rP -- $__p9k_p10k_usage >&2
//	  return 1
//	fi
//
// An unset parameter is zero in arithmetic, `!0` is one, so a shell without
// this name takes *every* call to `p10k` as a call with no arguments: the
// theme printed thirteen lines of help onto a real startup and the reload
// never happened, which left the prompt built from the configuration in effect
// before the config file ran. The count is read a further nine times in the
// same file — `(( ARGC > 1 ))` guards `reload` and `configure`, `(( OPTIND <=
// ARGC ))` ends `p10k segment` — so the name is load-bearing for the whole
// dispatch and not for one branch of it.
//
// `$argv`, zsh's other name for the same thing, is deliberately not here: it
// is an *array* a script may write *through* to the positional parameters —
// `argv[1]=zz` moves `$1`, and inside a function the name is that frame's
// parameters — which is a different piece of work with a different failure
// mode. It has its own report, #1633, and neither p10k's dispatch nor the
// plugin manager beside it reads the name: zero uses across both files, against
// ten reads of `$ARGC` in the dispatch alone.
func registerARGC(r *interp.Runner) {
	r.SetDynamic("ARGC", func(rr *interp.Runner) string {
		return strconv.Itoa(len(rr.Params))
	})
	r.MarkReadonly("ARGC")
	// And silent to a `typeset -p`, which is this shell's third answer for a
	// produced parameter — see interp.ProducedDeclaration.Silent, where
	// `LINENO` is the other name that has it. Measured 2026-09-12, zsh
	// 5.9.2: `typeset -p ARGC` writes nothing at all and reports 0, and the
	// name is absent from a bare `typeset -p` too, while `typeset -r` writes
	// `ARGC=0` in the same run. So the silence belongs to the `-p` form and
	// not to the name, which is why it goes here rather than on the mark.
	//
	// Without it the readonly mark above put the name into both listings by
	// the route an ordinary attributed name takes, and this shell wrote
	// `typeset -r ARGC=0` where zsh writes nothing.
	//
	// The integer half is silent's *neighbour* rather than its opposite, and
	// leaving it out cost two answers rather than none (#2552). Silence is
	// the `-p` word's, so the two forms that do write the name still need
	// the letters: measured in the same run, a bare `typeset` writes
	// `integer 10 readonly ARGC=0` where this wrote `readonly ARGC=0`, and
	// `${(t)ARGC}` is `integer-readonly-special` where this said `scalar`.
	// Both read the declaration, so a Silent that carried nothing else was a
	// declaration answering one of the three questions asked of it.
	r.SetDynamicDeclaration("ARGC", interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
}
