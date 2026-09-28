// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// How many matches it takes before the editor asks rather than printing, in
// the dialect that keeps the number in a parameter.
//
// The question itself was here and the threshold was a constant hundred, so
// `LISTMAX=5` was a number a script could set and nothing read — which is what
// #4993 was filed as, and the filing was wrong about which half was missing:
// it said nothing asked at all.
//
// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 —
// `zsh -f -i` on a terminal of eighty columns, completing `ls alpha<TAB>` in a
// directory of five files that list in one row, and `ls beta<TAB>` in one of
// three hundred that list in thirty-eight. Eight rows, and every one of them
// re-run against this shell's own binary through the same driver.
//
// **The three readings are why this is a table and not a comparison.** A
// positive number is a count and a negative one is not a count at all, so a
// single `>=` gets every negative row wrong; and zero is neither, so the two
// obvious readings — "never ask" and "always ask" — each get one of its two
// rows wrong.
func TestTheListQueryThreshold(t *testing.T) {
	// One row and thirty-eight rows of listing, on a twenty-four row screen:
	// the two sizes the pty grid used, so the rows below are that grid.
	small := make([]Candidate, 5)
	for i := range small {
		small[i] = Candidate{Word: "alpha"}
	}
	large := make([]Candidate, 300)
	for i := range large {
		large[i] = Candidate{Word: "beta000"}
	}
	for _, tc := range []struct {
		name      string
		threshold int
		set       bool
		matches   []Candidate
		want      bool
	}{
		// A positive number is a count, and the comparison is **at** it
		// rather than past it: five matches and `LISTMAX=5` asks.
		{"below the count", 2, true, small, true},
		{"exactly the count", 5, true, small, true},
		{"and one past it does not", 6, true, small, false},
		{"a large listing under a large threshold", 400, true, large, false},
		// Zero is not a count at all: it asks about a listing that will not
		// fit the screen and nothing else. Both rows are needed — "never
		// ask" gets the second wrong and "always ask" the first.
		{"zero is silent about a listing that fits", 0, true, small, false},
		{"and asks about one that does not", 0, true, large, true},
		// And every negative asks whatever the size, which is what rules out
		// the reading where a negative is a number of rows to compare
		// against: `-10` asks about a one-row listing.
		{"a negative asks about a small listing", -1, true, small, true},
		{"and a large negative still asks", -10, true, small, true},
		// The control on the other side: with no parameter at all — which is
		// every dialect but one — the built-in hundred stands, and that is
		// the behavior this change must not have moved.
		{"with no parameter the built-in count stands", 0, false, small, false},
		{"and it is reached at a hundred", 0, false, make([]Candidate, 100), true},
		{"and not at ninety-nine", 0, false, make([]Candidate, 99), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &editor{
				width:  func() int { return 80 },
				height: func() int { return 24 },
			}
			if tc.set {
				e.listThreshold = func() (int, bool) { return tc.threshold, true }
			}
			if got := e.listQueryAsks(tc.matches); got != tc.want {
				t.Errorf("listQueryAsks(%d matches) with threshold %d set=%v = %v, want %v",
					len(tc.matches), tc.threshold, tc.set, got, tc.want)
			}
		})
	}
}

// A parameter holding something that is not a number is the built-in count.
//
// Its own row because it is the one state the pty grid cannot produce
// cleanly — the shell's own `LISTMAX` is an integer, so a word assigned to it
// arrives as a number — and because "unset" and "unreadable" have to answer
// the same way or a front end that failed to read it would silently stop
// asking about anything.
func TestAnUnreadableListQueryThresholdKeepsTheBuiltInCount(t *testing.T) {
	e := &editor{
		width:         func() int { return 80 },
		height:        func() int { return 24 },
		listThreshold: func() (int, bool) { return 0, false },
	}
	if got := e.listQueryAsks(make([]Candidate, 100)); !got {
		t.Errorf("a hundred matches with an unreadable threshold = %v, want the built-in count to ask", got)
	}
	if got := e.listQueryAsks(make([]Candidate, 99)); got {
		t.Errorf("ninety-nine matches with an unreadable threshold = %v, want silence", got)
	}
}
