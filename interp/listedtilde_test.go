// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which `~` a listed value has to be quoted for, and the same question asked
// of a listed key (#2298).

func listingTildeSem(tilde Answer) Semantics {
	s := testSemantics()
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	s.ListedHashIsBareAfterANonName = No
	s.ListedHashIsBareUnlessItOpensTheValue = No
	s.ListedTildeIsBareWhereItCannotExpand = tilde
	return s
}

// The axis is asked only where the two answers differ: a value with no `~`,
// one whose `~` opens it, and one with something else in it needing quotes
// are quoted under both.
func TestAListedTildeIsBareWhereItCannotExpandTheValue(t *testing.T) {
	for _, tc := range []struct{ value, bare, quoted string }{
		// A `~` that does not open the value.
		{"a~b", "a~b", "'a~b'"},
		{"b~", "b~", "'b~'"},
		{"a~b~c", "a~b~c", "'a~b~c'"},
		{"a~", "a~", "'a~'"},
		{"a,~b", "a,~b", "'a,~b'"},
		// The offsets a tilde expansion would start at, quoted either way:
		// the front of the value, and — because an assignment's value is a
		// tilde context after each of them — straight after a `:` or an `=`.
		{"~b", "'~b'", "'~b'"},
		{"~", "'~'", "'~'"},
		{"~/x", "'~/x'", "'~/x'"},
		{"a:~b", "'a:~b'", "'a:~b'"},
		{"a:~", "'a:~'", "'a:~'"},
		{":~b", "':~b'", "':~b'"},
		{"a=~b", "'a=~b'", "'a=~b'"},
		{"a:~b:~c", "'a:~b:~c'", "'a:~b:~c'"},
		// And the character in front is the whole of it: one further along
		// from the colon is bare again.
		{"a:x~b", "a:x~b", "'a:x~b'"},
		// Something else needing quotes is not this question.
		{"a~b x", "'a~b x'", "'a~b x'"},
		{"a~b*", "'a~b*'", "'a~b*'"},
		// And a value with no `~` at all asks nothing.
		{"plain", "plain", "plain"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			src := "v=" + shellSingleQuoted(tc.value) + "; typeset -p v"
			out, _ := run(t, src, withSem(listingTildeSem(Yes)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.bare {
				t.Errorf("bare: got %q, want %q", got, "declare -- v="+tc.bare)
			}
			out, _ = run(t, src, withSem(listingTildeSem(No)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.quoted {
				t.Errorf("quoted: got %q, want %q", got, "declare -- v="+tc.quoted)
			}
		})
	}
}

// listingBothPositionsSem turns both position rules on or off together, which
// is what the composition rows below need: the column that leaves a `~` alone
// where it stands is the column that leaves a `#` alone there.
func listingBothPositionsSem(answer Answer) Semantics {
	s := testSemantics()
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	s.ListedHashIsBareAfterANonName = No
	s.ListedHashIsBareUnlessItOpensTheValue = answer
	s.ListedTildeIsBareWhereItCannotExpand = answer
	return s
}

// A value carrying both characters is bare exactly when neither of them opens
// it. Asking the two rules as separate passes would quote every row here,
// which no column does — see Runner.positionallyBareValue.
func TestTheTwoListedPositionRulesCompose(t *testing.T) {
	for _, tc := range []struct{ value, bare, quoted string }{
		{"a#~b", "a#~b", "'a#~b'"},
		{"a~b#c", "a~b#c", "'a~b#c'"},
		{"a#:~b", "'a#:~b'", "'a#:~b'"},
		// And the openings still decide, whichever character takes the
		// position.
		{"~a#b", "'~a#b'", "'~a#b'"},
		{"#a~b", "'#a~b'", "'#a~b'"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			src := "v=" + shellSingleQuoted(tc.value) + "; typeset -p v"
			out, _ := run(t, src, withSem(listingBothPositionsSem(Yes)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.bare {
				t.Errorf("bare: got %q, want %q", got, "declare -- v="+tc.bare)
			}
			out, _ = run(t, src, withSem(listingBothPositionsSem(No)))
			if got := strings.TrimSpace(out); got != "declare -- v="+tc.quoted {
				t.Errorf("quoted: got %q, want %q", got, "declare -- v="+tc.quoted)
			}
		})
	}
}

// keyListingSem is the clustered listing, whose keys are the other half of
// both position rules: the engine that writes `[a#b]` and `[a~b]` bare is the
// one that writes `v=a#b` and `v=a~b` bare from a bare `set`.
func keyListingSem(answer Answer) Semantics {
	s := testSemantics()
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	s.ListedHashIsBareAfterANonName = No
	s.ListedHashIsBareUnlessItOpensTheValue = answer
	s.ListedTildeIsBareWhereItCannotExpand = answer
	return s
}

// A listed key takes the position rules its dialect's values take. Before
// this the key read the plainest predicate, which quotes every one of the
// bare rows.
func TestAListedKeyTakesThePositionRules(t *testing.T) {
	for _, tc := range []struct{ key, bare, quoted string }{
		{"a~b", `[a~b]="v"`, `["a~b"]="v"`},
		{"b~", `[b~]="v"`, `["b~"]="v"`},
		{"a#b", `[a#b]="v"`, `["a#b"]="v"`},
		{"a#", `[a#]="v"`, `["a#"]="v"`},
		{"a#~b", `[a#~b]="v"`, `["a#~b"]="v"`},
		{"a:x~b", `[a:x~b]="v"`, `["a:x~b"]="v"`},
		// Quoted under both: the two opening positions, and a key with
		// something else in it that needs quotes anyway.
		{"~b", `["~b"]="v"`, `["~b"]="v"`},
		{"a:~b", `["a:~b"]="v"`, `["a:~b"]="v"`},
		{"#b", `["#b"]="v"`, `["#b"]="v"`},
		{"a b", `["a b"]="v"`, `["a b"]="v"`},
		// And a plain key asks nothing.
		{"plain", `[plain]="v"`, `[plain]="v"`},
	} {
		t.Run(tc.key, func(t *testing.T) {
			// The key arrives through a parameter so the subscript holds
			// no quotes of its own: what is being measured is how the
			// listing spells a key, not how a subscript reads one.
			src := "k=" + shellSingleQuoted(tc.key) + "; typeset -A m; m[$k]=v; typeset -p m"
			out, _ := run(t, src, withSem(keyListingSem(Yes)))
			want := "declare -A m=(" + tc.bare + " )"
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("bare: got %q, want %q", got, want)
			}
			out, _ = run(t, src, withSem(keyListingSem(No)))
			want = "declare -A m=(" + tc.quoted + " )"
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("quoted: got %q, want %q", got, want)
			}
		})
	}
}
