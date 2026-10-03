// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnAnonymousFunctionCallSetsUnderscore pins that a call to an anonymous
// function moves `$_` to its last argument, or to empty where it has none,
// as a named function's call does: the body reads it on entry and the caller
// reads it afterwards (#5151, a chunk of D04parameter.ztst). Measured
// 2026-10-03 on zsh 5.9.2 under `-f`.
func TestAnAnonymousFunctionCallSetsUnderscore(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print a b; () { print "[$_]" }`, "a b\n[]\n"},
		{`print a b; () { print "[$_]" } x y`, "a b\n[y]\n"},
		{`print a b; function { print "[$_]" }`, "a b\n[]\n"},
		{`() { echo x y } p q; echo "[$_]"`, "x y\n[q]\n"},
		{`() { echo x y }; echo "[$_]"`, "x y\n[]\n"},
		{`() { () { echo "[$_]" } in } out; echo "[$_]"`, "[in]\n[out]\n"},
		// The control: a command inside the body still moves it there.
		{`print a b; () { :; print "[$_]" }`, "a b\n[:]\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
