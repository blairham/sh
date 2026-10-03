// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestAnAssignmentListsTraceLineIsWrittenAsItGoes pins where a diagnostic from
// a traced assignment list lands: inside the line, after the words already
// written. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5485).
func TestAnAssignmentListsTraceLineIsWrittenAsItGoes(t *testing.T) {
	const nestedG = "f: scalar parameter g set in enclosing scope in function f\n"
	const nestedH = "f: scalar parameter h set in enclosing scope in function f\n"
	cases := []struct{ src, want string }{
		{
			"f(){ g=2 }; o(){ local g=1; f }; functions -Wt f; o",
			"+f:0> g=2 " + nestedG + "\n",
		},
		{
			"f(){ g=2 h=3 }; o(){ local g=1 h=1; f }; functions -Wt f; o",
			"+f:0> g=2 " + nestedG + "h=3 " + nestedH + "\n",
		},
		{
			"f(){ g=2 x=5 }; o(){ local g=1; f }; functions -Wt f; o",
			"+f:0> g=2 " + nestedG + "x=5 \n",
		},
		{
			"f(){ g=(1 2) }; o(){ local g=1; f }; functions -Wt f; o",
			"+f:0> g=( 1 2 ) f: array parameter g set in enclosing scope in function f\n\n",
		},
		// A refused store: the rest of the list is not traced, and the line
		// still gets its newline.
		{"readonly r; set -x; a=1 r=2 j=3", "+zsh:1> a=1 r=2 zsh:1: read-only variable: r\n\n"},
		// An expansion that ends the shell: the name went out before the
		// value was expanded, and the line is left where it got to.
		{"set -x; a=1 b=${x?boom}", "+zsh:1> a=1 b=zsh:1: x: boom\n"},
		// The control: nothing in the middle, the line it always was.
		{"set -x; a=1 b=2", "+zsh:1> a=1 b=2 \n"},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// TestAPrefixTakenBackDrawsNoScopeLint pins that neither scope lint speaks
// about an assignment prefix that the command's end takes back, while one
// that persists is still an assignment. Measured 2026-10-02 on zsh 5.9.2
// under `-f` (#5485).
func TestAPrefixTakenBackDrawsNoScopeLint(t *testing.T) {
	cases := []struct{ src, want string }{
		{"setopt warncreateglobal; f(){ g=2 print hi; g=3 print -n; g=4 builtin print -n }; f; print ${g-unset}", "hi\nunset\n"},
		{"setopt warncreateglobal; f2(){ :; }; f(){ g=2 f2 }; f; print ${g-unset}", "unset\n"},
		{"f(){ g=5 :; g=6 eval : }; o(){ local g=1; f }; functions -W f; o; print done", "done\n"},
		// The controls: a prefix that persists, and an ordinary assignment
		// beside a taken-back prefix.
		{
			"setopt posixbuiltins warncreateglobal; f(){ g=2 :; print $g }; f",
			"f: scalar parameter g created globally in function f\n2\n",
		},
		{
			"setopt warncreateglobal; f(){ g=2 print -n; h=1 }; f",
			"f: scalar parameter h created globally in function f\n",
		},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
