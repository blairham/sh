// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.BareElementsInATableLiteralAreEachOneValue, both ways round, over
// one source that cannot answer itself.
//
// `typeset -A m=($k $v)` with `k='1 2'` and `v='3 4 5'` is one key under the
// axis and five fields — so three keys, the last holding nothing — without
// it. The table's **count** is the assertion rather than any one key, because
// the count is the thing the two readings cannot both produce.
func TestAKeyedLiteralsBareElementsFollowTheAxis(t *testing.T) {
	const src = `typeset -A m=($k $v); printf "[%s][%s]" "${#m[@]}" "${m[$k]}"`
	const prelude = `k='1 2'; v='3 4 5'; `
	for _, tc := range []struct {
		name string
		axis Answer
		want string
	}{
		{"one value each", Yes, `[1][3 4 5]`},
		{"an ordinary word list", No, `[3][]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKeyedLiteral(t, tc.axis, prelude+src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The star and the empty word are the rows a shell that merely declines to
// split still answers the other way, which is why the axis is about being an
// assignment's value and not about splitting.
func TestAKeyedLiteralsBareElementIsNotGlobbedOrDropped(t *testing.T) {
	out, st := runKeyedLiteral(t, Yes, `typeset -A m=(a '*'); printf "[%s]" "${m[a]}"`)
	if want := "[*]"; out != want || st != 0 {
		t.Errorf("the star = %q status %d, want %q at 0", out, st, want)
	}
	out, st = runKeyedLiteral(t, Yes, `e=; typeset -A m=(p $e q); printf "[%s]" "${#m[@]}"`)
	if want := "[2]"; out != want || st != 0 {
		t.Errorf("the empty word = %q status %d, want %q at 0", out, st, want)
	}
	out, st = runKeyedLiteral(t, No, `e=; typeset -A m=(p $e q); printf "[%s]" "${#m[@]}"`)
	if want := "[1]"; out != want || st != 0 {
		t.Errorf("the empty word without the axis = %q status %d, want %q at 0", out, st, want)
	}
}

// And an indexed literal never puts the question, which is the control: the
// axis is on and `a=($k)` is still two elements.
func TestAnIndexedLiteralDoesNotAskTheKeyedLiteralsQuestion(t *testing.T) {
	out, st := runKeyedLiteral(t, Yes, `k='1 2'; a=($k); printf "[%s]" "${#a[@]}"`)
	if want := "[2]"; out != want || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, want)
	}
}

func runKeyedLiteral(t *testing.T, axis Answer, src string) (string, int) {
	t.Helper()
	sem := testSemantics()
	sem.BareElementsInATableLiteralAreEachOneValue = axis
	return runGrammar(t, src, func(d *syntax.Dialect) { d.ParamIndirection = true },
		func(r *Runner) { r.Semantics = &sem })
}
