// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The quiet forms of the directory stack: `pushd -q`, `popd -q`, and the same
// letter in front of a rotation.
//
// The letter is `cd`'s — this shell's `pushd` and `popd` move through `cd` and
// the suppression belongs one level down, at the site that fires the
// directory-change hook (#1775) — so the prelude's job is to read the word and
// hand it on rather than to suppress anything of its own.
//
// Measured 2026-09-10 on zsh 5.9.2, no startup files, a scratch directory with
// `one` in it:
//
//	$ zsh -c 'chpwd() { print HOOK }; cd cw; pushd -q one; print at=${PWD##*/}'
//	at=one
//	$ zsh -c 'chpwd() { print HOOK }; cd cw; pushd one; print at=${PWD##*/}'
//	HOOK
//	at=one
//
// and the same pair for `popd -q` and for `pushd -q +1`.
//
// Before this the option loop matched `+N`, `-N` and `--` and broke on
// anything else, so `-q` fell through to `cd "$1"` as the *directory*: the
// operand was thrown away, the shell went to `$HOME`, and the stack entry went
// with it — a silent wrong answer rather than a diagnostic (#1789). Every case
// below therefore asserts **where the shell ended up and what the stack holds**
// as well as the hook's silence; a test reading only the hook would pass
// against a `pushd` that had gone home quietly.
func TestTheDirectoryStackReadsTheQuietLetter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"pushd -q moves and says nothing",
			"pushd -q one\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"at=one n=1\n",
		},
		{
			"pushd without it fires the hook",
			"pushd one\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"HOOKMARK\nat=one n=1\n",
		},
		{
			"popd -q moves back and says nothing",
			"pushd -q one\npopd -q\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"at=base n=0\n",
		},
		{
			"popd without it fires the hook",
			"pushd -q one\npopd\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"HOOKMARK\nat=base n=0\n",
		},
		{
			"a quiet rotation",
			"pushd -q one\npushd -q +1\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"at=base n=1\n",
		},
		{
			"the letter bundles with cd's others",
			"pushd -Pq one\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"at=one n=1\n",
		},
		{
			"and stands in front of the end of the options",
			"pushd -q -- one\nprint \"at=${PWD##*/} n=${#DIRSTACK[@]}\"",
			"at=one n=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := chpwdTree(t)
			base := dir[strings.LastIndexByte(dir, '/')+1:]
			out, st := runZshPrelude(t, dir,
				"chpwd() { print HOOKMARK }\n"+c.src)
			got := strings.ReplaceAll(out, "at="+base, "at=base")
			if got != c.want || st != 0 {
				t.Errorf("%q printed %q (status %d), want %q at 0",
					c.src, got, st, c.want)
			}
		})
	}
}
