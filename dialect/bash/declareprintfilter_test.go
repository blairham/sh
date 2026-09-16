// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `-p` beside an attribute letter and no operand is the *filtered* listing.
//
// Measured 2026-09-15 over a table holding one integer, one readonly, one
// export and one plain scalar, each shell counting the rows of its own
// `typeset -p -i` — bash 5.3.20 nine, bash 3.2.57 eight, zsh 5.9.2 nineteen,
// ksh93u+ ten, and every one of them the integer names alone. Unanimous, so
// it is the core's and not an axis.
//
// This wrote the whole table instead: `declare -pi` listed every variable the
// shell had, at status 0, where the letter asked for the integer ones. The
// excess side of a silent wrong answer — nothing said the listing had been
// asked to narrow and had not.
func TestDashPBesideAnAttributeLetterFiltersTheListing(t *testing.T) {
	const setup = `n=1; declare -i i=2; declare -r r=3; export x=4; `
	for _, c := range []struct {
		name string
		src  string
		want []string
		deny []string
	}{{
		"the integer letter selects the integer names",
		setup + `declare -pi`,
		[]string{`declare -i i="2"`},
		[]string{`n="1"`, `r="3"`, `x="4"`},
	}, {
		"the readonly letter selects the frozen ones",
		setup + `declare -pr`,
		[]string{`declare -r r="3"`},
		[]string{`declare -- n="1"`, `declare -i i="2"`},
	}, {
		"the export letter selects the exported ones",
		setup + `declare -px`,
		[]string{`declare -x x="4"`},
		[]string{`declare -- n="1"`, `declare -i i="2"`, `declare -r r="3"`},
	}, {
		// The letters on their own already filtered; the point of the row is
		// that `-p` does not widen what they select.
		"and the letter without -p selects the same names",
		setup + `declare -i`,
		[]string{`declare -i i="2"`},
		[]string{`declare -- n="1"`},
	}, {
		// Not a filter with an operand, in any shell on the panel: bash
		// writes `declare -- n="1"` at 0 for a name carrying no integer
		// attribute, so the operand form is the plain print.
		"an operand is printed whatever letter stands beside it",
		setup + `declare -pi n; echo "st=$?"`,
		[]string{`declare -- n="1"`, "st=0"},
		nil,
	}, {
		// And `-p` alone is still the whole table.
		"and -p alone narrows nothing",
		setup + `declare -p`,
		[]string{`declare -- n="1"`, `declare -i i="2"`, `declare -r r="3"`, `declare -x x="4"`},
		nil,
	}} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := bashRun(t, c.src)
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("got %q (stderr %q), want a line %q", out, errs, want)
				}
			}
			for _, deny := range c.deny {
				if strings.Contains(out, deny) {
					t.Errorf("got %q, which the letter should not have selected: %q", out, deny)
				}
			}
		})
	}
}
