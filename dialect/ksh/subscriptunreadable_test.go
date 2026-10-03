// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// ksh93 reads a subscript's whole text and refuses a byte it cannot read.
// Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c` (#5567). See
// interp.Semantics.SubscriptExpressionStopsAtAnUnreadableByte.
func TestAnUnreadableByteInASubscriptIsRefused(t *testing.T) {
	out, st := runKshEmptyArray(t, `a=(x y z); echo ${a[2@]}; echo after`)
	if !strings.Contains(out, "2@: arithmetic syntax error") || strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q at %d, want the refusal and nothing after", out, st)
	}
}
