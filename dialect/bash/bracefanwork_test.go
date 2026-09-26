// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A brace fan copies the **names**, and this is the column that copies the
// **work** with them: bash expands the word again for every name, so a command
// substitution in a fanned word runs once per name and an arithmetic one moves
// its variable once per name.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash`
// 3.2.57 — `go version -m` says *not a Go executable* for both — under `-c`,
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a directory of
// its own. The two versions agree row for row, which was asked rather than
// assumed: this is a count question and the two have split on one elsewhere in
// this campaign.
//
//	                                      TICK lines   words
//	echo {x,y}$(echo TICK >&2; echo z)             2   xz yz
//	echo {x,y,w}$(…)                              3   xz yz wz
//	echo {a,{b,c}}$(…)                            3   az bz cz
//	echo {1..3}$(…)                               3   1z 2z 3z
//	echo {,x}$(…)                                 2   z xz
//	echo {a,b}{c,d}$(…)                           4   acz adz bcz bdz
//	echo $(…){x,y}                                2   zx zy
//
//	i=0; echo {x,y,w}$((i++)); echo "i=$i"   ->   x0 y1 w2 / i=3
//
// zsh 5.9.2 and ksh93u+ answer the other way — one run however many names come
// out. The **words agree in every column**, so the count and the variable are
// the whole of the tell (#4694).
//
// A redirection **target** is left out of this file on purpose. This column
// refuses a fanned target as an ambiguous redirect, and while doing so it runs
// the word's substitutions **twice per name** — 4 for `{x,y}`, 6 for
// `{x,y,w}`, 8 for `{a,b}{c,d}`, and 1 for the `{x}` that makes no fan. That
// is a count of its own and a question of its own; it is measured and filed
// rather than asserted here.

// countBashFanTicks counts whole lines rather than asking whether the mark is
// in there: the two readings produce the same *words*, so containment cannot
// tell them apart and only the count can.
func countBashFanTicks(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "TICK" {
			n++
		}
	}
	return n
}

const bashFanTickSubst = `$(echo TICK >&2; echo z)`

func TestBashABraceFanExpandsEachNameOnItsOwn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, words string
		runs             int
	}{
		{"two alternatives", "echo {x,y}" + bashFanTickSubst, "xz yz", 2},
		{"three alternatives", "echo {x,y,w}" + bashFanTickSubst, "xz yz wz", 3},
		{"a nested group", "echo {a,{b,c}}" + bashFanTickSubst, "az bz cz", 3},
		{"a range", "echo {1..3}" + bashFanTickSubst, "1z 2z 3z", 3},
		{"an empty alternative", "echo {,x}" + bashFanTickSubst, "z xz", 2},
		{"two groups", "echo {a,b}{c,d}" + bashFanTickSubst, "acz adz bcz bdz", 4},
		{"the group behind it", "echo " + bashFanTickSubst + "{x,y}", "zx zy", 2},
		// The row that makes the counts above falsifiable: a group that makes
		// no fan runs the word once in this column too, so a count of "one per
		// name" is not a count of "one per group".
		{"one alternative, which is no fan", "echo {x}" + bashFanTickSubst, "{x}z", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), tc.src+"\n")
			if n := countBashFanTicks(out); n != tc.runs {
				t.Errorf("%s = %q: %d runs, want %d", tc.src, out, n, tc.runs)
			}
			if !strings.Contains(out, tc.words) {
				t.Errorf("%s = %q, want the names %q", tc.src, out, tc.words)
			}
		})
	}
}

// The side effect, which is the half a count cannot be argued with: the
// variable an arithmetic substitution moves is moved once per name here, and
// the values substituted move with it.
func TestBashABraceFanMovesAVariableOncePerName(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "i=0\necho {x,y,w}$((i++))\necho \"i=$i\"\n")
	if !strings.Contains(out, "x0 y1 w2") || !strings.Contains(out, "i=3") {
		t.Errorf("out = %q, want `x0 y1 w2` with i=3", out)
	}
}

// And the pair that holds the noun fixed. `$e` comes to two words with no
// brace in the word at all, and its substitution runs **once** even in the
// column that repeats a fan — so a rule keyed on "a word that came to several
// words" would repeat this one too.
func TestBashSeveralWordsWithoutBracesIsNotAFan(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "e='p q'\necho $e"+bashFanTickSubst+"\n")
	if n := countBashFanTicks(out); n != 1 {
		t.Errorf("out = %q: %d runs, want 1 — splitting is not a fan", out, n)
	}
	braced, _ := runBash(t, t.TempDir(), "echo {p,q}"+bashFanTickSubst+"\n")
	if n := countBashFanTicks(braced); n != 2 {
		t.Errorf("braced = %q: %d runs, want 2 — the probe cannot fire", braced, n)
	}
}

// A failure ends the fan here too, and that is the row that says the stop is
// core rather than the axis: this column repeats the word for every name and
// still writes one diagnostic.
func TestBashAFailedExpansionEndsTheFan(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "echo {x,y,w}$(( 1/0 ))\n")
	if n := strings.Count(out, "division by 0"); n != 1 {
		t.Errorf("out = %q: %d diagnostics, want exactly 1", out, n)
	}
}
