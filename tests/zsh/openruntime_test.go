// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `%_` drawn while a command runs says which constructs it is inside (#5150).
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, under `-c` and from a script file
// alike, `env -i PATH=/usr/bin:/bin LC_ALL=C`) with `PS4='[%_]'` and xtrace;
// each row is the trace that shell wrote to standard error.
func TestTheOpenStateFieldDrawsWhatACommandIsInside(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"if, then", "if true; then true; fi", "[if]true\n[then]true\n"},
		{"elif and its body", "if false; then :; elif true; then true; fi", "[if]false\n[elif]true\n[elif-then]true\n"},
		{"else", "if false; then :; else true; fi", "[if]false\n[else]true\n"},
		{"a loop around an if", "while true; do if true; then true; fi; break; done", "[while]true\n[while if]true\n[while then]true\n[while]break\n"},
		{"until", "until true; do :; done", "[until]true\n"},
		{"||", "false || true", "[]false\n[cmdor]true\n"},
		{"&& and a group", "true && { true || true; }", "[]true\n[cmdand cursh]true\n"},
		{"a pipe's later element", "true | true", "[]true\n[pipe]true\n"},
		{"a subshell draws nothing", "( true )", "[]true\n"},
		{"repeat", "repeat 1 true", "[repeat]true\n"},
		{"a call starts afresh", "f() { if true; then true; fi }; while true; do f; break; done", "[while]true\n[while]f\n[if]true\n[then]true\n[while]break\n"},
		{"case", "case y in y) true;; esac", "[case]case y (y)\n[case]true\n"},
		{"for in", "for i in 1; do true; done", "[for]i=1\n[for]true\n"},
		{"arithmetic for", "for ((i=0;i<1;i++)); do true; done", "[]i=0\n[for]i<1\n[for]true\n[for]i++\n[for]i<1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "PS4='[%_]'\nsetopt xtrace\n" + c.src + "\nunsetopt xtrace 2>/dev/null\n"
			out, _ := runZsh(t, dir, src+"\n")
			want := c.want + "[]unsetopt xtrace\n"
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
	// And outside a trace, through `print -P`: the field is the shell's and
	// not the tracer's.
	if out, _ := runZsh(t, dir, "if true; then print -P '[%_]'; fi\nrepeat 1 print -P '[%_]'\n"); out != "[then]\n[repeat]\n" {
		t.Errorf("print -P = %q, want %q", out, "[then]\n[repeat]\n")
	}
}
