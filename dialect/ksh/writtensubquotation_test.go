// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// An apostrophe a script *writes* between a subscript's brackets does not
// stop the expansion it holds here, and this is the column that makes the
// written spelling its own question.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; kq=q`: `(( m['$kq'] = 42 ))` stores under `q` here,
// where bash 5.3.20 stores under the three characters `$kq` — and the same
// brackets reached through a value store under `$kq` in *both* columns. So
// the written spelling and the arrived one are two answers, and only this
// column holds different ones. See
// interp.Semantics.WrittenSubscriptQuotationStopsItsExpansion (#3942).
func TestAWrittenSubscriptsApostropheIsPerformedHere(t *testing.T) {
	const read = `; printf "[%s][%s]" "${m[$kq]}" "${m[$a]}"`
	const table = `typeset -A m; kq=q; a='$kq'; `
	for _, tc := range []struct{ name, src, want string }{
		{"an apostrophe", table + `(( m['$kq'] = 42 ))`, "[42][]"},
		// The control: a double quotation is performed in every column.
		{"a double quotation", table + `(( m["$kq"] = 42 ))`, "[42][]"},
		// The discriminating row. The same characters through a value are
		// stopped here as well as in bash, which is why the written
		// spelling needed an axis of its own.
		{"through a value", table + `e="m['\$kq']"; (( $e = 42 ))`, "[][42]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src+read)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheWrittenSubscriptQuotationIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().WrittenSubscriptQuotationStopsItsExpansion; got != interp.No {
		t.Errorf("WrittenSubscriptQuotationStopsItsExpansion = %v, want no", got)
	}
}
