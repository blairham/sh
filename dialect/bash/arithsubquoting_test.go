// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A quotation inside an arithmetic subscript holds its brackets: what ends
// the subscript is the `]` written outside the quotes, so a key spelled with
// a bracket in it is reachable from arithmetic.
//
// Measured 2026-09-16 on bash 5.3.20 and on the same binary as `sh`, each
// probe from a script file with standard input on /dev/null. ksh93u+
// 2012-08-01 answers every row here the same way — see
// syntax.Dialect.ArithSubscriptQuoting, which is why this is a grammar flag
// and not a semantics axis (#3302).
func TestAQuotationInAnArithmeticSubscriptHoldsItsBrackets(t *testing.T) {
	const setup = `declare -A a; a[']']=5; a['x]']=6; a[q]=8; `
	for _, c := range []struct{ name, src, want string }{
		{"a key that is one bracket", setup + `(( r = a[']'] )); echo $r`, "5"},
		{"a key with a bracket in it", setup + `(( r = a['x]'] )); echo $r`, "6"},
		{"through a dollar-arithmetic", setup + `echo $(( a[']'] ))`, "5"},
		{"a condition reads it too", setup + `[[ a[']'] -eq 5 ]] && echo 5`, "5"},
		// The whole expression out of a value, where the brackets are the
		// value's own and there is no written bracket to prefer.
		{"the expression out of a value", setup + `e="a[']']"; echo $(( $e ))`, "5"},
		// The other half of the rule: a quote a *value* carried is a
		// character of the key and opens nothing, so the two spellings of
		// the same three characters reach two different elements.
		{"a value's own quotes", `declare -A a; a["'q'"]=21; a[q]=22; k="'q'"; (( r = a[$k] )); echo $r`, "21"},
		{"the script's quotes", `declare -A a; a["'q'"]=21; a[q]=22; (( r = a['q'] )); echo $r`, "22"},
		{"a value's own double quotes", `declare -A a; a['"q"']=23; a[q]=22; k='"q"'; (( r = a[$k] )); echo $r`, "23"},
		{"a value's own backslash", `declare -A a; a['\q']=31; a[q]=22; k='\q'; (( r = a[$k] )); echo $r`, "31"},
		// The controls. A written bracket closes the subscript and what
		// follows is an operator nobody wrote, and a written one left open
		// closes nothing — both refused, exactly as they were.
		{"a written bracket closes it", setup + `{ (( a[q]] )); } 2>/dev/null; echo refused=$?`, "refused=1"},
		{"a written bracket left open", setup + `{ (( a[q[r] )); } 2>/dev/null; echo refused=$?`, "refused=1"},
		// And a subscript with no quoting in it is untouched: still a key on
		// an association and still an expression on an indexed name.
		{"an absent key", setup + `(( r = a[zz] )); echo $r`, "0"},
		{"indexed stays arithmetic", `b=(9 8 7); (( r = b[1+1] )); echo $r`, "7"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The grammar flag and the axis this dialect answers, pinned where the rest
// of them are pinned. The two are separate measurements: bash and ksh93 agree
// on the first and part on the second, because a condition's operand reaches
// its arithmetic already expanded and only this shell takes the reading the
// word still offers.
func TestThisDialectReadsAQuotedArithmeticSubscript(t *testing.T) {
	if got := bash.Dialect().ArithSubscriptQuoting; !got {
		t.Errorf("ArithSubscriptQuoting = %v, want true", got)
	}
	if got := bash.Semantics().ConditionArithmeticReadsTheWrittenSubscript; got != interp.Yes {
		t.Errorf("ConditionArithmeticReadsTheWrittenSubscript = %v, want %v", got, interp.Yes)
	}
}
