// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`getopts` over an exported OPTARG and OPTIND** —
// Semantics.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters. Measured
// 2026-10-15 as a script file under `env -i PATH=/usr/bin:/bin`: the
// parameters read 6 and 2 in every column, and a child is handed `OPTARG=pre`
// and `OPTIND=1` by zsh 5.9.2 alone, `OPTARG=6` and `OPTIND=2` by bash
// 5.3.20, dash 0.5.12, ksh93u+ and BusyBox ash 1.37.0 (the pinned alpine
// image). The trailing `OPTIND=$OPTIND` is the control: an assignment
// refreshes the copy in all five (#6475).
func TestGetoptsRefreshesAnExportedParametersEnvironmentCopy(t *testing.T) {
	src := "PATH=/usr/bin:/bin\nexport OPTARG=pre OPTIND=1\ngetopts x: o -x6\necho $OPTARG $OPTIND\nenv | grep -E '^(OPTARG|OPTIND)=' | sort\nOPTIND=$OPTIND\nenv | grep -E '^OPTIND='\n"
	refreshed := "6 2\nOPTARG=6\nOPTIND=2\nOPTIND=2\n"
	for name, want := range map[string]string{
		"bash": refreshed,
		"dash": refreshed,
		"ash":  refreshed,
		"ksh":  refreshed,
		"zsh":  "6 2\nOPTARG=pre\nOPTIND=1\nOPTIND=2\n",
	} {
		out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if out != want {
			t.Errorf("%s: got %q, want %q", name, out, want)
		}
	}
}

// `getopts`'s name operand is the operand whatever it is spelled: measured
// 2026-10-15 on zsh 5.9.2, `set -a; getopts x: OPTARG -x6` hands a child
// `OPTARG=x`, marked as the name's store is rather than left alone as the
// parameter's own is, and over an exported OPTARG the same store refreshes the
// child's copy.
func TestGetoptsNameSpelledOPTARGIsTheOperand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"PATH=/usr/bin:/bin\nset -a\ngetopts x: OPTARG -x6\nset +a\nenv | grep -E '^OPTARG='\n", "OPTARG=x\n"},
		{"PATH=/usr/bin:/bin\nexport OPTARG=pre\ngetopts x: OPTARG -x6\nenv | grep -E '^OPTARG='\n", "OPTARG=x\n"},
	} {
		out, _, err := presets["zsh"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, out, tc.want)
		}
	}
}

// The copy zsh holds goes with the export: measured 2026-10-15 on zsh 5.9.2,
// `export OPTIND=1; getopts x: o -x6; typeset +x OPTIND` hands a child no
// OPTIND, and an `export OPTIND` after that hands it the shell's own 2 — where
// a bare `export OPTIND` with the export still on leaves it at 1.
func TestGetoptsHeldCopyGoesWithTheExport(t *testing.T) {
	src := "PATH=/usr/bin:/bin\nexport OPTIND=1\ngetopts x: o -x6\nexport OPTIND\nenv | grep -E '^OPTIND='\ntypeset +x OPTIND\nenv | grep -cE '^OPTIND='\nexport OPTIND\nenv | grep -E '^OPTIND='\n"
	out, _, err := presets["zsh"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "OPTIND=1\n0\nOPTIND=2\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
