// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A subscript whose quotation never closes is read as the key it looks like
// here, where the bash column calls it a bad subscript.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device:
// with `typeset -A a; k="q'r"; a[$k]=4`, `let "++a[$k]"` leaves 5 under the
// three-character key, where bash 5.3.20 refuses and leaves 4. See
// interp.Semantics.ArithSubscriptQuotationMustClose (#3796).
func TestAnUnclosedQuotationInAnArithmeticSubscriptIsTheKey(t *testing.T) {
	out, st := answersRun(t, `typeset -A a; k="q'r"; a[$k]=4; let "++a[$k]"; printf "[%s]" "${a[$k]}"`)
	if out != "[5]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[5]")
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheUnclosedQuotationRefusalIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().ArithSubscriptQuotationMustClose; got != interp.No {
		t.Errorf("ArithSubscriptQuotationMustClose = %v, want no", got)
	}
}
