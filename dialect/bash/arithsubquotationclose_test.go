// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A subscript whose quotation never closes is a bad subscript here, and not
// the key the three characters look like.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// over `typeset -A a; k="q'r"; a[$k]=4`: `let "++a[$k]"` writes
// `a[q'r]: bad array subscript` twice and leaves the element at 4, where
// ksh93u+ 2012-08-01 and zsh 5.9.2 both answer 5. `(( a[$k]++ ))` beside it
// is 5 in every column, which is what says the surface is the already
// word-expanded operand rather than the key. See
// interp.Semantics.ArithSubscriptQuotationMustClose (#3796).
func TestAnUnclosedQuotationInAnArithmeticSubscriptIsBad(t *testing.T) {
	out, st := answersRun(t, `declare -A a; k="q'r"; a[$k]=4; let "++a[$k]"; printf "[%s]" "${a[$k]}"`)
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if n := strings.Count(out, "a[q'r]: bad array subscript"); n != 2 {
		t.Errorf("= %q, want the sentence twice", out)
	}
	if !strings.HasSuffix(out, "[4]") {
		t.Errorf("= %q, want the element left at [4]", out)
	}
}

// And the same key reached through arithmetic's own expansion is not this
// question at all: the control that says the refusal is the giving-up scan
// and not the apostrophe.
func TestAnArithmeticExpandedKeyWithAQuoteIsNotRefused(t *testing.T) {
	out, st := answersRun(t, `declare -A a; k="q'r"; a[$k]=4; (( a[$k]++ )); printf "[%s]" "${a[$k]}"`)
	if out != "[5]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[5]")
	}
}

// The option a script turns the round off with reaches this surface too, and
// turns it into the reading the other two columns give.
//
// Measured in the same run: with `shopt -s assoc_expand_once` the probe above
// is 5 in bash 5.3.20 with no refusal at all.
func TestTheOptionWithholdsTheArithmeticSubscriptsRefusal(t *testing.T) {
	out, st := answersRun(t, `shopt -s assoc_expand_once; declare -A a; k="q'r"; a[$k]=4; let "++a[$k]"; printf "[%s]" "${a[$k]}"`)
	if out != "[5]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[5]")
	}
}

// The axis, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheUnclosedQuotationRefusalIsAnAxis(t *testing.T) {
	if got := bash.Semantics().ArithSubscriptQuotationMustClose; got != interp.Yes {
		t.Errorf("ArithSubscriptQuotationMustClose = %v, want yes", got)
	}
}
