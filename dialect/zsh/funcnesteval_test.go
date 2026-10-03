// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestFuncnestRefusalInsideEvalNamesTheCallee pins where the FUNCNEST
// refusal is located when the call is `eval` text. Measured 2026-10-02 on
// zsh 5.9.2 under `-f` (#5148).
func TestFuncnestRefusalInsideEvalNamesTheCallee(t *testing.T) {
	cases := []struct{ src, want string }{
		{
			"eval '(\nFUNCNEST=0\nfn() { true; }\nfn\n)'",
			"fn:4: maximum nested function level reached; increase FUNCNEST?\n",
		},
		{
			"f(){ eval 'FUNCNEST=1\ng(){ :; }\ng' }; f",
			"g:3: maximum nested function level reached; increase FUNCNEST?\n",
		},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
