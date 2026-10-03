// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheXFlagReportsWhatTheOthersPassOver pins `(X)`: a failure `(Q)`, `(e)`
// or `(#)` would pass over in silence is reported, and the line ends there
// (#5151, a chunk of D04parameter.ztst). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestTheXFlagReportsWhatTheOthersPassOver(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`foo='unmatched "'; print -r -- ${(QX)foo}; print next`, "zsh:1: unmatched \"\n"},
		{`foo="a'b"; print -r -- ${(QX)foo}; print next`, "zsh:1: unmatched '\n"},
		{`foo='$(x'; print -r -- ${(QX)foo}; print next`, "zsh:1: parse error in parameter value\n"},
		{`foo='${x'; print -r -- ${(QX)foo}; print next`, "zsh:1: closing brace expected\n"},
		{"foo='`x'; print -r -- ${(QX)foo}; print next", "zsh:1: unmatched `\n"},
		{`a=(x '"y'); print -r -- ${(@QX)a}; print next`, "zsh:1: unmatched \"\n"},
		{`foo=1+; print -r -- ${(X#)foo}; print next`, "zsh:1: bad math expression: operand expected at end of string\n"},
		{`foo=1/0; print -r -- ${(X#)foo}; print next`, "zsh:1: division by zero\n"},
		{`foo='$('; print -r -- ${(Xe)foo}; print next`, "zsh:1: parse error\n"},
		// The controls: what reads cleanly, and the same failures without
		// the flag.
		{`foo='a\'; print -r -- ${(QX)foo}; print next`, "a\nnext\n"},
		{`foo=ok; x=65; print -r -- ${(QX)foo} ${(X#)x} ${(X)foo}`, "ok A ok\n"},
		{`foo='unmatched "'; print -r -- ${(Q)foo}; print next`, "unmatched \"\nnext\n"},
		{`foo=1+; print -r -- "<${(#)foo}>"; print next`, "<>\nnext\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
