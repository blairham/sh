// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestATildeIsExpandedAfreshOnEveryCall pins that a function body's tildes
// read `HOME` and the options as each call runs. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5155).
func TestATildeIsExpandedAfreshOnEveryCall(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			"HOME=/h; f(){ print ~ x=~ }; f; HOME=/y; f; setopt magicequalsubst; f; unsetopt magicequalsubst; HOME=/z; f",
			"/h x=~\n/y x=~\n/y x=/y\n/z x=~\n",
		},
		{
			"HOME=/h; setopt magicequalsubst kshtypeset; print -l var=~ split=$(echo maybe not) x$(echo a b)",
			"var=/h\nsplit=maybe not\nxa\nb\n",
		},
		{"setopt kshtypeset; print -l split=$(echo maybe not)", "split=maybe\nnot\n"},
		{"setopt magicequalsubst; print -l split=$(echo maybe not)", "split=maybe\nnot\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
