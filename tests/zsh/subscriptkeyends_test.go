// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// No key ends early here, and this column *does* perform the expansion an
// apostrophe holds — so the answer is a reading of its own rather than a pin
// on the axis beside it.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; kq=q`: `(( m[q'$kq'z] = 42 ))` is the key `q'q'z` here
// — the expansion performed, the quotation kept and nothing dropped — where
// ksh93u+ 2012-08-01 keeps only `q`. See
// interp.Semantics.SubscriptQuotationEndsTheKey (#3968).
func TestNoKeyEndsAtAQuotedExpansionHere(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m[q'$kq'z] = 42 )); ` +
		`for k in "${(k)m[@]}"; do printf "<%s>" "$k"; done`
	out, st := answersRun(t, src)
	if out != "<q'q'z>" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "<q'q'z>")
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheKeyEndingIsAnAxis(t *testing.T) {
	if got := zsh.Semantics().SubscriptQuotationEndsTheKey; got != interp.No {
		t.Errorf("SubscriptQuotationEndsTheKey = %v, want no", got)
	}
}
