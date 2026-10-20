// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestTwoRuntimeRefusalsInBashsWords — a duplication target written as a
// plain number is named as that number, leading zeros gone, and a `=~`
// pattern that will not compile is refused with the construct, the pattern
// and regcomp's reason. Measured 2026-10-04 on bash 5.3.20.
func TestTwoRuntimeRefusalsInBashsWords(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo hi >&08; echo "st=$?"`, "line 1: 8: Bad file descriptor\n"},
		{`cat <&007`, "line 1: 7: Bad file descriptor\n"},
		{`x=08; echo hi >&$x`, "line 1: $x: Bad file descriptor\n"},
		{`p='['; [[ 'a[' =~ $p ]]; echo "st=$?"`, "line 1: [[: invalid regular expression `[': brackets ([ ]) not balanced\n"},
		{`p='a('; [[ a =~ $p ]]`, "line 1: [[: invalid regular expression `a(': parentheses not balanced\n"},
	} {
		_, errs, _ := runAlias(t, c.src)
		if !strings.HasSuffix(errs, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, errs, c.want)
		}
	}
}
