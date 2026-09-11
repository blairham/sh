// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// withKeyListing turns on the grammar for `${!name[@]}`, which is how a script
// asks a table for its keys. Named for the construct rather than for a shell,
// as every grammar switch in these tests is.
func withKeyListing(d *syntax.Dialect) { d.ParamIndirection = true }

// The order a keyed table lists in — #1758, filed as "this shell lists by key
// where the shell it models keeps insertion order, so the first-value axis is
// right only by coincidence".
//
// Re-measured 2026-09-11, and **no shell in the panel keeps insertion order**.
// The same three keys list identically however they were assigned in every
// column: bash and zsh list in the order their hash puts them, ksh93 sorts by
// key, and this shell sorts by key. So what is pinned here is the property
// this implementation actually promises — a deterministic order that does not
// depend on how the table was built — and the probes are the ones that can
// tell the three readings apart. docs/spec/semantics.md holds the panel.
func TestAKeyedTableListsByKeyWhateverOrderItWasBuiltIn(t *testing.T) {
	// The three permutations of one table. Insertion order would give three
	// different answers, a hash order would give one that is neither sorted
	// nor historical, and sorting gives this.
	for _, build := range []string{
		`m[z]=1; m[a]=2; m[m]=3`,
		`m[a]=2; m[m]=3; m[z]=1`,
		`m[m]=3; m[z]=1; m[a]=2`,
	} {
		src := `typeset -A m; ` + build + `; printf "[%s]" "${!m[@]}"; printf "[%s]" "${m[@]}"`
		got, _ := runGrammar(t, src, withKeyListing, nil)
		if want := "[a][m][z][2][3][1]"; got != want {
			t.Errorf("%s listed %q, want %q", build, got, want)
		}
	}
	// Byte order and not something that happens to agree with it: a digit, an
	// upper-case letter, an underscore and a lower-case letter sort in that
	// order, which is the row no hash produces by accident. It is ksh93's
	// answer exactly — measured, `1 B _x a`.
	got, _ := runGrammar(t, `typeset -A m; m[B]=1; m[a]=2; m[1]=3; m[_x]=4; printf "[%s]" "${!m[@]}"`,
		withKeyListing, nil)
	if want := "[1][B][_x][a]"; got != want {
		t.Errorf("mixed keys listed %q, want %q", got, want)
	}
}

// Removing a key and assigning it again does not move it, which is the probe
// that separates "insertion order" from "first seen": under either of those a
// re-added key would move to the end, and under a sorted order it cannot move
// at all. Measured on zsh too, where it does not move either.
func TestARemovedKeyComesBackWhereItWas(t *testing.T) {
	src := `typeset -A m; m[z]=1; m[a]=2; m[m]=3; unset "m[z]"; m[z]=9; printf "[%s]" "${!m[@]}" "${m[@]}"`
	got, _ := runGrammar(t, src, withKeyListing, nil)
	if want := "[a][m][z][2][3][9]"; got != want {
		t.Errorf("= %q, want %q", got, want)
	}
}

// Every surface that walks a table walks it in that one order: the keys, the
// values, the two together, and a `for` over either. One order rather than
// several is the part a later change could break silently — a table listing
// one way for `${m[@]}` and another for a loop reads as working until
// something lines the two up.
func TestOneOrderForEverySurface(t *testing.T) {
	const build = `typeset -A m; m[z]=1; m[a]=2; m[m]=3; `
	for _, c := range []struct{ name, src, want string }{
		{"the keys", `printf "[%s]" "${!m[@]}"`, "[a][m][z]"},
		{"the values", `printf "[%s]" "${m[@]}"`, "[2][3][1]"},
		{"a loop over the keys", `for k in "${!m[@]}"; do printf "[%s]" "$k"; done`, "[a][m][z]"},
		{"a loop over the values", `for v in "${m[@]}"; do printf "[%s]" "$v"; done`, "[2][3][1]"},
		{"the count, which is not an order but is the same table", `printf "[%s]" "${#m[@]}"`, "[3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := runGrammar(t, build+c.src, withKeyListing, nil)
			if got != c.want {
				t.Errorf("= %q, want %q", got, c.want)
			}
		})
	}
}

// And the axis reads the first of *that* order, which is what #1758 was
// really about: `KeyedTableScalarIsTheFirstValue` says which end to read and
// the order underneath it is this implementation's.
//
// The table is deliberately one whose sorted order is not the order it was
// written in. A probe over `m=(a 1 b 2)` answers `1` under every reading there
// is and says nothing at all.
func TestTheFirstValueIsTheFirstOfThisOrder(t *testing.T) {
	sem := testSemantics()
	sem.ArrayScalarIsTheWholeArray = No
	sem.KeyedTableScalarIsTheFirstValue = Yes
	got, _ := runGrammar(t, `typeset -A m; m[z]=1; m[a]=2; m[m]=3; printf "[%s]" "$m"`,
		withKeyListing, func(r *Runner) { r.Semantics = &sem })
	if want := "[2]"; got != want {
		t.Errorf("= %q, want %q — the value of the first key, `a`", got, want)
	}
	// The shell this axis was taken from answers `1` here, because `z` is
	// first in its hash order. That is recorded rather than reproduced: a
	// hash order is a property of a hash function and a table size, which is
	// implementation and not behavior. See docs/spec/semantics.md.
	if strings.Contains(got, "1") {
		t.Errorf("= %q: the recording machine's zsh answers 1 here and we must not, by accident", got)
	}
}
