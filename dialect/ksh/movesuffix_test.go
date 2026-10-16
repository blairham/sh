// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A duplication's `-` is read off the expansion here, so one that arrives by
// expansion still moves: 9 is closed after. Measured 2026-10-16 on ksh93u+
// 2012-08-01 under `-c` (#6746), where bash refuses the same word. See
// interp.Semantics.FdMoveSuffixMustBeWritten.
func TestAMovesSuffixIsReadFromTheExpansion(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `x=9-; exec 9</dev/null; exec 4<&$x; echo st=$?; true 2>/dev/null <&9; echo "9:$?"`)
	if want := "st=0\n9:1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
