// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether a quotation inside a subscript that arrived already expanded stops
// the expansion it holds — which is Semantics.SubscriptIsAQuotingContext read
// at the second reading of a subscript, and not an axis of its own.
//
// `e="m['$kq']"; $(( $e ))` has brackets that came out of `$e`, so the
// subscript is read a second time. Where the subscript is a quoting context
// the apostrophes stop the `$kq` and are then removed, and the key is the
// three characters `$kq`; where it is not, they stop nothing and stay, and
// the key is `'q'`.
//
// Measured 2026-09-20 over a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// storing through the re-read subscript and listing the table's key:
//
//	text in the brackets  bash 5.3.20  ksh93u+  zsh 5.9.2
//	`'$kq'`               `$kq`        `$kq`    `'q'`
//	`"$kq"`               `q`          `q`      `"q"`
//	`$kq`                 `q`          `q`      `q`
//
// bash and ksh93 answer the axis yes and zsh no, and the rows part exactly
// there. The last two are the controls: a double quotation performs what it
// holds in every column, and an unquoted expansion is performed in every
// column, so neither moves with the reading and only the first row does.
func quotationRun(t *testing.T, src string, quoting interp.Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *interp.Semantics) {
		s.SubscriptIsAQuotingContext = quoting
		// Every row here holds an **expansion** inside the brackets, and
		// that is what decides the other quoting axis out of the question:
		// a subscript still holding one is unmarked before it is read
		// again, so the two answers cannot part on these rows. Answered
		// the harder way — the marking on — so that they have to prove it.
		// See Semantics.ArrivedSubscriptIsAQuotingContext.
		s.ArrivedSubscriptIsAQuotingContext = interp.No
	})
}

func TestAQuotationInAReReadSubscriptStopsTheExpansionItHolds(t *testing.T) {
	// The table holds the key the expansion would produce and the key the
	// quotation leaves, so every row tells the two readings apart by which
	// element it finds rather than by a count.
	const table = `typeset -A m; kq=q; m[q]=7; m['$kq']=5; `
	for _, tc := range []struct{ name, src, quoting, asWritten string }{
		{
			"an apostrophe around the expansion",
			`e="m['\$kq']"; printf "[%s]" "$(( $e ))"`,
			`[5]`, `[0]`,
		},
		{
			// The control that makes this one question rather than two: the
			// same expansion, in the same brackets, reached by the same
			// re-read, and a double quotation performs it under both
			// readings. What the two readings do differ about there is the
			// quote characters, which is the axis's other half.
			"a double quotation around it",
			`e='m["$kq"]'; printf "[%s]" "$(( $e ))"`,
			`[7]`, `[0]`,
		},
		{
			// And the control that says the re-read itself is not in doubt.
			"no quotation at all",
			`e='m[$kq]'; printf "[%s]" "$(( $e ))"`,
			`[7]`, `[7]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := quotationRun(t, table+tc.src, interp.Yes); out != tc.quoting {
				t.Errorf("a quoting context: = %q, want %q", out, tc.quoting)
			}
			if out, _ := quotationRun(t, table+tc.src, interp.No); out != tc.asWritten {
				t.Errorf("taken as written: = %q, want %q", out, tc.asWritten)
			}
		})
	}
}
