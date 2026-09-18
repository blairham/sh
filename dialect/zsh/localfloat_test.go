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
// zsh refuses them here too, in the same words.
//
// `-L`, `-R` and `-Z` have left this list: they are the width attributes and
// are built now (#1461). `-E` has left it too — it is a *format* rather than
// a width and the two shells that spell it do not share one, which is what
// took it a separate axis and #2559 rather than a place in that change. `-t`
// left it last (#3101): the **variable** attribute is built and listed, and
// what is not built is the mark the same letter makes on a *function*, which
// this shell writes inside the body where the one other shell with the letter
// writes a row after it. So the refusal that is left is a `-f` line's, and
// naming it here is what keeps the two halves of the letter apart.
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
	// The variable half of `-t` is built: a silent declaration that lists
	// back with the letter.
	if out, st := runZsh(t, dir, `typeset -t x=1; typeset -p x`); out != "typeset -t x=1\n" || st != 0 {
		t.Errorf("typeset -t x=1 = %q (status %d), want the declaration listed back at 0", out, st)
	}
	// The function half is not, and it is refused by name under every word
	// that reaches it — with an operand and without one, since a letter that
	// is really missing has no listing to write either.
	for _, src := range []string{`f(){ :; }; typeset -ft f`, `f(){ :; }; typeset -ft`} {
		out, st := runZsh(t, dir, src)
		if !strings.Contains(out, "-t is not implemented yet") || st == 0 {
			t.Errorf("%s = %q (status %d), want it refused by name", src, out, st)
		}
	}
}
