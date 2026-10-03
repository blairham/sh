// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAKeywordOutranksAFunctionOfItsName: measured 2026-10-02 on ksh93u+
// 2012-08-01, `whence -v time` with a function of that name defined is a
// keyword (#5267).
func TestAKeywordOutranksAFunctionOfItsName(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), "function time { :; }; whence -v time\n")
	if want := "time is a keyword\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
