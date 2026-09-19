// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestTheGetoptsBadNameKeepsNeitherTheBuiltinNorTheLine is #3555's location.
//
// This shell's bad-name refusals carry the builtin and the line — `unset 1bad`
// is `<file>[N]: unset: 1bad: invalid variable name` — and `getopts` carries
// neither. Measured 2026-09-18 against ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file, from `-c` and from
// standard input:
//
//	a script file   x.sh: 1bad: invalid variable name        1
//	-c              /bin/ksh: 1bad: invalid variable name    1
//	standard input  /bin/ksh: 1bad: invalid variable name    1
//	inside f()      x.sh: 1bad: invalid variable name        1
//
// So the file half is the ordinary location's and only the count goes, which
// is what parts Diagnostics.BadNameRefusalOmitsTheLine from
// BuiltinBadNameNamesTheShellAlone next door: that one writes the name the
// *shell* was invoked by, and this shell writes the script's.
//
// And the script runs on: `echo A; getopts x 1bad -x; echo "B st=$?"` writes
// all three lines here, where zsh stops at the refusal.
func TestTheGetoptsBadNameKeepsNeitherTheBuiltinNorTheLine(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"echo A\ngetopts x 1bad -x\necho \"B st=$?\"")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "1bad: invalid variable name") {
		t.Errorf("said %q, want this shell's bad-name sentence", out)
	}
	if strings.Contains(out, "getopts:") {
		t.Errorf("said %q, want no builtin in the location", out)
	}
	// Neither spelling of a count: this shell's own is `[2]` and the shape
	// the ordinary path falls back to when the builtin is hidden is
	// `line 2`.
	if strings.Contains(out, "[2]") || strings.Contains(out, "line 2") {
		t.Errorf("said %q, want no line number in the location", out)
	}
	if !strings.Contains(out, "A\n") || !strings.Contains(out, "B st=1") {
		t.Errorf("said %q, want A, the refusal and B st=1 — the script runs on", out)
	}
	if st != 0 {
		t.Errorf("the script reported %d, want 0 — the refusal is not fatal here", st)
	}
	// The contrast that says the location is this builtin's and not the
	// dialect's: `unset` on the same shell keeps both.
	out, _, err = preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, "\nunset 1bad")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "unset: 1bad") || !strings.Contains(out, "[2]") {
		t.Errorf("unset said %q, want the builtin and the line in its location", out)
	}
}

// TestTheGetoptsNameOperandMayNameAnElement is #3555's subscript axis in this
// column, which is the one that answers it differently from bash.
//
// Measured 2026-09-18: `a=(p q r); getopts x 'a[1]' -x` is 0 here and leaves
// `p x r`, where bash — a shell with arrays that fills `read 'a[1]'` — refuses
// the same operand as a name.
func TestTheGetoptsNameOperandMayNameAnElement(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`a=(p q r); getopts x 'a[1]' -x; echo "st=$? a=[${a[*]}]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "st=0 a=[p x r]") {
		t.Errorf("said %q, want the element filled at 0", out)
	}
}
