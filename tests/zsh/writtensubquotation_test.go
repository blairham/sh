// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// An apostrophe a script writes between a subscript's brackets does not stop
// the expansion it holds here either, and the apostrophes then stay in the
// key because no subscript of this dialect is a quoting context.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; kq=q`: `(( m['$kq'] = 42 ))` stores under the three
// characters `'q'` — the expansion performed and the quotation kept — where
// bash 5.3.20 stores under `$kq` and ksh93u+ 2012-08-01 under `q`. See
// interp.Semantics.WrittenSubscriptQuotationStopsItsExpansion (#3942).
func TestAWrittenSubscriptsApostropheIsPerformedAndKeptHere(t *testing.T) {
	const src = `typeset -A m; kq=q; b="'q'"; (( m['$kq'] = 42 )); printf "[%s][%s]" "${m[$kq]}" "${m[$b]}"`
	out, st := answersRun(t, src)
	if out != "[][42]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[][42]")
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from. Not a pin on another axis: this one is reachable here, and the row
// above is what reaches it.
func TestTheWrittenSubscriptQuotationIsAnAxis(t *testing.T) {
	if got := zsh.Semantics().WrittenSubscriptQuotationStopsItsExpansion; got != interp.No {
		t.Errorf("WrittenSubscriptQuotationStopsItsExpansion = %v, want no", got)
	}
}
