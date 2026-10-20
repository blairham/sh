// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A brace fan copies the **names** and not the **work**: zsh 5.9.2 expands the
// word once and hands every name the same results, so a command substitution
// in a fanned word runs once however many names come out and an arithmetic one
// moves its variable once.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh -f` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable* —
// under `-c`, `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a
// directory of its own:
//
//	                                      TICK lines   words
//	echo {x,y}$(echo TICK >&2; echo z)             1   xz yz
//	echo {x,y,w}$(…)                              1   xz yz wz
//	echo {a,{b,c}}$(…)                            1   az bz cz
//	echo {1..3}$(…)                               1   1z 2z 3z
//	echo {,x}$(…)                                 1   z xz
//	echo {a,b}{c,d}$(…)                           1   acz adz bcz bdz
//	echo $(…){x,y}                                1   zx zy
//	: > {x,y}$(…)                                 1   files xz and yz
//
//	i=0; echo {x,y,w}$((i++)); echo "i=$i"   ->   x0 y0 w0 / i=1
//
// bash 5.3.20 and 3.2.57 answer the other way — two runs for two names, `x0 y1
// w2` and `i=3` — and ksh93u+ answers as this shell does. The **words are the
// same in every column**, so the count and the variable are the whole of the
// tell (#4694).
// countZshTicks counts whole lines rather than asking whether the mark is
// in there: the two readings produce the same *words*, so containment cannot
// tell them apart and only the count can.
func countZshTicks(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "TICK" {
			n++
		}
	}
	return n
}

const zshTickSubst = `$(echo TICK >&2; echo z)`

func TestZshABraceFanSharesOneRunOfTheWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, words string }{
		{"two alternatives", "echo {x,y}" + zshTickSubst, "xz yz"},
		{"three alternatives", "echo {x,y,w}" + zshTickSubst, "xz yz wz"},
		{"a nested group", "echo {a,{b,c}}" + zshTickSubst, "az bz cz"},
		{"a range", "echo {1..3}" + zshTickSubst, "1z 2z 3z"},
		{"an empty alternative", "echo {,x}" + zshTickSubst, "z xz"},
		{"two groups", "echo {a,b}{c,d}" + zshTickSubst, "acz adz bcz bdz"},
		{"the group behind it", "echo " + zshTickSubst + "{x,y}", "zx zy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runZsh(t, t.TempDir(), tc.src+"\n")
			// One run however many names, which is the whole claim.
			const want = 1
			if n := countZshTicks(out); n != want {
				t.Errorf("%s = %q: %d runs, want %d", tc.src, out, n, want)
			}
			if !strings.Contains(out, tc.words) {
				t.Errorf("%s = %q, want the names %q", tc.src, out, tc.words)
			}
		})
	}
}

// The side effect, which is the half a count cannot be argued with: the
// variable an arithmetic substitution moves is moved once per name or once for
// the word, and the values substituted move with it.
func TestZshABraceFanMovesAVariableOnceForTheWord(t *testing.T) {
	t.Parallel()
	out, _ := runZsh(t, t.TempDir(), "i=0\necho {x,y,w}$((i++))\necho \"i=$i\"\n")
	if want := "x0 y0 w0"; !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// The control that says the rule is keyed on the **fan** and not on where the
// word stands: the same word as a redirection target answers the same way.
func TestZshARedirectionTargetsFanAnswersLikeAnArgument(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _ := runZsh(t, dir, ": > {x,y}"+zshTickSubst+"\n")
	if n := countZshTicks(out); n != 1 {
		t.Errorf("out = %q: %d runs, want %d", out, n, 1)
	}
	for _, name := range []string{"xz", "yz"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("want the name %q opened: %v", name, err)
		}
	}
}

// And the pair that holds the noun fixed. `$e` comes to two words with no
// brace in the word at all, and its substitution runs once in every column —
// so a rule keyed on "a word that came to several words" would report this
// shell's fan reading for a word that has no fan.
func TestZshSeveralWordsWithoutBracesIsNotAFan(t *testing.T) {
	t.Parallel()
	out, _ := runZsh(t, t.TempDir(), "e='p q'\necho $e"+zshTickSubst+"\n")
	if n := countZshTicks(out); n != 1 {
		t.Errorf("out = %q: %d runs, want 1 — splitting is not a fan", out, n)
	}
	// The positive beside it, so the 1 above is falsifiable in this column.
	braced, _ := runZsh(t, t.TempDir(), "echo {p,q}"+zshTickSubst+"\n")
	if n := countZshTicks(braced); n != 1 {
		t.Errorf("braced = %q: %d runs, want 1 — the probe cannot fire", braced, n)
	}
}

// A failure ends the fan, whichever way this column answers the axis: one
// diagnostic, counted rather than contained, because the defect was a second
// sentence after the first.
func TestZshAFailedExpansionEndsTheFan(t *testing.T) {
	t.Parallel()
	out, _ := runZsh(t, t.TempDir(), "echo {x,y,w}$(( 1/0 ))\n")
	if n := strings.Count(out, "division by zero"); n != 1 {
		t.Errorf("out = %q: %d diagnostics, want exactly 1", out, n)
	}
}
