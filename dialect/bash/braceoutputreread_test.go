// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// What brace expansion produced comes back to the rest of word expansion as
// ordinary shell **text** here, which is this shell alone in the panel —
// ksh93 and zsh substitute it into the word the parse cut. See
// interp.Semantics.BraceOutputRereadAsText.
//
// Measured 2026-09-22 against `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash`
// 3.2.57, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`. The braces
// are resolved before parameter expansion in every column that has them —
// that much was already modeled, and `echo {$a,2}` agrees — so what this pins
// is what the words that come out *are*: the strings `$varx` and `$vary`, in
// which the name runs on into the character the group wrote.
func TestBraceOutputIsRereadAsShellText(t *testing.T) {
	const set = `var=baz; varx=vx; vary=vy; `
	for _, tc := range []struct{ name, src, want string }{
		{"a bare name the produced text continues", set + `echo $var{x,y}`, "vx vy\n"},
		// The row that says it is the bare spelling and not the brace: a
		// braced name is already ended, so both readings answer alike and
		// this one was right before the axis existed.
		{"a braced name is ended already", set + `echo ${var}{x,y}`, "bazx bazy\n"},
		{"and the tail behind it runs on too", set + `varxz=vxz; printf '[%s]' $var{x,y}z`, "[vxz]"},
		// Ordinary text is inert, so the two readings are one answer.
		{"plain text either side", `echo a{b,c}d`, "abd acd\n"},
		{"a numeric range", `echo {1..4}`, "1 2 3 4\n"},
		{"a range and a list", `echo {1..3}{x,y}`, "1x 1y 2x 2y 3x 3y\n"},
		// A range counted between two letters walks the code points, and what
		// it writes is raw text rather than data: the backslash reaches quote
		// removal like any other unquoted one, so its element is empty — a
		// field, and an empty one.
		{"a produced backslash is an escape", `printf '[%s]' {Z..a}`, "[Z][[][][]][^][_][`][a]"},
		{"and quotes nothing at the end of a word", `printf '[%s]' ab{Z..a}`, "[abZ][ab[][ab][ab]][ab^][ab_][ab`][aba]"},
		// A produced word with no text at all is no field, which is the
		// contrast that says the empty element above is a quoted empty
		// string rather than a word that vanished.
		{"a produced word with no text is no field", `printf '[%s]' {,}`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runBash(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The sharpest probe, and the one that says this is a re-reading rather than
// "a backslash is stripped": the backtick a character range counts **opens a
// command substitution**, and the text behind it in the word never closes it.
//
// Measured 2026-09-22 on bash 5.3.20 from a script file: the line is abandoned
// at status 1 with nothing printed, and the next line runs — which is
// interp.Semantics.FailedExpansionAbandonsTheLine and not a syntax error. zsh
// answers eight elements for the same word, xZy through xay, with the
// backslash and the backtick both standing as characters.
func TestAProducedBacktickOpensASubstitutionNothingCloses(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "printf '[%s]' x{Z..a}y\necho \"after=$?\"\n")
	const want = "bash: line 1: bad substitution: no closing \"`\" in `y\nafter=1\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, want)
	}
}
