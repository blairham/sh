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

// What a plain `unset NAME` does to a *function*, in all five dialects —
// #3205, where this shell answered "nothing" in every one of them.
//
// Measured 2026-09-16 with a call after the `unset` whose status is printed,
// both streams captured separately and a marker after it, so the table says
// whether the script continued as well as what it wrote. Every cell continued
// and every cell was silent, which is what makes the status the whole answer:
//
//	shell                          unset b; b   unset c (c=val too); c
//	bash 5.3.20, as-`sh`, 3.2.57   127          the variable goes, `c` runs
//	zsh 5.9.2                      runs, 0      the variable goes, `c` runs
//	ksh93u+ 2012-08-01             runs, 0      the variable goes, `c` runs
//	dash 0.5.12                    runs, 0      the variable goes, `c` runs
//	BusyBox ash 1.37.0             runs, 0      the variable goes, `c` runs
//
// One column against six, so the second half of that table is *not* the
// discriminating one — it is the control, and it is here because without it a
// shell that never reached the function table would still pass the four rows
// where the right answer is "the function runs".
//
// This is the end-to-end half of the guard and it is not redundant beside the
// per-preset answers tables or beside `share/suite/bash/functions.tests`. The
// suite rows carry the column that reaches the table, and a per-dialect tier
// is run only under the reference it claims to be alone against — so the four
// columns that must *not* reach it have no suite row that could see them, and
// a preset holding the right value while the builtin ignored it would pass
// every table in the tree.
func unsetFunctionTablePresets() []struct {
	dialecttest.Preset
	// reaches is whether a plain `unset` over a name with a function and no
	// parameter takes the function away.
	reaches bool
} {
	return []struct {
		dialecttest.Preset
		reaches bool
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, true},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, false},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, false},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, false},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, false},
	}
}

func TestEachDialectAnswersAPlainUnsetOverAFunction(t *testing.T) {
	for _, p := range unsetFunctionTablePresets() {
		t.Run(p.Name, func(t *testing.T) {
			// The disagreement: a function and no parameter of that name.
			gone := "b() { printf 'orig\\n'; }\nunset b\nb 2>/dev/null\n" +
				"printf 'call=%s\\n' \"$?\"\nprintf 'CONTINUED\\n'\n"
			want := "orig\ncall=0\nCONTINUED\n"
			if p.reaches {
				want = "call=127\nCONTINUED\n"
			}
			if out, st, err := p.Combined(t, dialecttest.Base{}, gone); err != nil {
				t.Fatalf("err %v: %s", err, out)
			} else if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q at 0", out, st, want)
			}

			// The controls, and they answer alike in all five. The first is
			// the parameter table doing its own job — one `unset` takes the
			// variable and the function is still there, everywhere — so a
			// column that simply always removed the function would fail it.
			for _, c := range []struct{ name, src, want string }{
				{
					"a variable takes the first turn",
					"c() { printf 'c fn\\n'; }\nc=val\nunset c\n" +
						"printf 'c=[%s]\\n' \"${c-UNSET}\"\nc 2>/dev/null\n" +
						"printf 'call=%s\\n' \"$?\"\n",
					"c=[UNSET]\nc fn\ncall=0\n",
				},
				{
					// `-v` names the parameter namespace and reaches no
					// function anywhere, bash included.
					"-v reaches no function",
					"d() { printf 'd fn\\n'; }\nunset -v d\n" +
						"printf 'st=%s\\n' \"$?\"\nd 2>/dev/null\n" +
						"printf 'call=%s\\n' \"$?\"\n",
					"st=0\nd fn\ncall=0\n",
				},
				{
					// `unset -f` is the spelling every column has, so it is
					// the row that says the function table is reachable at
					// all in the four that refuse the plain one.
					"-f removes it everywhere",
					"e() { printf 'e fn\\n'; }\nunset -f e\ne 2>/dev/null\n" +
						"printf 'call=%s\\n' \"$?\"\n",
					"call=127\n",
				},
				{
					"a name with neither is quiet",
					"unset nothing_at_all_zz\nprintf 'st=%s\\n' \"$?\"\n",
					"st=0\n",
				},
			} {
				out, st, err := p.Combined(t, dialecttest.Base{}, c.src)
				if err != nil {
					t.Fatalf("%s: err %v: %s", c.name, err, out)
				}
				if out != c.want || st != 0 {
					t.Errorf("%s: = %q (status %d), want %q at 0", c.name, out, st, c.want)
				}
			}
		})
	}
}
