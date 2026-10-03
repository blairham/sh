// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An indexed subscript is read the way `$(( ))` text is: an apostrophe and a
// backslash stay characters of the expression, which refuses them, and a
// double quotation is quoting. Measured 2026-10-03 on bash 5.3.20 under `-c`
// with `a=(x y z)` (#5562). See interp.Semantics.IndexedSubscriptKeepsItsQuoting.
func TestAQuotedIndexedSubscriptIsArithmeticText(t *testing.T) {
	const arr = "a=(x y z); "
	for _, c := range []struct{ src, out, errs string }{
		{"echo ${a['2']}; echo after", "", `'2': arithmetic syntax error: operand expected (error token is "'2'")`},
		{"a['1']=Q; echo after", "", `'1': arithmetic syntax error: operand expected (error token is "'1'")`},
		{"b['1']=Q; echo after", "", `'1': arithmetic syntax error`},
		{"echo ${#a['1']}; echo after", "", `'1': arithmetic syntax error`},
		{`echo ${a[\1]}; echo after`, "", `\1: arithmetic syntax error`},
		{`echo ${a["2"]}; a["1"]=Q; i=1; echo ${a[$i]} ${a["$i"]}`, "z\nQ Q\n", ""},
		{"typeset -A m; m['k']=v; echo ${m[k]} ${m['k']}", "v v\n", ""},
	} {
		out, st := runBash(t, t.TempDir(), arr+c.src)
		if c.errs == "" {
			if out != c.out || st != 0 {
				t.Errorf("%s\n got %q at %d, want %q at 0", c.src, out, st, c.out)
			}
			continue
		}
		if !strings.Contains(out, c.errs) || strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s\n got %q at %d, want the refusal %q and nothing after", c.src, out, st, c.errs)
		}
	}
}
