// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a shell started in a table too crowded for its own rule answers.
//
// Every rule here offers at most sixty-four numbers from its base — the
// descent from the top of the table walks [10,63] and each climb spans
// sixty-four from where it starts — so a process already holding seventy
// descriptors is a process where **no wish can land**. That is not a
// contrived shape: it is a Runner embedded in a long-lived program, which is
// what this library is for, and it is what a CI runner handed `go test`
// (#4459, #4463).
//
// The panel was measured in exactly that state before any of this was
// changed, on 2026-09-26, with 3..72 opened on the null device before the
// shell started — so the lowest number really free is 73. References are
// `/opt/homebrew/bin/bash` 5.3.20, `/opt/homebrew/bin/zsh` 5.9.2 run `-f`,
// and `/bin/ksh` AT&T 93u+ 2012-08-01; `go version -m` says *not a Go
// executable* for each.
//
//	                              clean table        3..72 held
//	bash  echo <(true)            /dev/fd/63         /dev/fd/73
//	bash  three of them           63 62 61           73 74 75
//	zsh   three of them           11 12 13           74 75 76
//	ksh93 three of them           3 4 5              73 74 75
//	bash  cat <(echo <(true))     /dev/fd/63         /dev/fd/73
//	ksh93 the same                /dev/fd/3          /dev/fd/73
//	zsh   the same                /dev/fd/10         /dev/fd/73
//
// Two facts come out of it and they are the two cases below. **The fallback
// is right**: bash's descent, zsh's climb and ksh93's climb all collapse onto
// the lowest free number and then walk *upward* from it, which is the answer
// this shell's fallback already gives — so nothing about the search needed
// widening, which is what the issue suspected and what the measurement
// settles. And **the release is not part of that collapse**: a nested
// substitution still republishes the number the enclosing one took, in all
// three.
//
// What this shell answered against the same table was one above the lowest
// free number, every time — its own pipe's original was sitting on the best
// one while it was being duplicated — and `75 78` for the flat-and-nested
// pair where both should be 75.

// The lowest free number, in a table where the rule's own region is gone.
//
// The same claim as TestSeveralSubstitutionEndsTakeConsecutiveNumbers and
// asked the same way — against what the kernel would hand out, taken in the
// same process — with the one difference that decides it: here every wish
// misses, so what is graded is the fallback.
//
// **One substitution and not three, and that is a measurement rather than a
// smaller ambition.** A real shell **forks** for a body and the far end is
// closed in the parent, so it holds exactly one descriptor per live
// substitution and three of them land on three consecutive free numbers. A
// body here is a goroutine in one process, so this shell holds *two* — the
// published end and its own end of the same pipe — and in a clean table the
// second is hidden above everything the dialect could publish. A crowded
// table has no such region: every number above the crowd is one the next
// substitution wants. So three of them interleave with this shell's own ends
// by construction, and a row asserting otherwise would be asserting that a
// clone is a fork. The first one is the whole of what the fallback decides
// and is stable.
func TestASubstitutionEndTakesTheLowestFreeNumberInACrowdedTable(t *testing.T) {
	if measuredInACrowdedDescriptorTable(t) {
		return
	}
	const attempts = 3
	var got, want []int
	for i := range attempts {
		want = lowestFreeFds(t, 1)
		got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
			AllocateDescriptorsFromTen, nil, "echo <(true)", nil))
		if len(got) != 1 {
			t.Fatalf("got %v, want one number", got)
		}
		if sameFds(got, want) {
			return
		}
		t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
	}
	t.Errorf("got %v over %d attempts, want %v — the number that was free. Every wish "+
		"misses in this table, so the answer is the fallback, and this pipe's own "+
		"original is being counted in it", got, attempts, want)
}

// And the descent's own rule reaches the same place, which is what says the
// collapse is the fallback's and not one dialect's.
//
// bash is the column measured doing it: `/dev/fd/63` on a clean table and
// `/dev/fd/73` on a crowded one, with the three-substitution form reversing
// direction from `63 62 61` to `73 74 75` because the region it descends
// through is gone.
func TestTheDescentFallsBackToTheLowestFreeNumberInACrowdedTable(t *testing.T) {
	if measuredInACrowdedDescriptorTable(t) {
		return
	}
	const attempts = 3
	var got, want []int
	for i := range attempts {
		want = lowestFreeFds(t, 1)
		got = substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
			AllocateDescriptorsFromTen, nil, "echo <(true)", nil))
		if len(got) != 1 {
			t.Fatalf("got %v, want one number", got)
		}
		if sameFds(got, want) {
			return
		}
		t.Logf("attempt %d: got %v, wanted %v", i+1, got, want)
	}
	t.Errorf("got %v over %d attempts, want %v — a descent with nothing to descend "+
		"through answers the lowest free number", got, attempts, want)
}

// A nested substitution still takes the number the enclosing one published,
// with every wish missing.
//
// The release is not offered by the kernel and never was — it is a number a
// *fork* would have freed, which this shell has no fork to free — so it is
// offered by the wish list, and a wish list that cannot land offered it
// nowhere. The nested case then took a fresh number: `75 78` where both
// should be 75.
//
// Asked against the same script's flat answer rather than against digits, for
// the reason TestANestedSubstitutionTakesTheEnclosingNumber gives: the
// numbers belong to the whole test binary, and a fault one number wide cannot
// be seen by a region.
func TestANestedSubstitutionTakesTheEnclosingNumberInACrowdedTable(t *testing.T) {
	if measuredInACrowdedDescriptorTable(t) {
		return
	}
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		"echo <(true)\nread line < <(echo <(true))\necho \"$line\"", nil))
	if len(got) != 2 {
		t.Fatalf("got %v, want two numbers", got)
	}
	if got[0] != got[1] {
		t.Errorf("got %v, want the nested substitution on the same number as the flat "+
			"one — the release is reached only through a wish list that, in this "+
			"table, can never land", got)
	}
}
