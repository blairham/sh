// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A subscript whose brackets arrived already word-expanded is the same
// quoting context a written one is here, which is the half ksh93 does not
// share.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `declare -A a; a[k]=1; a['"k"']=2`: both `(( ++a["k"] ))` and
// `let '++a["k"]'` leave the bare key at 2 and the quoted key at 2, where
// ksh93u+ 2012-08-01 answers the first the same way and the second by
// incrementing the four-character key instead. See
// The value route is the same answer and is the one that says the axis is
// about arrival rather than about the builtin: `e="a[q'r'z]"; (( ++$e ))`
// reaches the bare key here and the five-character one in ksh93u+. See
// interp.Semantics.ArrivedSubscriptIsAQuotingContext (#3871, #3917).
func TestAnArrivedSubscriptIsAQuotingContextHere(t *testing.T) {
	const read = `; printf "[%s][%s]" "${a[k]}" "${a[$q]}"`
	// The quoted key is stored and read back through a value, because a
	// value's quote characters are characters and not quoting — so the two
	// spellings compared are the ones the operand wrote.
	const quoted = `declare -A a; a[k]=1; q='"k"'; a[$q]=2; `
	for _, tc := range []struct{ name, src string }{
		{"through the builtin", quoted + `let '++a["k"]'`},
		{"through the expression", quoted + `(( ++a["k"] ))`},
		// And through a value, which no builtin brought: the brackets came
		// out of `$e` and the answer is the same one.
		{"through a value", quoted + `e='a["k"]'; (( ++$e ))`},
		// The backslash goes with the quotes rather than being spared,
		// which is what says this is quote removal.
		{"a backslash rather than a quote", `declare -A a; a[k]=1; q='\k'; a[$q]=2; let '++a[\k]'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src+read)
			if out != "[2][2]" || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, "[2][2]")
			}
		})
	}
}

// The shape a script meets: the quote characters arrive in a value, and the
// operand is word-expanded before the builtin sees it — so this column takes
// them off and reaches a key the table does not hold.
//
// Measured in the same run: `declare -A a; k="q'r'z"; a[$k]=4; let "++a[$k]"`
// leaves the element at 4 here, where ksh93u+ and zsh 5.9.2 both answer 5.
// `(( a[$k]++ ))` beside it is 5 in every column, the value's own quotes
// carrying marks there.
func TestAValuesQuotesAreRemovedFromALetOperandsSubscript(t *testing.T) {
	const table = `declare -A a; k="q'r'z"; a[$k]=4; `
	out, st := answersRun(t, table+`let "++a[$k]"; printf "[%s]" "${a[$k]}"`)
	if out != "[4]" || st != 0 {
		t.Errorf("let = %q status %d, want %q at 0", out, st, "[4]")
	}
	if out, st := answersRun(t, table+`(( a[$k]++ )); printf "[%s]" "${a[$k]}"`); out != "[5]" || st != 0 {
		t.Errorf("arithmetic = %q status %d, want %q at 0", out, st, "[5]")
	}
}

// The store through a bad subscript is refused in its own words here, which
// is a second sentence beside the read's and is written per store.
//
// Measured in the same run over `declare -A a; k="q'r"; a[$k]=4`:
//
//	let "a[$k] = 9"      ``let: `a[q'r]': not a valid identifier`` once,
//	                     and no `bad array subscript` at all
//	let "x = a[$k] + 1"  `a[q'r]: bad array subscript` twice, and this
//	                     sentence never
//	let "++a[$k]"        the read's twice and then this one — three lines
//
// The element is 4 and the status 0 in all three. See
// interp.Diagnostics.ArithSubscriptUnclosedQuoteTarget (#3870).
func TestAStoreThroughABadSubscriptIsRefusedAsAnOperand(t *testing.T) {
	const table = `declare -A a; k="q'r"; a[$k]=4; `
	for _, tc := range []struct {
		name          string
		expr          string
		reads, stores int
	}{
		{"a store alone", `let "a[$k] = 9"`, 0, 1},
		{"a read alone", `let "x = a[$k] + 1"`, 2, 0},
		{"an increment", `let "++a[$k]"`, 2, 1},
		{"an assignment operator", `let "a[$k] += 1"`, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, table+tc.expr+`; printf "[%s]" "${a[$k]}"`)
			if n := strings.Count(out, "a[q'r]: bad array subscript"); n != tc.reads {
				t.Errorf("= %q, want %d read refusals", out, tc.reads)
			}
			if n := strings.Count(out, "let: `a[q'r]': not a valid identifier"); n != tc.stores {
				t.Errorf("= %q, want %d store refusals", out, tc.stores)
			}
			if !strings.HasSuffix(out, "[4]") || st != 0 {
				t.Errorf("= %q status %d, want the element left at [4] at 0", out, st)
			}
		})
	}
}

// The axis, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheArrivedSubscriptQuotingIsAnAxis(t *testing.T) {
	if got := bash.Semantics().ArrivedSubscriptIsAQuotingContext; got != interp.Yes {
		t.Errorf("ArrivedSubscriptIsAQuotingContext = %v, want yes", got)
	}
}
