// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The value an assigning operator stores and substitutes under a flag group
// — `${(P)n::=w}`, `${(U)v:=w}`, `${(j:,:)v=w}` — is the word as a value:
// not split, not matched against the filesystem, and read back as text the
// way every expansion's result is in this dialect. The route with a flag
// group matched it, so F-Sy-H's git chroma, which caches `(add|am|…)`
// through `${(P)__var_name::=(${(j:|:)reply})}`, ended the shell with
// `number expected` on every `git` line (#6966). The `-` and `+` operators'
// words are the control: those are command-line words and are matched.
// Measured 2026-10-27 on zsh 5.9.2 under -fc in a directory holding one file
// `q`; every `want` is that shell's output.
func TestAFlaggedAssignmentsValueIsNotMatched(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"(P) ::= a group", `n=v; print -r -- ${(P)n::=(a)}; print -r -- "[$v]"`, "(a)\n[(a)]\n"},
		{"(P) := a group", `n=v; print -r -- ${(P)n:=(a)}; print -r -- "[$v]"`, "(a)\n[(a)]\n"},
		{"(P) = a group", `n=v; print -r -- ${(P)n=(a)}; print -r -- "[$v]"`, "(a)\n[(a)]\n"},
		{"(P) ::= a pattern that would match", `n=v; print -r -- ${(P)n::=q*}; print -r -- "[$v]"`, "q*\n[q*]\n"},
		{"(P) ::= a pattern that would not", `n=v; print -r -- ${(P)n::=zz*}; print -r -- "[$v]"`, "zz*\n[zz*]\n"},
		{"(P) ::= inside a word", `n=v; print -r -- x${(P)n::=(a)}y`, "x(a)y\n"},
		{"(P) ::= into an element", `typeset -gA H; n='H[k]'; : ${(P)n::=(a|b)}; print -r -- "[${H[k]}]"`, "[(a|b)]\n"},
		{"(P) ::= a bracket into an element", `typeset -gA H; n='H[k]'; : ${(P)n::=[a]}; print -r -- "[${H[k]}]"`, "[[a]]\n"},
		{"(U) ::= a group", `print -r -- ${(U)v::=(a)}; print -r -- "[$v]"`, "(A)\n[(a)]\n"},
		{"(U) := a pattern", `print -r -- ${(U)v:=q*}; print -r -- "[$v]"`, "Q*\n[q*]\n"},
		{"(U) = a miss", `print -r -- ${(U)v=zz*}; print -r -- "[$v]"`, "ZZ*\n[zz*]\n"},
		{"(U) ::= beside another word", `print -r -- ${(U)v::=q*} other; print -r -- "[$v]"`, "Q* other\n[q*]\n"},
		{"(j) ::= a pattern", `print -r -- ${(j:,:)v::=q*}; print -r -- "[$v]"`, "q*\n[q*]\n"},
		{"the tilde flag asks for the match", `n=v; print -r -- ${(P)~n::=q*}; print -r -- "[$v]"`, "q\n[q*]\n"},
		{"the control: a - word is matched", `n=v; unset v; print -r -- ${(P)n:-q*}`, "q\n"},
		{"the control: a - word that misses", `n=v; unset v; print -r -- ${(P)n:-zz*}; print after`, "zsh:1: no matches found: zz*\n"},
		{"the control: a - group is a qualifier list", `unset v; print -r -- ${(U)v:-(a)}; print after`, "zsh:1: no matches found: (A)\n"},
		{"the control: the route with no group", `print -r -- ${v::=q*}; print -r -- "[$v]"`, "q*\n[q*]\n"},
		{
			"the chroma's own shape",
			`typeset -gA def; reply=(add am commit); n='def[subcommands]'; : ${(P)n::=(${(j:|:)reply})}; print -r -- "[${def[subcommands]}]"`,
			"[(add|am|commit)]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "q"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if out, _ := runZsh(t, dir, tc.src); out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
