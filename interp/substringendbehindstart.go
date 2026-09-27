// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"

	"github.com/blairham/sh/syntax"
)

// substringEndBehindTheStart answers a negative length whose end falls behind
// the offset, and reports whether the caller should hand back everything from
// the offset instead of an empty slice.
//
// One function for the string spelling and the list spelling alike, because
// the shell that refuses says the identical sentence for both — measured
// 2026-09-27 on zsh 5.9.2, `v=abcdef; ${v:1:-9}` and `a=(p q r s t u);
// ${a[@]:1:-9}` are each `substring expression: -3 < 1` and each ends the
// shell. A copy per spelling is where the two would come to disagree about
// which numbers are named.
//
// The numbers are the *computed* ones and the text is the one that was
// **written**, because the panel blames both: bash names `-9` and zsh names
// `-3 < 1`. See Diagnostics.SubstringEndBehindTheStart, whose verbs are
// indexed so each column takes what it names.
//
// How far the complaint unwinds is not decided here. r.expandErr and
// r.badRange are the same door the neighboring list-slice refusal goes
// through, which is what gives bash a line it gives up at 1 and zsh a shell
// that ends — see Semantics.FailedExpansionAbandonsTheLine and
// Runner.badRange.
func (r *Runner) substringEndBehindTheStart(lenWord *syntax.Word, end, start int) (rest bool) {
	switch r.sem().SubstringEndBehindTheStart {
	case SubstringEndBehindStartIsTheRest:
		return true
	case SubstringEndBehindStartIsRefused:
		r.diagf("%s\n", Wording(r.diag().SubstringEndBehindTheStart,
			"%[1]s: substring expression < 0",
			syntax.PrintWord(lenWord), strconv.Itoa(end), strconv.Itoa(start)))
		r.expandErr = true
		r.badRange = true
		return false
	default:
		r.diagf("%s\n", r.unanswered(
			"a substring length whose end falls behind the offset"))
		r.status, r.unspecified = 2, true
		return false
	}
}
