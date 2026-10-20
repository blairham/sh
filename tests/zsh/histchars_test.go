// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A fresh shell has zsh's three characters in `$histchars`, and the
// subscripts a reader uses answer with one character each.
//
// Measured on zsh 5.9.2 with no startup files: `!^#`, and `${(t)histchars}`
// is `scalar-special`.
func TestHistoryCharactersHaveZshsValue(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(),
		`print -r -- "[$histchars] [${histchars[1]}] [${histchars[2]}] [${histchars[3]}] ${#histchars}"`)
	want := "[!^#] [!] [^] [#] 3\n"
	if out != want || st != 0 {
		t.Errorf("histchars = %q (status %d), want %q", out, st, want)
	}
}

// The discriminating one, and the reason #2536 was a P1 rather than a
// cosmetic gap: the parameter is read as the **left end of a pattern**, so an
// empty value does not read as "no character" but as `*`.
//
// This is the highlighter's own test for a history expansion, on the issue's
// own buffer, written out here as shell: with `$histchars` empty every word
// of two or more characters matched it and was styled as one, and the
// single-character `x` was spared only by the second half of the test. Nothing
// errors and nothing is unset, which is why no guard anywhere noticed.
//
// It fails against the pre-image — every word below answers `HIST` — and it
// would go on failing for any future change that left the parameter empty,
// which a test asserting only the value could not distinguish from a test
// asserting nothing.
func TestAnEmptyHistoryCharacterDoesNotTurnItsPatternIntoAStar(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `for w in echo hello '|' grep x '!!' '!$' '^a^b'; do
  if [[ $w = ${histchars[1]}* && -n ${w[2]} ]]; then
    print -rn -- "${w}:HIST "
  elif [[ $w = ${histchars[2]}* ]]; then
    print -rn -- "${w}:QUICK "
  elif [[ $w = ${histchars[3]}* ]]; then
    print -rn -- "${w}:COMMENT "
  else
    print -rn -- "${w}:plain "
  fi
done
print`)
	want := "echo:plain hello:plain |:plain grep:plain x:plain !!:HIST !$:HIST ^a^b:QUICK \n"
	if out != want || st != 0 {
		t.Errorf("the highlighter's own tests = %q (status %d), want %q", out, st, want)
	}
}

// One parameter under two names, written through either and read back through
// both — the shape promptnames.go settled for `PROMPT` and `PS1`, and asserted
// the same way, because a tie that holds until the first write is not a tie.
func TestTheHistoryCharacterSpellingsAreOneParameter(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `print -r -- "[$histchars][$HISTCHARS]"
histchars=@;  print -r -- "[$histchars][$HISTCHARS]"
HISTCHARS='%&'; print -r -- "[$histchars][$HISTCHARS]"
histchars=;   print -r -- "[$histchars][$HISTCHARS]"`)
	want := "[!^#][!^#]\n[@][@]\n[%&][%&]\n[][]\n"
	if out != want || st != 0 {
		t.Errorf("the histchars spellings = %q (status %d), want %q", out, st, want)
	}
}

// And it is a parameter a script owns, not a constant: it takes an
// assignment, it is scoped by `local`, and the outer value is back when the
// function returns. Measured the same way in zsh 5.9.2.
func TestHistoryCharactersAreAnOrdinaryScalarToAScript(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `f() { local histchars=xyz; print -r -- "in:[$histchars]"; }
f
print -r -- "out:[$histchars]"`)
	want := "in:[xyz]\nout:[!^#]\n"
	if out != want || st != 0 {
		t.Errorf("local histchars = %q (status %d), want %q", out, st, want)
	}
}
