// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A brace fan copies the **names** and not the **work**: ksh93 runs the word's
// expansions once and every name the braces make shares the results.
//
// It reaches that from the other side from zsh — this shell brace-expands the
// *text* its expansions produced, where zsh expands the word once and fans what
// came out — and the count it leaves is the same, which is what the axis is
// about. Where the braces are *found* is a neighboring question and is not
// this one.
//
// Measured 2026-09-26 on `/bin/ksh` — `Version AJM 93u+ 2012-08-01`, `go
// version -m` says *not a Go executable* — under `-c`, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, each case in a directory of its
// own:
//
//	                                      TICK lines   words
//	echo {x,y}$(echo TICK >&2; echo z)             1   xz yz
//	echo {x,y,w}$(…)                              1   xz yz wz
//	echo {a,{b,c}}$(…)                            1   az bz cz
//	echo {1..3}$(…)                               1   1z 2z 3z
//	echo {,x}$(…)                                 1   z xz
//	echo {a,b}{c,d}$(…)                           1   acz adz bcz bdz
//	echo $(…){x,y}                                1   zx zy
//
//	i=0; echo {x,y,w}$((i++)); echo "i=$i"   ->   x0 y0 w0 / i=1
//
// The redirection **target** is the row that says the fan and the redirection
// are different questions: this shell does not brace-expand a target at all,
// so `: > {x,y}$(…)` writes the single file `{x,y}z` and runs the
// substitution once because there is one name (#4694).
//
// bash 5.3.20 and 3.2.57 answer the axis the other way, and the **words agree
// in every column**: the count and the variable are the whole of the tell.
// countKshTicks counts whole lines rather than asking whether the mark is
// in there: the two readings produce the same *words*, so containment cannot
// tell them apart and only the count can.
func countKshTicks(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "TICK" {
			n++
		}
	}
	return n
}

const kshTickSubst = `$(echo TICK >&2; echo z)`

func TestKshABraceFanSharesOneRunOfTheWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, words string }{
		{"two alternatives", "echo {x,y}" + kshTickSubst, "xz yz"},
		{"three alternatives", "echo {x,y,w}" + kshTickSubst, "xz yz wz"},
		{"a nested group", "echo {a,{b,c}}" + kshTickSubst, "az bz cz"},
		{"a range", "echo {1..3}" + kshTickSubst, "1z 2z 3z"},
		{"an empty alternative", "echo {,x}" + kshTickSubst, "z xz"},
		{"two groups", "echo {a,b}{c,d}" + kshTickSubst, "acz adz bcz bdz"},
		{"the group behind it", "echo " + kshTickSubst + "{x,y}", "zx zy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), tc.src+"\n")
			// One run however many names, which is the whole claim.
			const want = 1
			if n := countKshTicks(out); n != want {
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
func TestKshABraceFanMovesAVariableOnceForTheWord(t *testing.T) {
	t.Parallel()
	out, _ := runKsh(t, t.TempDir(), "i=0\necho {x,y,w}$((i++))\necho \"i=$i\"\n")
	if want := "x0 y0 w0"; !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// The same word as a redirection target, which in this column is not a fan at
// all: one literal name, one run. It is here because it is the row that would
// move if the sharing were installed on the redirection rather than on the
// braces.
func TestKshARedirectionTargetIsNotAFanHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, _ := runKsh(t, dir, ": > {x,y}"+kshTickSubst+"\n")
	if n := countKshTicks(out); n != 1 {
		t.Errorf("out = %q: %d runs, want %d", out, n, 1)
	}
	if _, err := os.Stat(filepath.Join(dir, "{x,y}z")); err != nil {
		t.Errorf("want the one literal name opened: %v", err)
	}
}

// And the pair that holds the noun fixed. `$e` comes to two words with no
// brace in the word at all, and its substitution runs once in every column —
// so a rule keyed on "a word that came to several words" would report this
// shell's fan reading for a word that has no fan.
func TestKshSeveralWordsWithoutBracesIsNotAFan(t *testing.T) {
	t.Parallel()
	out, _ := runKsh(t, t.TempDir(), "e='p q'\necho $e"+kshTickSubst+"\n")
	if n := countKshTicks(out); n != 1 {
		t.Errorf("out = %q: %d runs, want 1 — splitting is not a fan", out, n)
	}
	// The positive beside it, so the 1 above is falsifiable in this column.
	braced, _ := runKsh(t, t.TempDir(), "echo {p,q}"+kshTickSubst+"\n")
	if n := countKshTicks(braced); n != 1 {
		t.Errorf("braced = %q: %d runs, want 1 — the probe cannot fire", braced, n)
	}
}

// A failure ends the fan, whichever way this column answers the axis: one
// diagnostic, counted rather than contained, because the defect was a second
// sentence after the first.
func TestKshAFailedExpansionEndsTheFan(t *testing.T) {
	t.Parallel()
	out, _ := runKsh(t, t.TempDir(), "echo {x,y,w}$(( 1/0 ))\n")
	if n := strings.Count(out, "divide by zero"); n != 1 {
		t.Errorf("out = %q: %d diagnostics, want exactly 1", out, n)
	}
}
