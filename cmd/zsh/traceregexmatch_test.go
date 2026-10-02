// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestATracedRegexMatchIsWrittenAsTheScriptWroteIt pins the trace of `=~`:
// the condition module's spelling, `-regex-match`, with both operands as
// written — through the front end, which is what hands the runner the text
// the words were read from. Measured 2026-10-02 on zsh 5.9.2 under `-f`
// (#5347).
func TestATracedRegexMatchIsWrittenAsTheScriptWroteIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -x; [[ a =~ "f o" ]]`, `+zsh:1> [[ a -regex-match "f o" ]]` + "\n"},
		{`set -x; [[ a =~ x\ y ]]`, `+zsh:1> [[ a -regex-match x\ y ]]` + "\n"},
		{`set -x; [[ a =~ ^a$ ]]`, `+zsh:1> [[ a -regex-match ^a$ ]]` + "\n"},
		{`v="p q"; set -x; [[ $v =~ $v ]]`, `+zsh:1> [[ $v -regex-match $v ]]` + "\n"},
		{`set -x; eval '[[ a =~ "f o" ]]'`, "+zsh:1> eval '[[ a =~ \"f o\" ]]'\n" + `+(eval):1> [[ a -regex-match "f o" ]]` + "\n"},
		// The control: `==` beside it traces the values.
		{`v="p q"; set -x; [[ $v == "$v" ]]`, `+zsh:1> [[ 'p q' == p\ q ]]` + "\n"},
	} {
		if got := runZshAs(t, "zsh", "-fc", tc.src); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
