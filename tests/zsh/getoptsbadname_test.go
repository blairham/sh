// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestABadGetoptsNameEndsTheScript is #3555's cost in the one column that
// stops.
//
// Measured 2026-09-18 against zsh 5.9.2 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, from a script file with stdin on /dev/null:
//
//	echo A; getopts x 1bad -x; echo "B st=$?"
//	  A
//	  <file>:2: not an identifier: 1bad
//	  — and the shell ends at 1
//
// The other six columns reach B. This shell stops here exactly as it stops at
// `read 1bad` and at `printf -v 1bad`, which is why the fatality is a field of
// its own rather than one shared with them.
//
// The location carries no `getopts`, where `read` and `printf -v` in the same
// shell are `<file>:read:N:` and `<file>:printf:N:` —
// Diagnostics.BadNameRefusalHidesTheBuiltin, which `set -A` already reaches.
func TestABadGetoptsNameEndsTheScript(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"echo A\ngetopts x 1bad -x\necho \"B st=$?\"")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "not an identifier: 1bad") {
		t.Errorf("said %q, want this shell's bad-name sentence", out)
	}
	if strings.Contains(out, "B st=") {
		t.Errorf("said %q, want the script to stop at the refusal", out)
	}
	if strings.Contains(out, "getopts:") {
		t.Errorf("said %q, want no builtin in the location", out)
	}
	if st == 0 {
		t.Errorf("the script reported 0, want a failure")
	}
}

// TestTheGetoptsNameOperandMayNameAnElementHere is the subscript axis in the
// second column that answers it Yes.
//
// Measured 2026-09-18: `getopts x 'o[1]' -x` is 0 with the letter in `$o`,
// where `getopts x 'o[0]' -x` is `o: assignment to invalid subscript range` —
// this shell's one-based arrays refusing the *position* rather than the
// brackets. A probe that asked only the `o[0]` spelling would have read this
// column as a refusal and set the axis the other way.
func TestTheGetoptsNameOperandMayNameAnElementHere(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`a=(p q r); getopts x 'a[2]' -x; echo "st=$? a=[${a[*]}]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "st=0 a=[p x r]") {
		t.Errorf("said %q, want the element filled at 0", out)
	}
}
