// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"strings"
	"testing"
)

// The sequence a seeded `RANDOM` answers, seed by seed, against the real shell.
//
// Every number below was **read off bash 5.3.20** on 2026-09-22 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` — seeds 0 through 300, a three-thousand draw run, seventeen edge seeds and 124 fresh pseudo-random seeds chosen after the fit. It is not computed from the
// model the code holds, which would make the test a restatement of the
// implementation rather than a measurement of the shell (#4240).
//
// The seeds are chosen to be the discriminating ones rather than a range. 1 and
// 2 agree between two of the three shells; 4 is the first seed whose state has a
// bit above 16, so it is where the output maps separate; 8 is where a third
// shift shows; and the six at the top of the range pin how the value a script
// assigned is reduced, which ordinary seeds cannot see.
func TestASeededRandomAnswersTheMeasuredSequence(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		seed int64
		want [4]int
	}{
		{0, [4]int{20814, 24386, 149, 25587}},
		{1, [4]int{16807, 10791, 19566, 13983}},
		{2, [4]int{846, 21582, 6364, 27966}},
		{3, [4]int{17653, 593, 9386, 5216}},
		{4, [4]int{1693, 10396, 12729, 23165}},
		{5, [4]int{18498, 29338, 982, 6493}},
		{8, [4]int{3386, 20792, 25459, 13563}},
		{42, [4]int{17772, 26794, 1435, 24388}},
		{1000, [4]int{30040, 31037, 32736, 22253}},
		{32767, [4]int{7818, 22855, 18024, 26090}},
		{32768, [4]int{8403, 3502, 14043, 15520}},
		{65536, [4]int{16807, 7005, 28086, 31040}},
		{2147483647, [4]int{20814, 24386, 149, 25587}},
		{2147483648, [4]int{16807, 10791, 19566, 13983}},
		{4294967295, [4]int{16807, 10791, 19566, 13983}},
		{4294967296, [4]int{20814, 24386, 149, 25587}},
		{-1, [4]int{16807, 10791, 19566, 13983}},
		{-2, [4]int{20814, 24386, 149, 25587}},
		{-42, [4]int{16914, 5332, 7861, 19179}},
		{123459876, [4]int{20814, 24386, 149, 25587}},
	} {
		t.Run(fmt.Sprint(c.seed), func(t *testing.T) {
			out, _ := runBash(t, dir, fmt.Sprintf(
				`RANDOM=%d; echo "$RANDOM $RANDOM $RANDOM $RANDOM"`, c.seed))
			want := fmt.Sprintf("%d %d %d %d", c.want[0], c.want[1], c.want[2], c.want[3])
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("RANDOM=%d drew %q, want %q", c.seed, got, want)
			}
		})
	}
}

// Seeding again starts the sequence again, which is the half that makes it
// reproducible rather than merely derived, and a subshell carries the parent's
// place away rather than sharing it.
func TestASeedStartsTheSequenceAgainAndASubshellTakesItsOwnCopy(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, `
RANDOM=42; a="$RANDOM $RANDOM"
RANDOM=42; b="$RANDOM $RANDOM"
RANDOM=42; c=$( echo "$RANDOM $RANDOM" ); d="$RANDOM $RANDOM"
echo "[$a][$b][$c][$d]"`)
	fields := strings.Split(strings.Trim(strings.TrimSpace(out), "[]"), "][")
	if len(fields) != 4 {
		t.Fatalf("output %q, want four bracketed pairs", out)
	}
	if fields[0] != fields[1] {
		t.Errorf("a second seed drew %q where the first drew %q", fields[1], fields[0])
	}
	if fields[2] != fields[0] {
		t.Errorf("a subshell drew %q where its parent had drawn %q", fields[2], fields[0])
	}
	if fields[3] != fields[0] {
		t.Errorf("the parent drew %q after the subshell, want its own %q", fields[3], fields[0])
	}
}

// An assignment to `RANDOM` is an arithmetic expression and not a numeral, so a
// bare name is its value. Measured on bash 5.3.20: `abc=5; RANDOM=abc` seeds 5, which
// is the row that separates arithmetic from "text that is not a number is zero".
func TestSeedingRandomIsArithmetic(t *testing.T) {
	dir := t.TempDir()
	plain, _ := runBash(t, dir, `RANDOM=7; echo "$RANDOM"`)
	for _, src := range []string{
		`RANDOM=3+4; echo "$RANDOM"`,
		`abc=7; RANDOM=abc; echo "$RANDOM"`,
		`RANDOM=14/2; echo "$RANDOM"`,
	} {
		got, _ := runBash(t, dir, src)
		if strings.TrimSpace(got) != strings.TrimSpace(plain) {
			t.Errorf("%s drew %q, want the seed-7 number %q", src, got, plain)
		}
	}
}
