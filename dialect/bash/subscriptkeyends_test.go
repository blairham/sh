// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// No key ends early here, and the reason is one step back: this column does
// not perform the expansion an apostrophe holds at all, so there is nothing
// performed for a key to end at.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `declare -A m; kq=q`: `(( m[q'$kq'z] = 42 ))` is the whole of
// `q$kqz` here, where ksh93u+ 2012-08-01 keeps only `q`. See
// interp.Semantics.SubscriptQuotationEndsTheKey (#3968).
func TestNoKeyEndsAtAQuotedExpansionHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`(( m[q'$kq'z] = 42 ))`, "<q$kqz>"},
		{`(( m['$kq'z] = 42 ))`, "<$kqz>"},
		{`(( m[a'$kq'b'$kq'c] = 42 ))`, "<a$kqb$kqc>"},
	} {
		src := `declare -A m; kq=q; ` + tc.src +
			`; for k in "${!m[@]}"; do printf "<%s>" "$k"; done`
		out, st := answersRun(t, src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheKeyEndingIsAnAxis(t *testing.T) {
	if got := bash.Semantics().SubscriptQuotationEndsTheKey; got != interp.No {
		t.Errorf("SubscriptQuotationEndsTheKey = %v, want no", got)
	}
}
