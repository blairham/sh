// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestARepeatCountIsAnArgumentWord pins that a brace beginning a `repeat`
// count is text rather than the reserved word: the count is one word, not
// brace-expanded, and its text reaches the arithmetic as written. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5382).
func TestARepeatCountIsAnArgumentWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"repeat {1,2} print y", "zsh:1: bad math expression: illegal character: {\n"},
		// And a count that fails ends the script, as a failed expansion does.
		{"repeat {} print y; print after", "zsh:1: bad math expression: illegal character: {\n"},
		{"repeat 1+ print y; print after", "zsh:1: bad math expression: operand expected at end of string\n"},
		{"repeat 1/0 print y; print after", "zsh:1: division by zero\n"},
		{"f(){ repeat 1+ print y; print in }; f; print after", "f: bad math expression: operand expected at end of string\n"},
		// The control: a count that is merely not a number is zero.
		{"repeat x print y; print after", "after\n"},
		// The body after the count is still at command position.
		{"repeat 2 {print y}", "y\ny\n"},
		{"repeat 2 { print y }", "y\ny\n"},
		{"repeat a=2 print y; print $a", "y\ny\n2\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAFailedRepeatCountLeavesItsStatus pins the status a failed count leaves
// the shell with: 1, as a failed expansion in the count leaves. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5382).
func TestAFailedRepeatCountLeavesItsStatus(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
	}{
		{"repeat 1+ print y", 1},
		{"repeat 1/0 print y", 1},
		{"repeat $((1+)) print y", 1},
		{"(exit 3); repeat x print y", 0},
	} {
		if _, status := runZsh(t, t.TempDir(), tc.src); status != tc.status {
			t.Errorf("%s: status %d, want %d", tc.src, status, tc.status)
		}
	}
}
