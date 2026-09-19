// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A literal standing where an element goes — `a=( (1 2) (3 4) )`, which is one
// shell's multi-dimensional array.
//
// The store for it was already here: an element holding an array is what
// `a[1]=(p q)` and `a[1][2]=v` have always built. What was missing was the
// unsubscripted spelling inside the literal, and it was a **parse error**, so
// it cost every line of the script rather than its own (#3410).
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null, read through
// `sed -n l`. bash and dash refuse the paren outright and zsh reads it as a
// glob qualifier, so every row here is that column's.

func nestedLiteralRun(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := chainSem()
	sem.ArrayLiteralSubscriptIsAKey = Yes
	// A chain reaches *into* what an element holds rather than counting
	// through the value the link before it named, which is the reading the
	// one dialect with nested elements takes. See interp/nestedchainsub.go.
	sem.ChainedSubscriptReadsANestedValue = Yes
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ChainedAssignSubscript = true
		d.ChainedSubscript = true
		d.NestedArrayLiteral = true
		d.ArrayLiteralShapeFollowsTheFirstElement = true
		d.SubscriptSpansSeparators = true
		d.ParamIndirection = true
	}, withSem(sem))
}

// What the literal builds, read back through the listing that writes a nested
// element, and the issue's own example alongside it.
func TestANestedLiteralIsOneElementHoldingAnArray(t *testing.T) {
	for _, tc := range []struct{ why, src, want string }{
		{
			"the headline: two elements, each an array",
			`a=( (1 2) (3 4) ); echo "${a[1][0]} ${a[0][1]} ${#a[@]}"; typeset -p a`,
			"3 2 2\ntypeset -a a=((1 2) (3 4) )\n",
		},
		{
			"an element reads as its own first, and the subscripts are the positions",
			`a=( (1 2) (3 4) ); printf '<%s>' "${a[@]}" "${!a[@]}"`,
			"<1><3><0><1>",
		},
		{
			"one nested element",
			`a=( (1 2) ); echo "n=${#a[@]}"; typeset -p a`,
			"n=1\ntypeset -a a=((1 2) )\n",
		},
		{
			"an empty one is an element too, and takes no trailing blank",
			`a=( () ); echo "n=${#a[@]}"; typeset -p a`,
			"n=1\ntypeset -a a=(())\n",
		},
		{
			"plain elements stand beside nested ones",
			`a=( (1 2) x (3 4) ); typeset -p a`,
			"typeset -a a=((1 2) x (3 4) )\n",
		},
		{
			"and the trailing blank follows the last element, not the list",
			`a=( (1 2) x y ); typeset -p a`,
			"typeset -a a=((1 2) x y)\n",
		},
		{
			"it nests as far as it is written",
			`a=( ( (1 2) (3) ) (4) ); echo "${a[0][0][1]}"; typeset -p a`,
			"2\ntypeset -a a=(((1 2) (3) ) (4) )\n",
		},
		{
			"the close paren ends the element wherever it falls",
			`a=( (1 2)x ); typeset -p a`,
			"typeset -a a=((1 2) x)\n",
		},
		{
			"a chained write reaches into what it built",
			`a=( (1 2) (3 4) ); a[0][1]=Z; typeset -p a`,
			"typeset -a a=((1 Z) (3 4) )\n",
		},
		// The controls. Quoting makes the parentheses text, a literal with
		// no nested element is untouched, and the subscripted spelling still
		// places where it says.
		{
			"quoted, it is a string",
			`a=( "(1 2)" ); typeset -p a`,
			"typeset -a a=('(1 2)')\n",
		},
		{
			"a plain literal is what it was",
			`a=( x y ); typeset -p a`,
			"typeset -a a=(x y)\n",
		},
		{
			"and a subscripted one still is",
			`a=( [2]=c [0]=a ); typeset -p a`,
			"typeset -A a=([0]=a [2]=c)\n",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if out, st := nestedLiteralRun(t, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A literal that holds a nested element leaves a **keyed table** rather than an
// indexed array — unless its first element is the nested one.
//
// The first element decides and nothing else, which is what the last three
// rows are for: a literal whose first element nests stays indexed however many
// plain elements follow it, and the plain literal alongside says the rule is
// about nesting rather than about the elements.
//
// Nothing a script reads moves — the keys are the positions written out, so
// `${#a[@]}`, `${!a[@]}`, `${a[@]}` and `${a[1][0]}` answer the same either
// way. What changes is the attribute a listing shows.
func TestANestedElementAfterAPlainOneRetypesTheLiteral(t *testing.T) {
	for _, tc := range []struct{ why, src, want string }{
		{
			"a plain element in front of it",
			`a=( x (1 2) y ); typeset -p a`,
			"typeset -A a=([0]=x [1]=(1 2) [2]=y)\n",
		},
		{
			"two of them",
			`a=( x y (1 2) ); typeset -p a`,
			"typeset -A a=([0]=x [1]=y [2]=(1 2) )\n",
		},
		{
			"an empty one counts as one",
			`a=( "" (1 2) ); typeset -p a`,
			"typeset -A a=([0]='' [1]=(1 2) )\n",
		},
		{
			"and what a script reads does not move",
			`a=( x (1 2) y ); echo "${#a[@]} [${a[1][0]}] [${a[1][1]}]"; printf '<%s>' "${a[@]}" "${!a[@]}"`,
			"3 [1] [2]\n<x><1><y><0><1><2>",
		},
		{
			"a non-numeric subscript then goes in, which an indexed array refuses",
			`a=( x (1 2) y ); a[zz]=Q; typeset -p a`,
			"typeset -A a=([0]=x [1]=(1 2) [2]=y [zz]=Q)\n",
		},
		// The controls: the first element nesting keeps it indexed.
		{
			"the nested element first, with plain ones after",
			`a=( (1 2) x y ); typeset -p a`,
			"typeset -a a=((1 2) x y)\n",
		},
		{
			"the nested elements first and last",
			`a=( (1 2) x (3 4) ); typeset -p a`,
			"typeset -a a=((1 2) x (3 4) )\n",
		},
		{
			"and a literal with nothing nested in it at all",
			`a=( x y z ); typeset -p a`,
			"typeset -a a=(x y z)\n",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if out, st := nestedLiteralRun(t, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A literal whose first element names a subscript is the **keyed** shape, and
// there a parenthesis is a value and nothing else: it goes under the key the
// head named, and is refused anywhere else.
func TestANestedLiteralUnderAKeyGoesWhereTheHeadSaid(t *testing.T) {
	for _, tc := range []struct{ why, src, want string }{
		{"written against the head", `a=( [0]=(1 2) ); typeset -p a`, "typeset -A a=([0]=(1 2) )\n"},
		{"a blank between them changes nothing", `a=( [0]= (1 2) ); typeset -p a`, "typeset -A a=([0]=(1 2) )\n"},
		{"the append spelling takes it too", `a=( [0]+= (1 2) ); typeset -p a`, "typeset -A a=([0]=(1 2) )\n"},
		{
			"and it is the head just read, no further back",
			`a=( [0]= [1]= (1 2) ); typeset -p a`,
			"typeset -A a=([0]='' [1]=(1 2) )\n",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if out, st := nestedLiteralRun(t, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
