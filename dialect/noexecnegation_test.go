// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// What `sh -n` exits with over a file whose top-level command is negated, in
// all five dialects — the split #3179 was filed from, where this shell
// answered 1 in every one of them.
//
// `set -n` reads and runs nothing, so the status a `!` would invert was
// reported by no command. Measured 2026-09-16, `env -i PATH=/usr/bin:/bin
// LC_ALL=C <shell> -n x.sh`, with the two streams captured separately —
// **every cell wrote nothing on either stream**, so the status is the whole
// of the answer and this table is only readable because both halves are here:
//
//	shell                          ! true   true; ! true   ! true | cat
//	bash 5.3.20, as-`sh`, 3.2.57   0        0              0
//	ksh93u+ 2012-08-01             0        0              0
//	dash 0.5.12                    0        0              0
//	BusyBox ash 1.37.0             0        0              0
//	zsh 5.9 and 5.9.2              1        1              1
//
// The same six columns against the same one on `-c`, on stdin and on `-s`:
// the route does not move it.
//
// This is the end-to-end half of the guard and it is not redundant beside the
// per-preset answers tables. `share/suite/zsh/options.tests` carries the rows
// for the column that inverts, but a per-dialect suite tier is run only under
// the reference it claims to be alone against — so the four columns this
// defect was *wrong* in have no suite row that could see it, and a preset
// holding the right value while the runner ignored it would pass every table
// in the tree.
func noexecNegationPresets() []struct {
	dialecttest.Preset
	want int
} {
	return []struct {
		dialecttest.Preset
		want int
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, 0},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, 0},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, 0},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, 0},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, 1},
	}
}

func TestEachDialectAnswersAnUnrunNegation(t *testing.T) {
	for _, p := range noexecNegationPresets() {
		t.Run(p.Name, func(t *testing.T) {
			for _, src := range []string{
				"set -n\n! true\n",
				"set -n\ntrue; ! true\n",
				"set -n\n! true | cat\n",
			} {
				out, st, err := p.Combined(t, dialecttest.Base{}, src)
				if err != nil {
					t.Fatalf("%q: err %v: %s", src, err, out)
				}
				if out != "" || st != p.want {
					t.Errorf("%q wrote %q at status %d, want nothing at status %d",
						src, out, st, p.want)
				}
			}
			// The controls, and they answer alike in every column: two
			// negations invert back onto 0, `set -n` never walks into the
			// compound that holds the third, and the fourth is not negated
			// at all. Without them a column that simply always answered 0
			// would pass four of the five rows above.
			for _, src := range []string{
				"set -n\n! true\n! true\n",
				"set -n\nif ! true; then :; fi\n",
				"set -n\nfalse\n",
			} {
				out, st, err := p.Combined(t, dialecttest.Base{}, src)
				if err != nil {
					t.Fatalf("%q: err %v: %s", src, err, out)
				}
				if out != "" || st != 0 {
					t.Errorf("%q wrote %q at status %d, want nothing at status 0",
						src, out, st)
				}
			}
		})
	}
}
