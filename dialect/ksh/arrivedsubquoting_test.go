// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A subscript whose brackets arrived already word-expanded is not a quoting
// context here, where the same brackets a script wrote inside `(( … ))` are
// one — so this is the neighbor of ArithSubscriptQuotationMustClose and not a
// second spelling of it.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A a; a[k]=1; a['"k"']=2`: `(( ++a["k"] ))` leaves the bare
// key at 2, with bash 5.3.20, and `let '++a["k"]'` leaves the bare key at 1
// and the quoted key at 3. See
// The value route is the same answer, which is what says the axis is about
// arrival rather than about the builtin: measured in the same run,
// `typeset -A m; s="q'r'z"; e="m[$s]"; (( $e = 42 ))` stores under the five
// characters here and under `qrz` in bash 5.3.20. See
// interp.Semantics.ArrivedSubscriptIsAQuotingContext (#3871, #3917).
func TestAnArrivedSubscriptIsTakenAsWrittenHere(t *testing.T) {
	const read = `; printf "[%s][%s]" "${a[k]}" "${a[$q]}"`
	// The quoted key is stored and read back through a value, because a
	// value's quote characters are characters and not quoting — so the two
	// spellings compared are the ones the operand wrote.
	const quoted = `typeset -A a; a[k]=1; q='"k"'; a[$q]=2; `
	for _, tc := range []struct{ name, src, want string }{
		{"through the builtin", quoted + `let '++a["k"]'`, "[1][3]"},
		// The control that makes this a second axis rather than a rename of
		// the one the expression asks: the identical text one construct
		// over is the bare key here, as it is in the bash column.
		{"through the expression", quoted + `(( ++a["k"] ))`, "[2][2]"},
		// And through a value, which no builtin brought: the brackets came
		// out of `$e` and the answer is the builtin's rather than the
		// expression's.
		{"through a value", quoted + `e='a["k"]'; (( ++$e ))`, "[1][3]"},
		// The backslash is kept with the quotes rather than spared, so it
		// is quote removal that is not run rather than one character.
		{"a backslash rather than a quote", `typeset -A a; a[k]=1; q='\k'; a[$q]=2; let '++a[\k]'`, "[1][3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src+read)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The shape a script meets: the quote characters arrive in a value, and the
// operand is word-expanded before the builtin sees it — so nothing in it is
// marked, and this column keeps the two apostrophes as characters of the key.
//
// Measured in the same run: `typeset -A a; k="q'r'z"; a[$k]=4; let "++a[$k]"`
// leaves 5 here and in zsh 5.9.2, where bash 5.3.20 removes the quotation and
// leaves 4. `(( a[$k]++ ))` beside it is 5 in every column, the value's own
// quotes carrying marks there.
func TestAValuesQuotesSurviveALetOperandsSubscript(t *testing.T) {
	const table = `typeset -A a; k="q'r'z"; a[$k]=4; `
	out, st := answersRun(t, table+`let "++a[$k]"; printf "[%s]" "${a[$k]}"`)
	if out != "[5]" || st != 0 {
		t.Errorf("let = %q status %d, want %q at 0", out, st, "[5]")
	}
	if out, st := answersRun(t, table+`(( a[$k]++ )); printf "[%s]" "${a[$k]}"`); out != "[5]" || st != 0 {
		t.Errorf("arithmetic = %q status %d, want %q at 0", out, st, "[5]")
	}
}

// An **indexed** name puts neither question, because the brackets there are
// read as arithmetic rather than as a key: measured in the same run,
// `b=(10 20 30); let 'b["1"] = 9'` writes element one here and in bash 5.3.20
// alike.
func TestAnIndexedSubscriptInALetOperandIsUnmoved(t *testing.T) {
	out, st := answersRun(t, `b=(10 20 30); let 'b["1"] = 9'; printf "[%s]" "${b[*]}"`)
	if out != "[10 9 30]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[10 9 30]")
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheArrivedSubscriptQuotingIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().ArrivedSubscriptIsAQuotingContext; got != interp.No {
		t.Errorf("ArrivedSubscriptIsAQuotingContext = %v, want no", got)
	}
	// And the expression's own axis is unmoved, which is what makes the two
	// a split rather than one answer written twice.
	if got := ksh.Semantics().SubscriptIsAQuotingContext; got != interp.Yes {
		t.Errorf("SubscriptIsAQuotingContext = %v, want yes", got)
	}
}
