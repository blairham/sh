// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
	"unicode/utf8"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The word #1022 made this shell quote back is not printed whole: it is cut
// to twenty **bytes** and marked with `...`.
//
// Measured 2026-09-07, bisected by length over `-n` on a script file. The
// boundary is the part worth pinning, because it is not the obvious one: the
// mark appears at exactly twenty, where nothing has been removed. So the
// ellipsis means "twenty or more", and a limit that appended it only when it
// actually cut would differ from the shell on precisely one length — which is
// the length a fix would be most likely to get wrong and least likely to
// notice.
//
// Whole rendered lines with their locations, for the reason #1022's test
// gives: a substring assertion cannot tell a cut text from a prefixed one,
// and `parse error near ` + a shortened word is a prefix of the same sentence
// with the long word in it.
func TestAQuotedWordIsCutToTwentyBytesAndMarked(t *testing.T) {
	for _, tc := range []struct {
		why, src, want string
	}{
		{
			"nineteen bytes come back whole",
			"echo $(bbbbbbbbbbbbbbbbb\n",
			"parse error near `$(bbbbbbbbbbbbbbbbb'",
		},
		{
			"twenty is marked although nothing was cut",
			"echo $(bbbbbbbbbbbbbbbbbb\n",
			"parse error near `$(bbbbbbbbbbbbbbbbbb...'",
		},
		{
			"twenty-one is cut to twenty and marked",
			"echo $(bbbbbbbbbbbbbbbbbbb\n",
			"parse error near `$(bbbbbbbbbbbbbbbbbb...'",
		},
		{
			"and so is anything longer, to the same twenty",
			"echo $(bbbbbbbbbbbbbbbbbbbbbbbbbb\n",
			"parse error near `$(bbbbbbbbbbbbbbbbbb...'",
		},
		{
			// The shape #1024 wrote around the limit by using `:` instead of
			// `echo` to stay under it.
			"a nested substitution, eighteen bytes, still whole",
			"v=$(echo $(cat <<E\nz\nE))\n",
			"parse error near `v=$(echo $(cat <<E'",
		},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%s: %q parsed, want a refusal", tc.why, tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%s: %q:\n got %q\nwant %q", tc.why, tc.src, got, tc.want)
		}
	}
}

// The limit is a count of bytes and it cuts between them, which is measured
// rather than tolerated: a word whose twentieth byte begins a multibyte
// character comes back from the real shell with that character's lead byte
// alone, and the diagnostic it prints is not valid UTF-8.
//
// Pinned because the tempting improvement — round down to a character
// boundary so the output is always well formed — would be a *nicer*
// diagnostic than the shell's and would stop matching it. The row that
// catches that is the one whose expected text is deliberately invalid.
func TestTheTwentyByteCutIsByBytesAndSplitsACharacter(t *testing.T) {
	for _, tc := range []struct {
		why, src, want string
	}{
		{
			// `v=$(echo héllo wör` is eighteen characters in twenty bytes,
			// so this one lands on a boundary and stays well formed.
			"a word of twenty bytes and eighteen characters",
			"v=$(echo héllo wörld aaaaaaa\n",
			"parse error near `v=$(echo héllo wör...'",
		},
		{
			"nineteen bytes of the same word come back whole",
			"v=$(echo héllo wö\n",
			"parse error near `v=$(echo héllo wö'",
		},
		{
			"a two-byte character across the boundary keeps its lead byte",
			"v=$(echo héllo wöéxxxx\n",
			"parse error near `v=$(echo héllo wö\xc3...'",
		},
		{
			"a three-byte character, the same way",
			"v=$(echo héllo wö€xxxx\n",
			"parse error near `v=$(echo héllo wö\xe2...'",
		},
		{
			"and a four-byte one",
			"v=$(echo héllo wö\U0001F600xxxx\n",
			"parse error near `v=$(echo héllo wö\xf0...'",
		},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%s: %q parsed, want a refusal", tc.why, tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%s: %q:\n got %q\nwant %q", tc.why, tc.src, got, tc.want)
		}
	}
}

// The last three rows above assert text that is not valid UTF-8, and this
// says so out loud so that a later reader does not "fix" them.
func TestTheSplitCharacterRowsAreDeliberatelyInvalidUTF8(t *testing.T) {
	src := "v=$(echo héllo wöéxxxx\n"
	_, err := syntax.Parse(src, zsh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	got := zsh.Diagnostics().ParseFailure(err)
	if utf8.ValidString(got) {
		t.Errorf("%q rendered %q, which is valid UTF-8 — the real shell's is not, "+
			"so the cut has been rounded to a character boundary", src, got)
	}
}
