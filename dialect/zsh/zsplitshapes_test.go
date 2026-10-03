// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheShellWordSplitKeepsTheShellsTokens pins three shapes `(z)` splits
// the shell's way: an array assignment's parenthesis belongs to the
// assignment, an empty pair written together is one word, and the arithmetic
// `for` header is its parentheses and each expression (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestTheShellWordSplitKeepsTheShellsTokens(t *testing.T) {
	const setup = "z() { local -a w; w=(${(z)1}); print -rn -- \"${#w}:\"; for x in $w; print -rn -- \"<$x>\"; print }\n"
	for _, tc := range []struct{ src, want string }{
		{`z 'x=(a b)'`, "4:<x=(><a><b><)>\n"},
		{`z 'y+=(c)'; z 'a[1]=(x)'`, "3:<y+=(><c><)>\n3:<a[1]=(><x><)>\n"},
		{`z 'x= (a)'; z 'x=(a)y'`, "4:<x=><(><a><)>\n4:<x=(><a><)><y>\n"},
		{`z 'a=() b=(1)'`, "5:<a=(><)><b=(><1><)>\n"},
		{`z 'typeset x=(a b)'; z 'local -a y=(1)'`, "2:<typeset><x=(a b)>\n3:<local><-a><y=(1)>\n"},
		{`z 'f() {}'; z '() { :; }'`, "4:<f><()><{><}>\n5:<()><{><:><;><}>\n"},
		{`z 'for (( j = 0 ; j < 3 ; j++ )) do :; done'`, "10:<for><((><j = 0 ;><j < 3 ;><j++ ><))><do><:><;><done>\n"},
		{`z 'for ((i=0;i<3;i++))'; z 'for ((  ;  ; ))'`, "6:<for><((><i=0;><i<3;><i++><))>\n5:<for><((><;><;><))>\n"},
		// The controls: an arithmetic command and an arithmetic expansion
		// stay one word.
		{`z '(( 1 + 2 ))'; z 'echo $(( 1+1 ))'`, "1:<(( 1 + 2 ))>\n2:<echo><$(( 1+1 ))>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
