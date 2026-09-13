// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `${!prefix@}` and `${!prefix*}` list the names *extending* the prefix here.
//
// The name that matches the prefix exactly is set, readable, and left out.
// Measured 2026-09-13 against ksh93u+ 2012-08-01 with `ab=1; abc=2; abd=3`:
// both spellings answer `abc` and `abd`, where bash 5.3.15 answers with all
// three (#2619).
func TestAPrefixListingLeavesOutTheNameThatIsThePrefix(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`ab=1; abc=2; abd=3; printf "[%s]" "${!ab@}"`, `[abc][abd]`},
		{`ab=1; abc=2; abd=3; printf "[%s]" "${!ab*}"`, `[abc abd]`},
		// Unquoted, which is the same list through the splitting rather
		// than a second rule.
		{`ab=1; abc=2; abd=3; printf "[%s]" ${!ab@}`, `[abc][abd]`},
		// The exact name is the *only* one that matches, so the answer is
		// nothing at all rather than the name — which is what says the
		// filter is on the name and not on how many matched.
		{`ab=1; printf "[%s]" "${!ab@}"`, `[]`},
		// And a prefix nothing extends is still empty rather than a
		// refusal.
		{`printf "[%s]" "${!zqnosuch@}"`, `[]`},
		// A name that only *begins* with the prefix is listed, so the rule
		// is about equality and not about the prefix having to end at a
		// separator.
		{`ab=1; abX=2; printf "[%s]" "${!ab@}"`, `[abX]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The axis and not the code: a preset that stopped holding it would answer
// bash's way with every test above still passing against a hand-written
// filter somewhere else.
func TestThePrefixListingIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().NamePrefixListingExcludesTheExactName; got != interp.Yes {
		t.Errorf("NamePrefixListingExcludesTheExactName = %v, want yes", got)
	}
}
