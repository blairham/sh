// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An indexed subscript keeps its apostrophes and backslashes for the
// arithmetic, which refuses them and ends the script; a double quotation is
// quoting. Measured 2026-10-03 on zsh 5.9.2 under `-f -c` with `a=(x y z)`
// (#5562). The wording here is the arithmetic's own; zsh's subscript words
// it differently, which is #5567.
func TestAQuotedIndexedSubscriptIsRefused(t *testing.T) {
	const arr = "a=(x y z); "
	for _, c := range []struct{ src, out string }{
		{"echo ${a['2']}; echo after", ""},
		{"a['1']=Q; echo after", ""},
		{"echo ${#a['1']}; echo after", ""},
		{`echo ${a[\1]}; echo after`, ""},
		{`echo ${a["2"]}; a["1"]=Q; i=1; echo ${a[$i]} ${a["$i"]}`, "y\nQ Q\n"},
	} {
		out, st := runZsh(t, t.TempDir(), arr+c.src)
		if c.out != "" {
			if out != c.out || st != 0 {
				t.Errorf("%s\n got %q at %d, want %q at 0", c.src, out, st, c.out)
			}
			continue
		}
		if !strings.Contains(out, "bad math expression") || strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s\n got %q at %d, want a math refusal and nothing after", c.src, out, st)
		}
	}
}
