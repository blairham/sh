// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An exported name holding an **array or a table** has no environment
// representation, and the columns part over whether it reaches a child at all.
//
// The scalar view the array store keeps in step is not the answer: it handed
// a child the first element under every dialect — one column's answer given to
// three — and an empty entry for an empty array, which is nobody's (#1380).
func runExported(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true, "export": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ExportedCompoundReachesAChildAsItsFirstValue = a
		r.Semantics = &sem
	})
}

func TestAnExportedCompoundReachesAChildByAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, yes, no string }{
		{
			"an array declared with the letter",
			`typeset -x a=(p q); printf "[%s]" "$(env | grep '^a=')"`,
			"[a=p]", "[]",
		},
		{
			"an array exported afterwards",
			`a=(p q); export a; printf "[%s]" "$(env | grep '^a=')"`,
			"[a=p]", "[]",
		},
		{
			"a table",
			`typeset -Ax m; m[k]=v; printf "[%s]" "$(env | grep '^m=')"`,
			"[m=v]", "[]",
		},
		{
			// Nothing to hand over, so both answers hand a child nothing —
			// and an *empty* entry, which is what this used to write, is
			// neither of them.
			"an empty array",
			`typeset -x a=(); printf "[%s]" "$(env | grep '^a=')"`,
			"[]", "[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runExported(t, Yes, tc.src); out != tc.yes || st != 0 {
				t.Errorf("first value: %s = %q (status %d), want %q", tc.src, out, st, tc.yes)
			}
			if out, st := runExported(t, No, tc.src); out != tc.no || st != 0 {
				t.Errorf("no entry: %s = %q (status %d), want %q", tc.src, out, st, tc.no)
			}
		})
	}
}

// The scalar half is untouched under both answers, which is the control that
// says the axis moves the compound and nothing beside it.
func TestAnExportedScalarReachesAChildUnderEitherAnswer(t *testing.T) {
	const src = `export b=1; typeset -x c=(p); printf "[%s]" "$(env | grep -c '^[bc]=')"`
	if out, _ := runExported(t, Yes, src); out != "[2]" {
		t.Errorf("first value: = %q, want both names", out)
	}
	if out, _ := runExported(t, No, src); out != "[1]" {
		t.Errorf("no entry: = %q, want the scalar alone", out)
	}
}

// A subscripted operand's letters land on the name in two columns and on
// nothing in the third — which is a listing question, the compound reaching no
// child there anyway.
func TestASubscriptedOperandsAttributesAreAnAxis(t *testing.T) {
	const src = `typeset -x a[1]=v; typeset -p a`
	out, _ := runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.TypesetTakesASubscript = Yes

		sem.SubscriptedOperandCarriesTheAttributes = Yes
		r.Semantics = &sem
	})
	if !strings.Contains(out, "x") {
		t.Errorf("carried: %s = %q, want the export letter listed", src, out)
	}
	out, _ = runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.TypesetTakesASubscript = Yes

		sem.SubscriptedOperandCarriesTheAttributes = No
		r.Semantics = &sem
	})
	if strings.Contains(out, "-ax") || strings.Contains(out, "-x") {
		t.Errorf("not carried: %s = %q, want no export letter", src, out)
	}
	// Either way the element is written, which is what says the axis is
	// about the attribute and not about the assignment.
	if !strings.Contains(out, "v") {
		t.Errorf("not carried: %s = %q, want the element still written", src, out)
	}
}

// A subscripted operand carrying **no value** declares the name as an array
// and writes no element.
//
// It used to declare a variable literally named `a[3]` — invisible to
// `${a[3]}` and to `typeset -p a`, at status 0, so a script that declared an
// array that way had none (#1380).
func TestAValuelessSubscriptedOperandDeclaresTheArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an unset name becomes an array", `typeset a[3]; printf "[%s]" "${#a[@]}" "$(typeset -p a)"`, `[0][declare -a a=()]`},
		{"and no element is written", `typeset a[3]; printf "[%s]" "${a[3]-none}"`, `[none]`},
		{"an array already standing is left alone", `a=(x y); typeset a[3]; printf "[%s]" "${a[@]}"`, `[x][y]`},
		// The control: with a value it is the element declaration it always
		// was.
		{"with a value it writes the element", `typeset a[3]=v; printf "[%s]" "${a[3]}"`, `[v]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, func(d *syntax.Dialect) {
				d.ArraySubscript = true
				d.ArrayLiteral = true
				d.DeclarationUtilities = map[string]bool{"typeset": true}
			}, func(r *Runner) {
				sem := *r.Semantics
				sem.ArraysAreSparse = Yes
				sem.TypesetTakesASubscript = Yes
				r.Semantics = &sem
			})
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
