// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `PROMPT` and `PS1` are one parameter under two names, and so are the three
// pairs behind them. Written to through either name and read back through
// both, because a tie that only works one way is what a producer-side fix
// alone would leave.
//
// The rows that are *not* tied are here too, and they are the half that had
// to be measured rather than derived: `RPROMPT`/`RPS1` and `SPROMPT`/`SPS1`
// read exactly like their `PROMPT` siblings and are two parameters each in
// real zsh. A rule inferred from the four above would have tied all seven.
func TestThePromptSpellingsAreOneParameter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `for pair in PROMPT:PS1 PROMPT2:PS2 PROMPT3:PS3 PROMPT4:PS4 RPROMPT:RPS1 SPROMPT:SPS1; do
  a=${pair%%:*}; b=${pair#*:}
  eval "$a=XX"; eval "print -rn -- \"$pair ${a}->${b}=[\${$b-UNSET}] \""
  eval "$b=YY"; eval "print -r -- \"${b}->${a}=[\${$a-UNSET}]\""
done`)
	want := "PROMPT:PS1 PROMPT->PS1=[XX] PS1->PROMPT=[YY]\n" +
		"PROMPT2:PS2 PROMPT2->PS2=[XX] PS2->PROMPT2=[YY]\n" +
		"PROMPT3:PS3 PROMPT3->PS3=[XX] PS3->PROMPT3=[YY]\n" +
		"PROMPT4:PS4 PROMPT4->PS4=[XX] PS4->PROMPT4=[YY]\n" +
		"RPROMPT:RPS1 RPROMPT->RPS1=[UNSET] RPS1->RPROMPT=[XX]\n" +
		"SPROMPT:SPS1 SPROMPT->SPS1=[UNSET] SPS1->SPROMPT=[XX]\n"
	if out != want || st != 0 {
		t.Errorf("the prompt spellings = %q (status %d), want %q", out, st, want)
	}
}

// And the write goes *through* rather than into a stored variable of the
// same name. A stored one would shadow the producer from the next read on,
// so the tie would hold until a script used it — which is the failure the
// writer exists to prevent, asserted by writing twice through alternating
// names and reading back through both each time.
func TestWritingEitherPromptSpellingKeepsThemOne(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PROMPT=one; print -r -- "[$PROMPT][$PS1]"
PS1=two;     print -r -- "[$PROMPT][$PS1]"
PROMPT=three; print -r -- "[$PROMPT][$PS1]"`)
	want := "[one][one]\n[two][two]\n[three][three]\n"
	if out != want || st != 0 {
		t.Errorf("alternating writes = %q (status %d), want %q", out, st, want)
	}
}
