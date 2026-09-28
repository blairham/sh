// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The match memo is keyed on a **position** and a zero-width assertion is the
// one reading whose answer is not a function of the position alone: `\b`
// holds at the front of the piece whatever stands there, so the same
// position answers differently depending on where the piece began.
//
// Two trials of a trim reach the same position from different starts — the
// aliasing tildeHereAnchors names from the other side — and a memo that
// carried a dead end across them would answer the second from the first.
// matchPatternIn drops the memo when the base moves under a pattern carrying
// one of these escapes; this is what says so.
//
// **No surface reaches it today**, and that is why it is pinned here rather
// than left to a row. `${v%…}` is the only span-choosing operator that reads
// a `~(K)` group at all, and it tries the shortest suffix first — so every
// position it asks about is at or after its own base, and the trial that
// could poison a key always has the larger base. Lift #4978's gate for
// `%%`, which ascends, and the order reverses. The bases here ascend for
// exactly that reason: it is the shape the guard exists for, asked directly
// because nothing in the language can ask it yet.
func TestTheMemoDoesNotCarryAZeroWidthAnswerAcrossPieces(t *testing.T) {
	was := memoThreshold
	t.Cleanup(func() { memoThreshold = was })

	const subject = "aab"
	// `*\bab`: reached from base 0 the `\b` at offset 1 sits between two
	// word characters and fails, and reached from base 1 the same offset is
	// the piece's front and holds. Same pattern offset, same position, same
	// remaining length — one memo key, two answers.
	const pattern = `*\bab`

	run := func(threshold, base int) bool {
		memoThreshold = threshold
		o := patternOpts{
			bracket:      BracketLiteral,
			group:        true,
			extended:     true,
			escapes:      `-=!*?[]()|^~#<>\`,
			classEscapes: true,
		}
		o.where = &matchWhere{}
		// Ascending, and through one matchWhere, which is what a trim does
		// and is the whole of the hazard: a fresh one per trial would carry
		// nothing to carry wrongly.
		var got bool
		for b := 0; b <= base; b++ {
			got, _ = matchPatternIn(pattern, subject[b:], subject, b, o)
		}
		return got
	}

	for _, base := range []int{0, 1, 2, 3} {
		want := run(1<<30, base)
		got := run(0, base)
		if got != want {
			t.Errorf("base %d: %v with the memo recording everything, %v with it out of reach", base, got, want)
		}
	}

	// And the control, so that the agreement above is not two runs of a
	// match that never reaches the escape at all: the answer really does
	// depend on the base.
	if at0, at1 := run(1<<30, 0), run(1<<30, 1); at0 == at1 {
		t.Fatalf("base 0 and base 1 both answered %v, so this case separates nothing", at0)
	}
}
