// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestKshTypesetKeepsABuiltinDeclarationsAssignmentWhole is #5157's E03posix
// front `KSH_TYPESET option`. The option was recorded and inert. Every row
// is run under both states: a shell that ignores the option passes half of
// each pair, and one that applied it everywhere would fail the `unsetopt`
// half. See interp/kshtypeset.go for the rule.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
// Each is read by `eval` after `disable -r` so that the builtin runs, which is
// how the reference's own suite asks it.
func TestKshTypesetKeepsABuiltinDeclarationsAssignmentWhole(t *testing.T) {
	for _, c := range []struct{ name, src, off, on string }{
		{"the assignment", `typeset x=$(echo a b); print -r -- "[$x]" $+b`, "[a] 1\n", "[a b] 0\n"},
		{"a name from a parameter", `nm=z; typeset $nm=$(echo c d); print -r -- "[$z]" $+d`, "[c] 1\n", "[c d] 0\n"},
		{"a quoted name", `typeset "w"=$(echo e f); print -r -- "[$w]" $+f`, "[e] 1\n", "[e f] 0\n"},
		{"an escaped equals", `typeset w\=$(echo a b); print -r -- "[$w]" $+b`, "[a] 1\n", "[a b] 0\n"},
		{"a quoted equals", `typeset y"="$(echo e f); print -r -- "[$y]" $+f`, "[e] 1\n", "[e f] 0\n"},
		{"a substituted name", `typeset $(echo z)=$(echo g h); print -r -- "[$z]" $+h`, "[g] 1\n", "[g h] 0\n"},
		{"a second equals", `typeset q=r$(echo s t)=u; print -r -- "[$q]" $+t`, "[rs] 1\n", "[rs t=u] 0\n"},
		// An `=` the word writes is not enough when the text in front of it
		// splits: the word splits as it would have.
		{"a split in front", `typeset $(echo a b)c=d; print -r -- $+a "[$bc]"`, "1 [d]\n", "1 [d]\n"},
		{"a split inside the name", `typeset m$(echo n o)=p; print -r -- "[$mn]" $+o`, "[] 1\n", "[] 1\n"},
		// And an `=` an expansion produced is not one the word writes.
		{"an equals from an expansion", `typeset $(echo m=1 n=2); print $m $+n`, "1 1\n", "1 1\n"},
		{"local", `f() { local x=$(echo 3 q); print -r -- "[$x]" $+q; }; f`, "[3] 1\n", "[3 q] 0\n"},
		{"export", `export x=$(echo 3 q); print -r -- "[$x]" $+q`, "[3] 1\n", "[3 q] 0\n"},
		{"readonly", `readonly x=$(echo 3 q); print -r -- "[$x]" $+q`, "[3] 1\n", "[3 q] 0\n"},
		{"declare", `declare x=$(echo 3 q); print -r -- "[$x]" $+q`, "[3] 1\n", "[3 q] 0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			word := "typeset"
			for _, w := range []string{"local", "export", "readonly", "declare"} {
				if c.name == w {
					word = w
				}
			}
			for _, state := range []struct{ opt, want string }{{"unsetopt", c.off}, {"setopt", c.on}} {
				src := "disable -r " + word + "; " + state.opt + " kshtypeset; eval '" +
					strings.ReplaceAll(c.src, "'", `'\''`) + "'\n"
				out, _ := runZsh(t, t.TempDir(), src)
				if out != state.want {
					t.Errorf("%s: %s\ngot  %q\nwant %q", state.opt, c.src, out, state.want)
				}
			}
		})
	}
}

// The option reaches a declaration run through `builtin` too, with the
// reserved word still on, and leaves the reserved word itself alone: its
// operands were decided by the parse.
func TestKshTypesetReachesBuiltinAndNotTheReservedWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`setopt kshtypeset; builtin typeset c=$(echo 5 6); print -r "[$c]" $+6`, "[5 6] 0\n"},
		{`setopt kshtypeset; f() { typeset "z"=$(echo c d) x=$(echo g h); print -r -- "[$z][$x]" $+d $+h; }; f`, "[c][g h] 1 0\n"},
		{`unsetopt kshtypeset; f() { typeset "z"=$(echo c d) x=$(echo g h); print -r -- "[$z][$x]" $+d $+h; }; f`, "[c][g h] 1 0\n"},
		// Every emulation leaves it off.
		{`emulate ksh; [[ -o kshtypeset ]] && echo on || echo off`, "off\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
