// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestASubstringRangeKeepsItsSingleQuotes — a single quotation in a range is
// evaluated with its quotes on, so `'1'` is refused rather than read as 1,
// while a double quotation comes off. Measured 2026-10-04 on bash 5.3.20, and
// unanimous across the panel; see semantics.md.
func TestASubstringRangeKeepsItsSingleQuotes(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{`x=abcdef; echo "[${x:'1'}]"`, "", `x: '1': arithmetic syntax error: operand expected (error token is "'1'")`},
		{`x=abc; echo "[${x:1+'&'}]"`, "", `x: 1+'&': arithmetic syntax error: operand expected (error token is "'&'")`},
		{`x=abcdef; echo "[${x:"1"}]"`, "[bcdef]\n", ""},
	} {
		out, errs, _ := runAlias(t, c.src)
		if out != c.out || !strings.Contains(errs, c.errs) {
			t.Errorf("%s: wrote %q and said %q, want %q and %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
