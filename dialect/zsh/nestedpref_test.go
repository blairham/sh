// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedPInnerIsAReference is #6191: a `${(P)…}` standing as the inner
// of a nesting refers to the parameter its group names, so the level around
// it reads that parameter's elements, quoted or not, and the rest of the
// inner's group — flags and operator alike — ran on the *name*.
//
// Every row recorded from zsh 5.9.2 under `-f -c` on 2026-10-05, with
// `b=(yy x)`, `B=(Q R)`, `n=b` and `u` unset. The last rows are the
// controls: a plain name nested is joined by the quotes, a level above the
// `(P)` reads what it was handed, and the same `(P)` not nested runs its
// operator on the value.
func TestANestedPInnerIsAReference(t *testing.T) {
	const pre = "b=(yy x); B=(Q R); n=b\n"
	for _, c := range []struct{ src, want string }{
		{`print -r -- "${(j:|:)${(P)n}}"`, "yy|x"},
		{`print -r -- "${(j:|:)"${(P)n}"}"`, "yy|x"},
		{`print -r -- ${(j:|:)"${(P)n}"}`, "yy|x"},
		{`print -r -- "${(j:|:)${(@q)${(P)n}}}"`, "yy|x"},
		{`print -r -- "${(j:|:)${(P)${:-b}}}"`, "yy|x"},
		{`print -rl -- "${(@o)${(P)n}}"`, "x\nyy"},
		{`print -r -- "${(j:|:)${(PU)n}}"`, "Q|R"},
		{`print -r -- "${(j:|:)${(P)n/b/B}}"`, "Q|R"},
		{`print -r -- "${(j:|:)${(P)n%x}}"`, "yy|x"},
		{`print -r -- "${(j:|:)${(P)u:-n}}"`, "b"},
		{`print -r -- "[${(j:|:)${(P)n:#b}}]"`, "[]"},
		{`b=(x '' y); print -r -- ${(j:|:)${(P)n}}`, "x||y"},
		{`typeset -A h=(k1 v1); hn=h; print -r -- "${(j:|:)${(Pk)hn}}"`, "v1"},
		{`print -r -- "[${${(P)nope?}}]"`, "[]"},
		{`print -r -- "${(j:|:)${b}}"`, "yy x"},
		{`print -r -- "${(j:|:)${${(P)n}}}"`, "yy x"},
		{`print -r -- "${(j:|:)${:-${(P)n}}}"`, "yy x"},
		{`print -r -- ${(P)n/y/z}`, "zy x"},
	} {
		t.Run(c.src, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), pre+c.src)
			if out != c.want+"\n" || st != 0 {
				t.Errorf("got %q, %d, want %q, 0", out, st, c.want+"\n")
			}
		})
	}
}

// TestANestedPInnerOnAnUndeclaredNameIsRefusedBeforeItsWord: `-` with a word
// on a `(P)` of a name nothing declared is a bad substitution nested as it is
// not, and the word never runs. Recorded from zsh 5.9.2 on 2026-10-05.
func TestANestedPInnerOnAnUndeclaredNameIsRefusedBeforeItsWord(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "[${${(P)nope-$(echo RAN)}}]"; print -r -- after`)
	if want := "zsh:1: bad substitution\n"; out != want || st != 1 {
		t.Errorf("got %q, %d, want %q, 1", out, st, want)
	}
}
