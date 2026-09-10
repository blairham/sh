// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// `b` and `q` are one family in the group's grammar: a group may carry one
// member of it, once. Everything below is measured on zsh 5.9.2, 2026-09-10,
// and the position reported is always the offending character's own — the
// *second* member's, wherever in the group it stands and whatever else is
// written between the two.
func TestThePatternQuoteFlagIsRefusedBesideAnotherQuotingFlag(t *testing.T) {
	for _, tc := range []struct {
		src string
		pos int
	}{
		{`echo ${(bq)v}`, 5},     // a `q` behind a `b`
		{`echo ${(qb)v}`, 5},     // a `b` behind a `q`
		{`echo ${(bb)v}`, 5},     // a second `b`
		{`echo ${(bUq)v}`, 6},    // adjacency has nothing to do with it
		{`echo ${(qUb)v}`, 6},    //
		{`echo ${(bUb)v}`, 6},    //
		{`echo ${(q-b)v}`, 6},    // a modifier does not spend the `q`
		{`echo ${(q+b)v}`, 6},    //
		{`echo ${(qqqb)v}`, 7},   // nor does a repeat
		{`echo ${(bqq)v}`, 5},    // and the first `q` behind a `b` is the report
		{`echo ${(j:x:bq)a}`, 9}, // the count is of source characters,
		{`echo ${(bj:x:q)a}`, 9}, // flag arguments included
		{`echo ${(bs:x:b)v}`, 9}, //
		{`echo ${(bQb)v}`, 6},    // `Q` is not in the family and does not
		{`echo ${(qQb)v}`, 6},    // stand between the two
		{`echo ${(bQq)v}`, 6},    //
		{`echo ${(b+)v}`, 5},     // a `+` behind a `b` is no modifier at all
	} {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if e.FlagsErrPos != tc.pos {
			t.Errorf("%q: FlagsErrPos = %d, want %d", tc.src, e.FlagsErrPos, tc.pos)
		}
	}
}

// And the groups next door that the rule must not catch.
func TestThePatternQuoteFlagIsCarriedBesideEverythingElse(t *testing.T) {
	for _, tc := range []struct{ src, flags string }{
		{`echo ${(b)v}`, "b"},
		{`echo ${(@b)a}`, "@b"},
		{`echo ${(bU)v}`, "bU"},
		{`echo ${(bQ)v}`, "bQ"},
		{`echo ${(Qb)v}`, "Qb"},
		// A `-` behind a `b` is the signed-numeric sort flag, exactly as it
		// is anywhere a `q` did not eat it.
		{`echo ${(b-)v}`, "b-"},
		{`echo ${(-b)v}`, "-b"},
		{`echo ${(bj:|:)a}`, "bj"},
		{`echo ${(bl:8:)v}`, "bl"},
		{`echo ${(bZ+n+)v}`, "bZ"},
		// And a `q` group with no `b` in it is untouched by the rule.
		{`echo ${(qqqq)v}`, "qqqq"},
		{`echo ${(q)v}`, "q"},
	} {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if e.FlagsErrPos != 0 {
			t.Errorf("%q: FlagsErrPos = %d, want a clean read", tc.src, e.FlagsErrPos)
		}
		if e.Flags != tc.flags {
			t.Errorf("%q: Flags = %q, want %q", tc.src, e.Flags, tc.flags)
		}
	}
}
