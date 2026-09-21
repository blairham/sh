// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A compound assignment's word does not end at its `)` here: text written
// after it stays part of the same word, and the word is then not an array
// assignment at all.
//
// Measured 2026-09-21 on bash 5.3.20 and 3.2.57, one `-c` per row under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`. See
// syntax.Dialect.CompoundAssignmentWordRunsPastItsParenthesis for the panel
// — ksh93u+ and zsh 5.9.2 end the word at the parenthesis and go looking for
// a command called `x`.
func TestAnOperandThatRunsPastItsParenthesis(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the whole of it is one operand",
			`declare() { printf "<%s>" "$@"; echo; }; declare a=(1 2)x`,
			`<a=(1 2)x>`,
		},
		{
			"and it declares a scalar",
			`declare a=(1 2)x; declare -p a`,
			`declare -- a="(1 2)x"`,
		},
		{
			"a prefix assignment folds the same way",
			`a=(1 2)x; declare -p a`,
			`declare -- a="(1 2)x"`,
		},
		{
			"the elements are written out again with one blank",
			`declare() { printf "<%s>" "$@"; echo; }; declare a=( 1    2 )x`,
			`<a=(1 2)x>`,
		},
		{
			"an expansion in an element still expands",
			`v=Q; declare() { printf "<%s>" "$@"; echo; }; declare a=($v 2)x`,
			`<a=(Q 2)x>`,
		},
		{
			"and a quoted element loses its quotes where it stands",
			`declare() { printf "<%s>" "$@"; echo; }; declare a=(1 "2 3")x`,
			`<a=(1 2 3)x>`,
		},
		{
			"what follows the word is the next operand",
			`let() { printf "<%s>" "$@"; echo; }; let a=(5+3)x y`,
			`<a=(5+3)x><y>`,
		},
		{
			// The line #2298's array row is about: the division is part of
			// the arithmetic rather than an operand of its own.
			"so arithmetic written after the parentheses is arithmetic",
			`let a=(5 + 3)/2; echo "a=$a st=$?"`,
			`a=4 st=0`,
		},
		{
			// The control: a blank ends the word, the array is an operand
			// again and `x` is a word of its own.
			"a blank ends the word",
			`declare a=(1 2) x; declare -p a`,
			`declare -a a=([0]="1" [1]="2")`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}

// A subscripted operand whose value is parenthesized is stored as its
// characters, with a sentence beside it.
//
// Measured 2026-09-21 on bash 5.3.20. See
// interp.Runner.warnQuotedCompoundAtASubscript for the twenty rows the three
// conditions were read off.
func TestASubscriptedOperandHoldingACompoundIsDeprecated(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the warning, and the characters stored",
			`declare a[1]="(var)"; declare -p a`,
			"sh: line 1: warning: a[1]=(var): quoted compound array assignment deprecated\n" +
				`declare -a a=([1]="(var)")`,
		},
		{
			"a blank inside does not matter",
			`declare a[1]="(v ar)"; declare -p a`,
			"sh: line 1: warning: a[1]=(v ar): quoted compound array assignment deprecated\n" +
				`declare -a a=([1]="(v ar)")`,
		},
		{
			"no declaration word, nothing said",
			`a[1]="(var)"; declare -p a`,
			`declare -a a=([1]="(var)")`,
		},
		{
			"nor for a table",
			`declare -A m; declare m[k]="(v)"; declare -p m`,
			`declare -A m=([k]="(v)" )`,
		},
		{
			"nor where the name is already an array",
			`declare -a a; declare a[1]="(v)"; declare -p a`,
			`declare -a a=([1]="(v)")`,
		},
		{
			"nor where the declaration makes a local",
			`f() { declare a[1]="(v)"; declare -p a; }; f`,
			`declare -a a=([1]="(v)")`,
		},
		{
			"unless the letter puts it back on the global cell",
			`f() { declare -g a[1]="(v)"; }; f; declare -p a`,
			"sh: line 1: warning: a[1]=(v): quoted compound array assignment deprecated\n" +
				`declare -a a=([1]="(v)")`,
		},
		{
			"the parentheses have to be at the ends",
			`declare a[1]=" (v) "; declare -p a`,
			`declare -a a=([1]=" (v) ")`,
		},
		{
			// And the letter on the same line takes the subscript out of
			// the operand altogether: the value is re-read into the base
			// name, replacing what stood there, and nothing is said. #4105.
			"the array letter drops the subscript",
			`declare -a a=(z); declare -a a[1]="(v w)"; declare -p a`,
			`declare -a a=([0]="v" [1]="w")`,
		},
		{
			"the append spelling is dropped with it",
			`declare -a a[1]+="(v)"; declare -p a`,
			`declare -a a=([0]="v")`,
		},
		{
			"but an ordinary value keeps its subscript",
			`declare -a a[1]=plain; declare -p a`,
			`declare -a a=([1]="plain")`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := answersRun(t, c.src)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
		})
	}
}

// `eval` and `let` read a `name=( … )` operand themselves, and what they are
// handed is the assignment written out again — as the **fields** of its
// expansions, since the text is an ordinary word.
//
// Measured 2026-09-21 on bash 5.3.20 with `x="p q"`. See
// interp/rejoinedoperand.go for the whole table.
func TestARejoinedOperandIsFieldSplit(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the break the expansion made",
			`eval() { printf "<%s>" "$@"; echo; }; x="p q"; eval a=($x)`,
			`<a=(p><q)>`,
		},
		{
			// The discriminator: ` r)` is literal text of the same word, so
			// it joins the expansion's last field rather than standing
			// alone. One word per element would give three words here.
			"literal text after it joins the last field",
			`eval() { printf "<%s>" "$@"; echo; }; x="p q"; eval a=($x r)`,
			`<a=(p><q r)>`,
		},
		{
			"a quoted element is one field",
			`eval() { printf "<%s>" "$@"; echo; }; eval a=("p q" r)`,
			`<a=(p q r)>`,
		},
		{
			// Where the rejoined text parses differently split than joined,
			// which is the only place `eval` can see the difference at all.
			"and `let` reads the words as arithmetic",
			`x="1 + 2"; let a=($x) 2>/dev/null; echo "a=$a st=$?"`,
			`a= st=1`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := answersRun(t, c.src)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
		})
	}
}
