// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A function's text, now that a read keeps it, still follows what the text
// depends on: a redefinition, a listing's `-x` indent, an autoload stub, and
// a subshell's own definition. Measured on zsh 5.9.2 (`-f`, 2026-10-12),
// byte for byte; see interp/bodytext.go and #6423.
func TestAFunctionsKeptTextFollowsItsDefinition(t *testing.T) {
	t.Parallel()
	const src = `g() { if true; then a; fi; }
x=${functions[g]}
functions -x 2 g
y=${functions[g]}
g() { b; }
z=${functions[g]}
autoload -Uz fsa
w=${functions[fsa]}
(g() { c; }; print -r -- "sub:${functions[g]}")
v=${functions[g]}
print -rl -- "$x" "$y" "$z" "$w" "$v"
`
	const want = "g () {\n  if true\n  then\n    a\n  fi\n}\n" +
		"sub:\tc\n" +
		"\tif true\n\tthen\n\t\ta\n\tfi\n" +
		"\tif true\n\tthen\n\t\ta\n\tfi\n" +
		"\tb\n" +
		"builtin autoload -XU\n" +
		"\tb\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
