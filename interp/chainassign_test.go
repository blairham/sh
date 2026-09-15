// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An assignment whose name carries more than one subscript, where each one
// after the first reaches into the value the one before it named.
//
// The grammar flag is named here and the shell that sets it is not. What the
// chain builds is a value this store already had — Element.Nested, which
// `a[1]=([2]=v)` has always built — so these rows are about the *walk*.

// chainSem is the axes on the way to a nested element, answered the way the
// one dialect with the grammar answers them so that a row about the *walk* is
// not stopped by a question about something else.
func chainSem() Semantics {
	sem := permissive()
	sem.SubscriptedArrayLiteral = SubscriptedArrayLiteralNests
	sem.ArrayBaseIsZero = Yes
	sem.ArraysAreSparse = Yes
	sem.TypesetTakesASubscript = Yes
	sem.DeclarationTakesASubscript = Yes
	sem.SubscriptedOperandTakesTheIntegerAttribute = Yes
	sem.DeclaredNameWithoutValueIsEmpty = No
	sem.CompoundAttribute = CompoundAttributeFoldsEveryElement
	sem.CompoundElementsGoThroughTheAttribute = Yes
	sem.DeclareListing = DeclareListingBareAssignments
	sem.DeclareValueQuoting = ListingQuoteWhenNeededDollar
	return sem
}

func chainRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return chainRunWith(t, src, chainSem())
}

func chainRunWith(t *testing.T, src string, sem Semantics) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ChainedAssignSubscript = true
	}, withSem(sem))
}

// The store the chain builds, read back through the listing that writes a
// nested element.
func TestAChainedAssignmentNestsTheValue(t *testing.T) {
	for _, tc := range []struct{ why, src, want string }{
		{
			"the headline, and the same value the literal spelling builds",
			`a[1][2]=v; typeset -p a`, `typeset -a a=([1]=([2]=v) )`,
		},
		{
			"a second write into the same nested array grows it",
			`a[1][2]=v; a[1][3]=w; typeset -p a`, `typeset -a a=([1]=([2]=v [3]=w) )`,
		},
		{
			"and a different first subscript builds a second nested array",
			`a[1][2]=v; a[2][0]=w; typeset -p a`, `typeset -a a=([1]=([2]=v) [2]=(w) )`,
		},
		{
			// A walk and not a special case for two.
			"three deep",
			`a[1][2][3]=v; typeset -p a`, `typeset -a a=([1]=([2]=([3]=v) ) )`,
		},
		{
			// appendedOverAScalar's rule one level down.
			"a string already in the element is promoted rather than replaced",
			`a=(x y); a[1][2]=v; typeset -p a`, `typeset -a a=(x ([0]=y [2]=v) )`,
		},
		{
			"an element holding the empty string is a value and is promoted too",
			`a[1]=""; a[1][2]=v; typeset -p a`, `typeset -a a=([1]=([0]='' [2]=v) )`,
		},
		{
			// The control for the row above: an element that is not there is
			// not promoted, so the base stays empty.
			"an element that is not there leaves no base behind",
			`a[1][2]=v; typeset -p a`, `typeset -a a=([1]=([2]=v) )`,
		},
		{
			"and the scalar the whole name was holding is promoted as well",
			`s=abc; s[1][2]=v; typeset -p s`, `typeset -a s=(abc ([2]=v) )`,
		},
		{
			"the last subscript is an ordinary element write, append included",
			`a[1][2]=v; a[1][2]+=Q; typeset -p a`, `typeset -a a=([1]=([2]=vQ) )`,
		},
		{
			"an append to an element that is not there is the value alone",
			`a[1][2]+=v; typeset -p a`, `typeset -a a=([1]=([2]=v) )`,
		},
		{
			"the subscripts are expanded, not taken as text",
			`i=1; j=2; a[$i][$j]=v; typeset -p a`, `typeset -a a=([1]=([2]=v) )`,
		},
	} {
		out, st := chainRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: %s gave %q, want %q", tc.why, tc.src, strings.TrimSpace(out), tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.why, st)
		}
	}
}

// The chain does not change how long the array is: the whole of it is one
// element, however deep the nesting under it goes.
func TestAChainedAssignmentLeavesOneElement(t *testing.T) {
	out, st := chainRun(t, `a[1][2][3]=v; echo "n=${#a[@]}"; typeset -p a`)
	if !strings.HasPrefix(strings.TrimSpace(out), "n=1\n") {
		t.Errorf("got %q, want one element however deep the nesting", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// Only the first subscript can be a key. The table attribute belongs to the
// *name*, and what a link indexes is an array — so a link is arithmetic, and
// one naming nothing evaluates to the base.
func TestOnlyTheFirstSubscriptOfAChainCanBeAKey(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[k][2]=v; typeset -p m`, `typeset -A m=([k]=([2]=v) )`},
		{`typeset -A m; m[k][x]=v; typeset -p m`, `typeset -A m=([k]=(v) )`},
	} {
		out, st := chainRun(t, tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s gave %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.src, st)
		}
	}
}

// A declaration's operand takes the chain too, and lands in the same place:
// the two spellings are one value in the shell that has them, so they are one
// walk here.
func TestADeclarationOperandTakesTheChain(t *testing.T) {
	plain, _ := chainRun(t, `a[1][2]=v; typeset -p a`)
	for _, src := range []string{
		`typeset a[1][2]=v; typeset -p a`,
		`export a[1][2]=v; typeset -p a`,
	} {
		out, st := chainRun(t, src)
		if !strings.Contains(out, "([2]=v)") {
			t.Errorf("%s gave %q, want the nested element the plain spelling builds (%q)", src, out, plain)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", src, st)
		}
	}
}

// The name's attributes reach the value the chain writes, because that is
// where an element assignment's value meets them wherever it is written.
//
// Not the *words of a nested literal*, which is the neighboring row and the
// opposite answer: `typeset -i a; a[1]=(5+5)` keeps its text.
func TestTheChainsValueGoesThroughTheNamesAttribute(t *testing.T) {
	sem := chainSem()
	for _, tc := range []struct{ why, src, want string }{
		{
			"the chain's own value is evaluated",
			`typeset -i a[1][2]=5+5; typeset -p a`, `typeset -a -i a=([1]=([2]=10) )`,
		},
		{
			"a nested literal's words are not",
			`typeset -i a; a[1]=(5+5); typeset -p a`, `typeset -a -i a=([1]=(5+5) )`,
		},
		{
			// And the attribute *arriving* over a standing nested array does
			// fold, and folds into the nesting rather than flattening it.
			"the attribute arriving over one folds inside it",
			`a[1]=(5+5); typeset -i a; typeset -p a`, `typeset -a -i a=([1]=(10) )`,
		},
	} {
		out, st := chainRunWith(t, tc.src, sem)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: %s gave %q, want %q", tc.why, tc.src, strings.TrimSpace(out), tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.why, st)
		}
	}
}
