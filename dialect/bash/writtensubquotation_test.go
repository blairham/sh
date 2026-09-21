// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// An apostrophe a script *writes* between a subscript's brackets stops the
// expansion it holds here, alone in the panel.
//
// Measured 2026-09-20 against bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `declare -A m; kq=q`: `(( m['$kq'] = 42 ))` leaves the three-character
// key `$kq`, where ksh93u+ 2012-08-01 stores under `q` and zsh 5.9.2 under
// `'q'`. See interp.Semantics.WrittenSubscriptQuotationStopsItsExpansion
// (#3942).
func TestAWrittenSubscriptsApostropheStopsItsExpansionHere(t *testing.T) {
	const read = `; printf "[%s][%s]" "${m[$kq]}" "${m[$a]}"`
	const table = `declare -A m; kq=q; a='$kq'; `
	for _, tc := range []struct{ name, src, want string }{
		// Stopped, so the key is the three characters the apostrophes held.
		{"an apostrophe", table + `(( m['$kq'] = 42 ))`, "[][42]"},
		// A double quotation performs what it holds, which is the control
		// that says this is the apostrophe and not quoting in general.
		{"a double quotation", table + `(( m["$kq"] = 42 ))`, "[42][]"},
		// And an apostrophe inside a double quotation stops nothing: the
		// double quotation is the one in charge.
		{"an apostrophe inside one", table + `(( m["'$kq'"] = 42 ))`, "[][]"},
		// Reached through a value the apostrophe stops it too, which is the
		// other axis and agrees here — so this column cannot tell the two
		// apart on its own. `dialect/ksh` is where they part.
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

// The expansion is not performed rather than performed and discarded, so a
// command substitution an apostrophe holds is a command that does not run.
func TestAStoppedSubscriptExpansionRunsNothingHere(t *testing.T) {
	out, st := answersRun(t, `declare -A m; ran=no; (( m['$(ran=yes; echo x)'] = 1 )); printf "[%s]" "$ran"`)
	if out != "[no]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[no]")
	}
}

// The axis, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheWrittenSubscriptQuotationIsAnAxis(t *testing.T) {
	if got := bash.Semantics().WrittenSubscriptQuotationStopsItsExpansion; got != interp.Yes {
		t.Errorf("WrittenSubscriptQuotationStopsItsExpansion = %v, want yes", got)
	}
}
