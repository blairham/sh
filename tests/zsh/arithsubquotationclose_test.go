// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A subscript whose quotation never closes is read as the key it looks like
// here, with ksh93 and against bash.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device:
// with `typeset -A a; k="q'r"; a[$k]=4`, `let "++a[$k]"` leaves 5 under the
// three-character key, where bash 5.3.20 writes `a[q'r]: bad array
// subscript` twice and leaves 4.
//
// A **pin and not a control**, and labeled as one: this dialect reads no
// quoting inside a subscript at all — syntax.Dialect.ArithSubscriptQuoting
// is off here — so no scan ever has a quotation to give up on and the axis
// cannot be reached from this preset in either answer. It is answered rather
// than left open for the reason
// Semantics.ArithWholeArraySubscriptIsReportedAsBad is answered here: an
// unanswered axis is a refusal, and a reading a dialect cannot reach must
// not be able to produce one. See
// interp.Semantics.ArithSubscriptQuotationMustClose (#3796).
func TestTheUnclosedQuotationRefusalIsAnswered(t *testing.T) {
	if got := zsh.Semantics().ArithSubscriptQuotationMustClose; got != interp.No {
		t.Errorf("ArithSubscriptQuotationMustClose = %v, want no", got)
	}
}
