// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A chain of subscripts — `${m[k][2]}` — where each one reads what the one
// before it named.
//
// Measured on zsh 5.9.2, the only panel shell with the grammar; the tests
// here name the grammar flag and the axes and never the shell, and
// dialect/zsh carries the end-to-end half.
//
// The one question the construct adds is what each link is *handed*: a
// subscript counts elements when it is given a list and characters when it is
// given one string, and a chain is that rule applied as many times as it is
// written. Everything else — the base, the range, the search letters, the
// character reading — is the subject of its own tests already.

// chainedSubscriptGrammar is the grammar this construct needs: the chain
// itself, and a subscript that may carry a flag group.
func chainedSubscriptGrammar(d *syntax.Dialect) {
	d.ArrayLiteral = true
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	d.ChainedSubscript = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
}

// runChainedSubscript runs src with that grammar and with every axis the
// *reading* depends on pinned, so a test asserting the chain is not also
// asserting one of them by accident. Each is the subject of its own tests
// elsewhere: where an array starts, whether a comma in a subscript is a
// range, whether a subscript on a string names a character, whether a bare
// array name is its elements, and whether an unquoted expansion is split.
func runChainedSubscript(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, chainedSubscriptGrammar, func(r *Runner) {
		d := syntax.Core()
		chainedSubscriptGrammar(&d)
		r.Dialect = &d
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.SubscriptCommaIsARange = Yes
		sem.ScalarSubscriptIsACharacter = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.SplitParamExpansion = No
		r.Semantics = &sem
	})
}

// A link that named one value hands the next one a string, which it counts by
// character.
func TestASubscriptBehindOneValueCountsCharacters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an element", `a=(one two three); printf "[%s]" "${a[1][2]}"`, "[n]"},
		{"the first character", `a=(one two three); printf "[%s]" "${a[2][1]}"`, "[t]"},
		{"counting back from the end", `a=(one two three); printf "[%s]" "${a[2][-1]}"`, "[o]"},
		{"a range of characters", `a=(one two three); printf "[%s]" "${a[3][2,4]}"`, "[hre]"},
		{"past the end is no character", `a=(one two three); printf "[%s]" "${a[1][9]}"`, "[]"},
		{"a key's value", `typeset -A m; m[k]=abc; printf "[%s]" "${m[k][2]}"`, "[b]"},
		{"a key's value counted back", `typeset -A m; m[k]=abc; printf "[%s]" "${m[k][-1]}"`, "[c]"},
		{"a key's value by range", `typeset -A m; m[k]=abc; printf "[%s]" "${m[k][1,2]}"`, "[ab]"},
		{"behind a search", `a=(one two three); printf "[%s]" "${a[(r)two][1]}"`, "[t]"},
		{"a range over a string", `s=abcdef; printf "[%s]" "${s[2,4][2]}"`, "[c]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runChainedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A link that named several values hands the next one a list, which it counts
// by element.
//
// This is the half a rewrite into the nested reading would get wrong:
// `${${a[2,4]}[1]}` is `t`, because quoting joins what the inner expansion
// came to before the subscript is read, and `${a[2,4][1]}` is `two`.
func TestASubscriptBehindAListCountsElements(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"behind a range", `a=(one two three four five); printf "[%s]" "${a[2,4][1]}"`, "[two]"},
		{"the second of them", `a=(one two three four five); printf "[%s]" "${a[2,4][2]}"`, "[three]"},
		{"the last of them", `a=(one two three four five); printf "[%s]" "${a[2,4][-1]}"`, "[four]"},
		{"past the end of them", `a=(one two three four five); printf "[%s]" "${a[2,4][9]}"`, "[]"},
		{"behind the whole array", `a=(one two three); printf "[%s]" "${a[@][2]}"`, "[two]"},
		{"behind the joined array", `a=(one two three); printf "[%s]" "${a[*][2]}"`, "[two]"},
		{"a range of them", `a=(one two three four five); printf "[%s]" "${a[2,4][1,2]}"`, "[two three]"},
		{"searching them", `a=(one two three four five); printf "[%s]" "${a[2,4][(r)three]}"`, "[three]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runChainedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The rule applies as many times as it is written, and each link is decided
// by the link before it rather than by how deep it is.
func TestAChainIsNotLimitedToTwoSubscripts(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"element then character", `a=(one two three four five); printf "[%s]" "${a[2,4][2][3]}"`, "[r]"},
		{"elements then an element", `a=(one two three four five); printf "[%s]" "${a[2,4][1,2][2]}"`, "[three]"},
		{"three characters deep", `s=abcdef; printf "[%s]" "${s[2,5][2,3][1]}"`, "[c]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runChainedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// How many fields the chain makes, and whether its length is a count or a
// width, are questions about the *final* subscript. Asked of the first they
// come out differently, which is what these assert.
func TestAChainsShapeIsTheLastSubscripts(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a range behind a range is a list", `a=(one two three); set -- ${a[1,3][1,2]}; printf "n=%d" $#`, "n=2"},
		{"an element behind a range is one field", `a=(one two three); set -- ${a[1,3][2]}; printf "n=%d" $#`, "n=1"},
		{"an element behind the whole array is one field", `a=(one two three); set -- "${a[@][2]}"; printf "n=%d" $#`, "n=1"},
		{"a length behind a range is a count", `a=(one two three); printf "[%s]" "${#a[1,3][1,2]}"`, "[2]"},
		{"a length of one element is a width", `a=(one two three); printf "[%s]" "${#a[1,3][2]}"`, "[3]"},
		{"a length behind the whole array is a width", `a=(one two three); printf "[%s]" "${#a[@][2]}"`, "[3]"},
		{"a length of a key's characters", `typeset -A m; m[k]=abc; printf "[%s]" "${#m[k][1,2]}"`, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runChainedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A link before the last that named nothing leaves the whole chain unset,
// which is what the conditionals test.
func TestAChainWhoseLinkNamedNothingIsUnset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no such element", `a=(x y); printf "[%s]" "${a[9][1]-none}"`, "[none]"},
		{"no such key", `typeset -A m; m[k]=abc; printf "[%s]" "${m[zz][1]-none}"`, "[none]"},
		{"no such element of a range", `a=(one two three); printf "[%s]" "${a[1,3][9][1]-none}"`, "[none]"},
		// The link that named nothing is what makes this unset, and a search
		// behind it must reach that answer rather than the refusal a search
		// over a *string* carries: with the miss read as an empty value, the
		// group would be searching one.
		{"no such element, then a search", `a=(x y); printf "[%s]" "${a[9][(i)q]-none}"`, "[none]"},
		{"a range that fell off the end still answers", `a=(x y); printf "[%s]" "${a[9,10][@]-none}"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runChainedSubscript(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A refusal from inside a chain names every subscript that was written, so a
// reader can find the text in the script. Named `a[(w)q]` it would name a
// subscript the file does not contain.
func TestARefusalFromInsideAChainNamesTheWholeChain(t *testing.T) {
	out, st := runChainedSubscript(t, `a=(x y); printf "[%s]" "${a[1][(w)q]}"`)
	const want = "${a[1][(w)q]}: the (w) subscript flag is not implemented"
	if !strings.Contains(out, want) {
		t.Errorf("gave %q, want a refusal naming %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the unbuilt flag refused")
	}
}

// A search over an *association* is not a source the rest of a chain reads,
// and this refuses it by name rather than reading the matches as one.
func TestAChainBehindAnAssociationSearchIsRefused(t *testing.T) {
	out, st := runChainedSubscript(t, `typeset -A m; m[alpha]=1; m[beta]=2; printf "[%s]" "${m[(I)*][2]}"`)
	const want = "${m[(I)*][2]}: a chain behind a search over an association is not implemented"
	if !strings.Contains(out, want) {
		t.Errorf("gave %q, want a refusal naming %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the unbuilt shape refused")
	}
}

// A chain on a nested expansion is the two constructs at once and is refused
// by name, rather than answered with the last subscript alone.
func TestAChainOnANestedExpansionIsRefused(t *testing.T) {
	out, st := runGrammar(t, `a=(hello world); printf "[%s]" "${${a}[1][2]}"`,
		func(d *syntax.Dialect) {
			chainedSubscriptGrammar(d)
			d.NestedParamExpansion = true
		}, func(r *Runner) {
			d := syntax.Core()
			chainedSubscriptGrammar(&d)
			d.NestedParamExpansion = true
			r.Dialect = &d
			sem := *r.Semantics
			sem.ArrayBaseIsZero = No
			sem.SubscriptCommaIsARange = Yes
			sem.ScalarSubscriptIsACharacter = Yes
			sem.ArrayNameWithoutSubscriptIsTheList = Yes
			r.Semantics = &sem
		})
	const want = "a chain of subscripts on a nested expansion is not implemented"
	if !strings.Contains(out, want) {
		t.Errorf("output %q, want a refusal naming %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the unbuilt shape refused")
	}
}
