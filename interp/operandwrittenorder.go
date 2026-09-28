// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// Where a declaration's **array-literal operand** stood when it was written.
//
// `typeset a=(x y) b` is one command carrying one assignment, and the parser
// hands the two halves over separately: the word `b` is in `c.Args` and the
// literal is in `c.Assigns`. This engine then rebuilds the utility's argument
// list by walking the words and **appending** each operand's bare name at the
// end, which is a different list from the one that was written — `typeset b a`
// rather than `typeset a b`.
//
// The lost order is visible in **places that look unrelated**, and naming them
// all is the point of this file, because a fix keyed on any one of them alone
// leaves the others wrong:
//
//   - `set -x`. The trace writes argv, so it wrote the permutation.
//     `typeset a=(x y) b` traced as `typeset b a=( x y )`, `typeset -a q=(1)
//     r=(2) s` as `typeset -a s q=( 1 ) r=( 2 )`, and `export v=(1 2) w` as
//     `export w v=( 1 2 )` — measured 2026-09-28 on zsh 5.9.2, which writes
//     each of them as written.
//   - `typeset -T`, whose operands are **positional**: a scalar, an array and
//     at most a separator. `typeset -T A a=(x y) +` reached Runner.declareTie
//     as `A`, `+`, `a`, so the separator arrived in the array half's position
//     and was refused as a name — `not valid in this context: +` where the
//     reference ties the pair with `+` for a separator and leaves `$A` as
//     `x+y` (#5096).
//
// A **third** surface turned up as soon as the first two were fixed, and it is
// the reason to say all this out loud rather than patch the tie: the *printer*
// made the same permutation, writing `let a=(5+3)x y` back as `let x y
// a=(5+3)`. Nothing caught it, because the runtime was permuting the same list
// the same way — two wrongs that agreed with each other, and a round-trip
// property that could not tell. See syntax/print.go's `simple`, which now
// interleaves by the same key. Its neighbor `compoundVariableItem` is left
// alone deliberately: a compound body's items cannot hold a bare word at all
// (`typeset -C c=( x=1 w y=2 )` is `"w" unexpected`, measured), so there is
// nothing there to order against.
//
// So the thing to key on is **argv itself**, and not either surface: put the
// operand's name back at the index it was written at, and the trace and the
// positional reader are both handed the list the script wrote. Everything that
// reads argv by position gets the same answer, including readers that do not
// exist yet.
//
// The index is computed from source positions rather than counted, because a
// word can contribute more than one element — `x='a b'; typeset q=(1) $x`
// splits — so the number of words before an operand is not the number of argv
// entries before it. argvBefore[i] is how long argv was when word i was
// reached, which is that count exactly, and `end` is the length when the words
// ran out.
func writtenOperandIndex(args []*syntax.Word, argvBefore []int, end int, a *syntax.Assign) int {
	at := a.Pos()
	for i, w := range args {
		if i >= len(argvBefore) {
			// A word the loop never reached, because an expansion failed and
			// the command was abandoned. There is nothing after it to be
			// ordered against.
			break
		}
		if w.Pos().After(at) {
			return argvBefore[i]
		}
	}
	return end
}
