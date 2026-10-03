// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestAParseErrorNamesTheTokenWithoutItsContinuations pins that a parse error
// quoting a token names it with the backslash-newlines the lexer stepped over
// taken out, up to the opener of a substitution, and up to the first newline
// (#5151, a chunk of D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2
// under `-fc`.
func TestAParseErrorNamesTheTokenWithoutItsContinuations(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"$\\\n(", "zsh:2: parse error near `$('\n"},
		{"echo $\\\n((", "zsh:2: parse error near `$(('\n"},
		{"v=a\\\nb$\\\n(echo", "zsh:3: parse error near `v=ab$(echo'\n"},
		{"if a\\\nb", "zsh:2: parse error near `ab'\n"},
		{"(echo 'a\\\nb'", "zsh:2: parse error near `'a\\'\n"},
		// The body of the substitution is named as written.
		{"echo $(ec\\\nho", "zsh:2: parse error near `$(ec\\'\n"},
	} {
		_, errs, _ := runZsh(t, "-fc", tc.src)
		if errs != tc.want {
			t.Errorf("%q\n got %q\nwant %q", tc.src, errs, tc.want)
		}
	}
}
