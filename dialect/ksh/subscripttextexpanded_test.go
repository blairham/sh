// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A subscript that reaches a construct as **text** stands as the text it was
// written as here: this is the column that takes no second round, and it is
// the one that makes the two axes axes rather than a fix.
//
// Measured 2026-09-19 on ksh93u+ 2012-08-01, each probe from a script file
// with standard input on /dev/null. `typeset -A d; k='x y'; typeset
// 'd[$k]'=Q` lists `typeset -A d=(['$k']=Q)`, `[[ -v 'd[$k]' ]]` is false
// against an element stored under `x y`, and `i=3; typeset 'a[$i]'=Q` is
// `typeset: $i: arithmetic syntax error` — an unexpanded `$` reaching an
// expression that has already been expanded once. See
// interp.Semantics.ConditionIsSetExpandsAFlatSubscript and
// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
func TestASubscriptThatArrivedAsTextIsTheKey(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a declaration writes the text as the key",
			`typeset -A d; k='x y'; typeset 'd[$k]'=Q; for q in "${!d[@]}"; do printf '[%s]' "$q"; done`,
			"[$k]",
		},
		{
			"the condition looks the text up as the key",
			`typeset -A m; k='x y'; m[$k]=V; [[ -v 'm[$k]' ]] && echo SET || echo UNSET`,
			"UNSET\n",
		},
		{
			"and it finds the element stored under the text",
			`typeset -A m; k='x y'; m['$k']=V; [[ -v 'm[$k]' ]] && echo SET || echo UNSET`,
			"SET\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The two axes this dialect answers, pinned where the rest of them are.
func TestThisDialectTakesNoSecondRoundOverASubscriptText(t *testing.T) {
	if got := ksh.Semantics().ConditionIsSetExpandsAFlatSubscript; got != interp.No {
		t.Errorf("ConditionIsSetExpandsAFlatSubscript = %v, want %v", got, interp.No)
	}
	if got := ksh.Semantics().DeclarationOperandExpandsItsSubscript; got != interp.No {
		t.Errorf("DeclarationOperandExpandsItsSubscript = %v, want %v", got, interp.No)
	}
}
