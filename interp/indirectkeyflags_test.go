// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"
)

// The `(P)` flag moves the lookup one step along, and `k` and `v` are the two
// letters a *lookup* answers — so they belong to the one `(P)` moved to and
// not to the one that produced the name.
//
// The group was parsed, the letters were accepted, and then the indirection
// was handed no group at all: `${(kP)n}` came back with the values, which is
// a plausible list at status 0 rather than an error. The direct `${(k)m}` was
// already right, which is what localizes this to the route rather than to the
// letters — and is why the cases below are written in pairs, the direct
// spelling beside the one through the flag.
//
// Named for the flags rather than for a shell, per the rule in AGENTS.md; the
// same pairs are asserted as one shell's answer in dialect/zsh. See #1608.
func TestTheIndirectionFlagCarriesKeyAndValueToTheLookupItMovesTo(t *testing.T) {
	// tab is the table the indirection lands on, m an association whose one
	// value names it, and `a` — the *key* of that one pair — a live scalar.
	// So reading `k` at the wrong lookup does not come back empty: it comes
	// back with `A_val`, a word, which is the shape a caller cannot see.
	const setup = `typeset -A tab; tab=(k1 v1 k2 v2)
typeset -A m; m=(a tab)
a=A_val
n=tab
`
	tests := []struct{ name, src, want string }{
		{
			"k through the indirection is the keys of the table it names",
			`printf "[%s]" "${(kP)n}"`, "[k1 k2]",
		},
		{
			"kv through the indirection interleaves that table's pairs",
			`printf "[%s]" "${(kvP)n}"`, "[k1 v1 k2 v2]",
		},
		{
			"and the fields survive the group that keeps them",
			`set -- "${(@kvP)n}"; printf "[%s]" "$#" "$1" "$2" "$3" "$4"`,
			"[4][k1][v1][k2][v2]",
		},
		{
			"the letters may be written on either side of the flag",
			`printf "[%s]" "${(Pk)n}"`, "[k1 k2]",
		},
		{
			"v through the indirection is the values, as it was",
			`printf "[%s]" "${(vP)n}"`, "[v1 v2]",
		},
		{
			"no letter at all is still the values",
			`printf "[%s]" "${(P)n}"`, "[v1 v2]",
		},
		{
			"the letters reach the second lookup and not the first",
			`printf "[%s]" "${(kP)m}"`, "[k1 k2]",
		},
		{
			"and the first lookup answers the value however they are written",
			`printf "[%s]" "${(kvP)m}"`, "[k1 v1 k2 v2]",
		},
		{
			"a letter the second lookup has no table for changes nothing",
			`arr=(x y z); na=arr; printf "[%s]" "${(kP)na}"`, "[x y z]",
		},
		{
			"nor does one over a scalar",
			`s=plain; ns=s; printf "[%s]" "${(kP)ns}"`, "[plain]",
		},
		{
			"a name that is not set is empty under the letters too",
			`nx=nosuchparam; printf "[%s]" "${(kP)nx}"`, "[]",
		},
		{
			"and the colon-less default still fires behind them",
			`nx=nosuchparam; printf "[%s]" "${(kP)nx-D}"`, "[D]",
		},
		{
			"a later letter acts on what the second lookup answered",
			`printf "[%s]" "${(kPU)n}"`, "[K1 K2]",
		},
		{
			"and the join separator joins the pairs it produced",
			`printf "[%s]" "${(kvPj:-:)n}"`, "[k1-v1-k2-v2]",
		},
		{
			"and the length counts the keys the letters chose",
			`printf "[%s]" "${(kP)#n}"`, "[2]",
		},

		// The controls: applied directly to the table these were already
		// right, so a case that fails only here is a case about the route.
		{
			"direct k is unchanged",
			`printf "[%s]" "${(k)tab}"`, "[k1 k2]",
		},
		{
			"direct kv is unchanged",
			`printf "[%s]" "${(kv)tab}"`, "[k1 v1 k2 v2]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, setup+tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean",
					tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The same rule where the base is a *subscript* rather than a bare name.
//
// Three lookups read `k` and `v` — a name, an association's subscript, and a
// search over one — and a fix at one of them leaves the other two answering
// the key of the wrong table. Each row here is a base that reaches a
// different one of the three, and each is paired with the direct spelling
// that was already right.
//
// On the subscript grammar rather than flagsRun's, because a search is a flag
// group inside the brackets and a plain group in front does not enable it.
func TestTheIndirectionFlagCarriesTheLettersOverASubscriptedBase(t *testing.T) {
	const setup = `typeset -A tab=(k1 v1 k2 v2)
typeset -A m=(a tab)
a=A_val
`
	tests := []struct{ name, src, want string }{
		{
			"a subscripted base names the table, and the letters land on it",
			`${(kP)m[a]}`, "[k1 k2]",
		},
		{
			"the same base with both letters",
			`${(kvP)m[a]}`, "[k1 v1 k2 v2]",
		},
		{
			"a search names it too",
			`${(kP)m[(r)tab]}`, "[k1 k2]",
		},
		{
			"and the search with both letters",
			`${(kvP)m[(r)tab]}`, "[k1 v1 k2 v2]",
		},
		{
			"without the letters the value is the name either way",
			`${(P)m[a]}`, "[v1 v2]",
		},
		{
			"and so is the search's",
			`${(P)m[(r)tab]}`, "[v1 v2]",
		},

		// The controls. `k` on a subscript substitutes the key and `k` on a
		// search substitutes the matched key, and both were right before —
		// so a row failing only above is a row about the indirection.
		{
			"a direct subscript still substitutes its key",
			`${(k)m[a]}`, "[a]",
		},
		{
			"a direct search still substitutes the key it matched",
			`${(k)m[(r)tab]}`, "[a]",
		},
		{
			"and a direct value is still the value",
			`${(v)m[a]}`, "[tab]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runAssoc(t, setup+`printf "[%s]" "`+tc.src+`"`)
			if out != tc.want || status != 0 {
				t.Errorf("%q: out=%q status=%d, want %q at 0",
					tc.src, out, status, tc.want)
			}
		})
	}
}
