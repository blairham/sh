// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `local` takes the float attribute, which `typeset` already had — #1594.
//
// The two letter sets were written apart, so `local -F` refused a letter this
// shell could answer: `typeset -F x=2` was already `2.0000000000` and
// `typeset -F 3 x=1.5` already `1.500`, both byte-identical to zsh 5.9.2.
// `_zsh_highlight_bind_widgets` opens with a `local -F`, which is where a real
// startup lost syntax highlighting.
//
// The precision comes free and is asserted here because it goes through a
// different axis: `declareOptionMayTakeANumber` reads
// DeclareOptionsTakingANumber, which names `F` whoever is asking, so the letter
// being accepted is the whole of what was needed.
func TestLocalTakesTheFloatAttribute(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`f(){ local -F x=1.5; print -r -- "[$x]"; }; f`, "[1.5000000000]\n"},
		{`f(){ local -F 3 x=1.5; print -r -- "[$x]"; }; f`, "[1.500]\n"},
		// The same value without the attribute, so the row above is the
		// attribute doing it and not the number being printed back.
		{`f(){ local x=1.5; print -r -- "[$x]"; }; f`, "[1.5]\n"},
		// And it is a local: gone when the function returns.
		{`f(){ local -F x=1.5; }; f; print -r -- "[$x]"`, "[]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The letters `local` does *not* take, which is the half that keeps this
// honest — and they fail for two different reasons.
//
// `-f` and `-g` are refused because a local cannot be a function or a global;
// zsh refuses them here too, in the same words. `-E`, `-L`, `-R` and `-t` are
// letters zsh *does* take on `local` and this shell has not built the
// attribute for — `typeset -E` is `-E is not implemented yet` in the same run
// — so they are refused by name rather than claimed. Claiming them would move
// the refusal from the letter to nowhere at all.
func TestLocalStillRefusesTheLettersItHasNoAttributeFor(t *testing.T) {
	if s := zsh.Semantics(); strings.ContainsAny(s.LocalOptions, "fg") {
		t.Errorf("LocalOptions %q claims -f or -g, which a local cannot be", s.LocalOptions)
	}
	dir := t.TempDir()
	for _, letter := range []string{"f", "g"} {
		out, st := runZsh(t, dir, `f(){ local -`+letter+` x; }; f`)
		if want := "f:local: bad option: -" + letter + "\n"; out != want || st == 0 {
			t.Errorf("local -%s = %q (status %d), want %q and a failure", letter, out, st, want)
		}
	}
	// Built under the other word is the test for `F`; not built at all is the
	// test for these, and the refusal names the letter.
	for _, letter := range []string{"E", "L", "R", "t"} {
		out, _ := runZsh(t, dir, `typeset -`+letter+` x=1`)
		if !strings.Contains(out, "-"+letter+" is not implemented yet") {
			t.Errorf("typeset -%s = %q, want it still refused by name", letter, out)
		}
	}
}
