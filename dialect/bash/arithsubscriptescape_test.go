// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// An arithmetic expression the script wrote inside `(( … ))` has its double
// quotes removed even when it had to be expanded first.
//
// The quotation is a quoting context while the expansions in it are performed
// and is gone from the result, so `(( "m[$k]++" ))` is the same expression
// `(( m[$k]++ ))` is. Measured 2026-09-22 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` with standard input on the
// null device, bash 5.3.20, `i=1`:
//
//	declare -A m; k='x],b['; m[$k]=1; (( "m[$k]++" ))   the element is 2
//	$(( 1"0" + $i ))                                    11
//	q='"'; $(( 1${q}0${q} + $i ))                       refused as an operator
//
// The last row is the control that says it is the *script's* quotes and not
// every quote: the same two bytes arriving in a value are characters of the
// result. Before this the removal ran only where the expression had nothing
// to expand, so the first two rows were refusals here (#4255).
func TestAWrittenArithmeticsDoubleQuotesComeOffAfterItIsExpanded(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a quoted subscript runs",
			`declare -A m; k='x],b['; m[$k]=1; (( "m[$k]++" )); printf '[%s]' "${m[$k]}"`,
			"[2]",
		},
		{
			"a quoted numeral joins the one beside it",
			`i=1; printf '[%s]' "$(( 1"0" + $i ))"`,
			"[11]",
		},
		{
			"a quoted expression is the expression",
			`i=1; printf '[%s]' "$(( "1+$i" ))"`,
			"[2]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A double quote a *value* carried is not the script's and stays, which is
// what makes the removal above a statement about provenance.
func TestAnArrivedDoubleQuoteIsNotRemovedFromArithmeticHere(t *testing.T) {
	out, st := answersRun(t, `i=1; q='"'; echo $(( 1${q}0${q} + $i ))`)
	if !strings.Contains(out, `1"0" + 1`) || st == 0 {
		t.Errorf("= %q status %d, want a refusal naming `1\"0\" + 1`", out, st)
	}
}

// The bytes an expansion put inside brackets the script wrote are quoted back
// with a backslash in front of them.
//
// Measured 2026-09-22, same invocation, bash 5.3.20, with
// `declare -A assoc; key='x],b[$(echo 9)'; assoc[$key]=1`:
//
//	(( 'assoc[$key]++' ))   ((: 'assoc[x\],b\[\$(echo 9)]++' : arithmetic
//	                        syntax error: operand expected
//
// The apostrophes are no quoting in an arithmetic expression, so both shells
// refuse the line; what the escaping says is that the `]` the value carried
// closed no subscript, and the unescaped text this used to name says the key
// ended where it did not (#4255).
func TestAValuesBytesInAWrittenSubscriptAreQuotedBackEscaped(t *testing.T) {
	const src = `declare -A assoc; key='x],b[$(echo 9)'; assoc[$key]=1; (( 'assoc[$key]++' ))`
	out, st := answersRun(t, src)
	const want = `'assoc[x\],b\[\$(echo 9)]++'`
	if !strings.Contains(out, want) || st == 0 {
		t.Errorf("= %q status %d, want a refusal naming %s", out, st, want)
	}
}

// The escaping is a diagnostic choice and this dialect is the one that makes
// it, so the value is pinned rather than inherited.
func TestTheEscapedArithmeticValueIsThisDialectsChoice(t *testing.T) {
	if got := bash.Diagnostics().ArithValueShownEscaped; !got {
		t.Errorf("ArithValueShownEscaped = %v, want true", got)
	}
}
