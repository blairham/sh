// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestARangeKeepsABackslashThatDoubleQuotesWould pins bash's reading of a
// backslash in a substring range: removed only before the characters a
// double-quoted string removes it before (#5637). Measured 2026-10-03 on
// bash 5.3.20.
func TestARangeKeepsABackslashThatDoubleQuotesWould(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{`foo=abc; echo ${foo:0:\1}`, `(error token is "\1")`},
		{`foo=abc; echo "${foo:0:\1}"`, `(error token is "\1")`},
		{`foo=abc; echo ${foo:0:\"}`, `(error token is """)`},
	} {
		got, _ := runBash(t, t.TempDir(), tc.src)
		if !strings.Contains(got, tc.token) {
			t.Errorf("%s\n got %q\nwant it to name %s", tc.src, got, tc.token)
		}
	}
	if got, _ := runBash(t, t.TempDir(), `foo=abcdef; echo ${foo:1:2}`); got != "bc\n" {
		t.Errorf("control: got %q", got)
	}
}
