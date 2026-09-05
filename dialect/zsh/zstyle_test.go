// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The styles database, measured against zsh 5.9.2 on 2026-09-05 with a scratch
// HOME and no startup files. Every want below is a line that binary printed.

// TestAStyleIsStoredAndSaidBack is the honest minimum the issue asked for: an
// rc file that sets styles for a completion system this shell has not got runs
// to the end, and the styles are there afterwards.
func TestAStyleIsStoredAndSaidBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zstyle ':completion:*' verbose yes; echo ok\nzstyle -L\n")
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	want := "ok\nzstyle ':completion:*' verbose yes\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheListingIsStyleThenPattern pins both halves of the order at once: the
// styles come out alphabetically whatever order they were set in, and the
// patterns under each in the specificity order the lookup uses.
func TestTheListingIsStyleThenPattern(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':b:*' sty B; zstyle ':a:*' sty A; zstyle ':c:*' other C\nzstyle -L\n")
	want := "zstyle ':c:*' other C\nzstyle ':b:*' sty B\nzstyle ':a:*' sty A\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestMoreComponentsSortFirst is the primary key, and the case that says it is
// the component count rather than the length of the literal part: a
// one-component pattern eight characters long still sorts after a
// three-component one two characters long.
func TestMoreComponentsSortFirst(t *testing.T) {
	for _, order := range []string{
		"zstyle ':a:b:c:d:*' v 1; zstyle ':q:*' v 2; zstyle ':m:n:*' v 3\n",
		"zstyle ':q:*' v 2; zstyle ':m:n:*' v 3; zstyle ':a:b:c:d:*' v 1\n",
	} {
		out, _ := runZsh(t, t.TempDir(), order+"zstyle -L\n")
		want := "zstyle ':a:b:c:d:*' v 1\nzstyle ':m:n:*' v 3\nzstyle ':q:*' v 2\n"
		if out != want {
			t.Errorf("for %q output = %q, want %q", order, out, want)
		}
	}
	out, _ := runZsh(t, t.TempDir(), "zstyle 'abcdefgh*' v 1; zstyle ':a:*' v 2\nzstyle -L\n")
	if want := "zstyle ':a:*' v 2\nzstyle 'abcdefgh*' v 1\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestAnExactPatternSortsBeforeAWildcardOne is the second key — and the case
// that keeps it *subordinate* to the first, which is what makes it a
// tie-break rather than the rule. `:x:y:z:*` has more components than the
// exact `:a:b:c` and still wins.
func TestAnExactPatternSortsBeforeAWildcardOne(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zstyle 'a*b' v 1; zstyle 'a?b' v 2; zstyle 'ab' v 3\nzstyle -L\n")
	if want := "zstyle ab v 3\nzstyle 'a*b' v 1\nzstyle 'a?b' v 2\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	out, _ = runZsh(t, t.TempDir(), "zstyle ':a:b:c' v 2; zstyle ':x:y:z:*' v 1\nzstyle -L\n")
	if want := "zstyle ':x:y:z:*' v 1\nzstyle :a:b:c v 2\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheMostSpecificPatternWins is the reason the order matters at all, and
// it is asserted from both insertion orders because an implementation that
// simply took the first or the last one set would pass one of them.
func TestTheMostSpecificPatternWins(t *testing.T) {
	for _, order := range []string{
		"zstyle ':a:*' v general; zstyle ':a:b:*' v specific\n",
		"zstyle ':a:b:*' v specific; zstyle ':a:*' v general\n",
	} {
		out, _ := runZsh(t, t.TempDir(), order+"zstyle -s ':a:b:c' v out; echo \"[$out]\"\n")
		if out != "[specific]\n" {
			t.Errorf("for %q output = %q, want %q", order, out, "[specific]\n")
		}
	}
	// And the fallback: a bare `*` is one component, so it sorts last and is
	// what a context nothing else claims reads.
	out, _ := runZsh(t, t.TempDir(),
		"zstyle '*' v star; zstyle ':a:*' v a\nzstyle -s ':a:b' v out; echo \"[$out]\"\nzstyle -s ':z' v two; echo \"[$two]\"\n")
	if want := "[a]\n[star]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRetrievalReadsTheValues covers `-g`, `-s` and `-a` together, because the
// three differ only in what shape the answer arrives in.
func TestRetrievalReadsTheValues(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "zstyle ':a:*' v x y z\nzstyle -s ':a:b' v s; echo \"[$s]\"\n"+
		"zstyle -s ':a:b' v d '-'; echo \"[$d]\"\nzstyle -a ':a:b' v arr; echo \"($arr)\"\n"+
		"zstyle -g g ':a:*' v; echo \"($g)\"\n")
	want := "[x y z]\n[x-y-z]\n(x y z)\n(x y z)\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// A style with exactly one value, which is the shape the rest of the file
	// never asks for and the one that catches an answer written into the
	// array table at the wrong subscript: this shell counts from 1, and a
	// single value put at 0 reads back as nothing at all while two values put
	// at 0 and 1 read back looking right.
	out, _ = runZsh(t, dir, "zstyle ':a:*' one only\nzstyle -a ':a:b' one arr; echo \"($arr)\"\n"+
		"zstyle -g g ':a:*' one; echo \"($g)\"\n")
	if want := "(only)\n(only)\n"; out != want {
		t.Errorf("single-value output = %q, want %q", out, want)
	}
}

// TestGReadsThePatternRatherThanMatchingIt is the one retrieval that does not
// match, measured: `-g` given `:a:b` finds nothing when the style was set for
// `:a:*`, where `-s` given the same context finds it.
func TestGReadsThePatternRatherThanMatchingIt(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' v hello\nzstyle -g g ':a:b' v; echo \"g=$? ($g)\"\nzstyle -s ':a:b' v s; echo \"s=$? [$s]\"\n")
	if want := "g=1 ()\ns=0 [hello]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheTestFormsHaveThreeStatuses pins what makes `-t` worth having: 0 true,
// 1 false and **2 for a style nobody set**, so a reader can tell "off" from
// "unsaid" — and `-T`, which is the same test with the unsaid case true.
func TestTheTestFormsHaveThreeStatuses(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' v true; zstyle ':a:*' w no\n"+
			"zstyle -t ':a:b' v; echo \"true=$?\"\nzstyle -t ':a:b' w; echo \"false=$?\"\n"+
			"zstyle -t ':zz' v; echo \"unset=$?\"\nzstyle -T ':zz' v; echo \"unsetT=$?\"\n")
	if want := "true=0\nfalse=1\nunset=2\nunsetT=0\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestOnlyFourWordsAreTrue is measured rather than guessed: a style set to
// something that is not one of them is false, not "set".
func TestOnlyFourWordsAreTrue(t *testing.T) {
	src := ""
	for _, w := range []string{"true", "yes", "on", "1", "no", "off", "random", "0"} {
		src += "zstyle ':a' v " + w + "; zstyle -t ':a' v; echo \"" + w + "=$?\"\n"
	}
	out, _ := runZsh(t, t.TempDir(), src)
	want := "true=0\nyes=0\non=0\n1=0\nno=1\noff=1\nrandom=1\n0=1\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestDeleteTakesTheWholeTableOrPartOfIt covers `-d`'s three shapes, and that
// a pattern nobody set is still 0 — deleting nothing is not a failure.
func TestDeleteTakesTheWholeTableOrPartOfIt(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' v 1; zstyle ':a:*' w 2; zstyle ':b:*' x 3\n"+
			"zstyle -d ':a:*' v; echo \"one=$?\"\nzstyle -L\n"+
			"zstyle -d ':a:*'; zstyle -L\nzstyle -d ':nothing' v; echo \"none=$?\"\n"+
			"zstyle -d; zstyle -L; echo end\n")
	want := "one=0\nzstyle ':a:*' w 2\nzstyle ':b:*' x 3\nzstyle ':b:*' x 3\nnone=0\nend\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestAnEvaluatedStyleIsRunWhenItIsRead is `-e`: the value is code that sets
// `reply`, and it runs at lookup rather than at the time it was set.
func TestAnEvaluatedStyleIsRunWhenItIsRead(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"n=one\nzstyle -e ':a:*' v 'reply=($n two)'\nn=changed\n"+
			"zstyle -a ':a:b' v arr; echo \"($arr)\"\nzstyle -L\n")
	want := "(changed two)\nzstyle -e ':a:*' v 'reply=($n two)'\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheBareCommandGroupsByStyle is the other listing shape, which is what a
// person gets for typing the command with nothing after it.
func TestTheBareCommandGroupsByStyle(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle\nzstyle ':b:*' sty B; zstyle ':a:*' sty A; zstyle ':c:*' other C\nzstyle\n")
	want := "other\n        :c:* C\nsty\n        :b:* B\n        :a:* A\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheListingQuotesOnlyWhatNeedsIt pins the spelling `-L` uses, including
// that the quote set is not the metacharacter set: `~` sorts as exact and is
// still quoted.
func TestTheListingQuotesOnlyWhatNeedsIt(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' v 'hello world'\nzstyle ':b' w '%d {x}'\nzstyle ':c' y ''\n"+
			"zstyle 'a~b' z 1\nzstyle ':d' q \"it's\"\nzstyle -L\n")
	want := "zstyle :d q 'it'\\''s'\n" +
		"zstyle ':a:*' v 'hello world'\n" +
		"zstyle :b w '%d {x}'\n" +
		"zstyle :c y ''\n" +
		"zstyle 'a~b' z 1\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestZstyleRefusesInItsOwnWords keeps the two builtins' wordings apart. This
// one says `invalid option`; `bindkey` says `bad option`, measured in both,
// and a shared helper would quietly make them agree.
func TestZstyleRefusesInItsOwnWords(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zstyle -z ':a' v\n")
	if !strings.Contains(out, "invalid option: -z") {
		t.Errorf("output = %q, want the option refused this builtin's way", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if !strings.Contains(out, ":zstyle:") {
		t.Errorf("output = %q, want the builtin named in the location", out)
	}
	for _, short := range []string{"zstyle ':a:*'\n", "zstyle -s ':a'\n", "zstyle -g\n"} {
		out, st := runZsh(t, t.TempDir(), short)
		if !strings.Contains(out, "not enough arguments") || st != 1 {
			t.Errorf("%q gave %q at %d, want the usage complaint at 1", short, out, st)
		}
	}
}

// TestStylesAreASubshellsOwn is why the table lives in the Runner rather than
// in a package variable: a clone must not write through to its parent.
func TestStylesAreASubshellsOwn(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':y:*' w 1\n(zstyle ':x:*' v inner; zstyle -d ':y:*')\nzstyle -L\n")
	if want := "zstyle ':y:*' w 1\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestThePatternIsAPatternAndTheStyleIsNot is the asymmetry a reader would not
// assume: the context is matched against the pattern, and the style name is
// compared for equality.
func TestThePatternIsAPatternAndTheStyleIsNot(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' verbose yes\nzstyle -s ':a:b:c' verbose hit; echo \"[$hit]\"\n"+
			"zstyle -s ':a:b:c' verb miss; echo \"[$miss]\"\n")
	if want := "[yes]\n[]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTiedPatternsKeepTheOrderTheyWereSetIn is the third sort key, and it is
// asserted over enough patterns to actually depend on the sort being stable.
//
// Four or five would not: an unstable sort of a short slice is stable in
// practice, so a handful of tied rows come out right by luck and the mutant
// that drops the guarantee survives. Sixteen is past where that holds.
func TestTiedPatternsKeepTheOrderTheyWereSetIn(t *testing.T) {
	// Two component counts, interleaved, and enough of each that the sort has
	// real work to do. Both halves matter. A run of rows that are *all* tied
	// is a sort with nothing to compare, which every algorithm leaves alone —
	// so a table like that keeps its order by accident and says nothing about
	// stability. Interleaving a key the sort must act on is what makes the
	// ties something it could disturb.
	set, deep, shallow := "", "", ""
	for _, name := range []string{
		"p", "d", "k", "a", "z", "m", "c", "w", "b", "y", "e", "n", "j", "t", "g", "r",
	} {
		set += "zstyle ':" + name + ":x:*' v deep-" + name + "\n"
		set += "zstyle ':" + name + ":*' v flat-" + name + "\n"
		deep += "zstyle ':" + name + ":x:*' v deep-" + name + "\n"
		shallow += "zstyle ':" + name + ":*' v flat-" + name + "\n"
	}
	out, _ := runZsh(t, t.TempDir(), set+"zstyle -L\n")
	if want := deep + shallow; out != want {
		t.Errorf("output = %q, want the deeper patterns first and each group in the order it was set, %q", out, want)
	}
}

// TestResettingAStyleKeepsItsPlace is what makes a listing stable across an rc
// file that sets the same style twice — measured, the second set replaces the
// value and does not move the row.
func TestResettingAStyleKeepsItsPlace(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zstyle ':a:*' v 1; zstyle ':b:*' v 2; zstyle ':a:*' v 3\nzstyle -L\n")
	if want := "zstyle ':a:*' v 3\nzstyle ':b:*' v 2\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
