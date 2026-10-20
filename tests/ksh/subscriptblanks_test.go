// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `a[ @ ]=x` is arithmetic and not the whole-array spelling: the blanks are
// part of the subscript (#6866). Measured 2026-10-16 on ksh93u+ 2012-08-01
// from a file.
func TestABlankedAtSignIsArithmetic(t *testing.T) {
	if out, _ := answersRun(t, "a=(1 2); a[ @ ]=x; echo st=$?\n"); out != "sh:  @ : arithmetic syntax error\n" {
		t.Errorf("got %q", out)
	}
}
