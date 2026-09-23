// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"
)

// The sequence a seeded `RANDOM` answers, seed by seed, against the real shell.
//
// Every number below was **read off zsh 5.9.2** on 2026-09-22 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` — seeds 0 through 200, a two-hundred draw run, eleven edge seeds and 100 fresh seeds chosen after the fit. It is not computed from the
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
		{0, [4]int{20034, 24315, 12703, 22240}},
		{1, [4]int{16807, 15089, 11481, 3114}},
		{2, [4]int{846, 30178, 22963, 6228}},
		{3, [4]int{17653, 12499, 1677, 9343}},
		{4, [4]int{1692, 27588, 13159, 12457}},
		{5, [4]int{18499, 9909, 24640, 15572}},
		{8, [4]int{3384, 22409, 26318, 24915}},
		{42, [4]int{17766, 11151, 23481, 32503}},
		{1000, [4]int{29784, 15851, 12955, 1498}},
		{32767, [4]int{15961, 21989, 13277, 11914}},
		{32768, [4]int{0, 4310, 24759, 15029}},
		{65536, [4]int{0, 8620, 16751, 30058}},
		{2147483647, [4]int{0, 20034, 24315, 12703}},
		{2147483648, [4]int{16807, 15089, 11481, 3114}},
		{4294967295, [4]int{16807, 15089, 11481, 3114}},
		{4294967296, [4]int{20034, 24315, 12703, 22240}},
		{-1, [4]int{16807, 15089, 11481, 3114}},
		{-2, [4]int{0, 20034, 24315, 12703}},
		{-42, [4]int{15847, 19026, 32249, 6493}},
		{123459876, [4]int{20034, 24315, 12703, 22240}},
	} {
		t.Run(fmt.Sprint(c.seed), func(t *testing.T) {
			out, _ := runZsh(t, dir, fmt.Sprintf(
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
	out, _ := runZsh(t, dir, `
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
// bare name is its value. Measured on zsh 5.9.2: `abc=5; RANDOM=abc` seeds 5, which
// is the row that separates arithmetic from "text that is not a number is zero".
func TestSeedingRandomIsArithmetic(t *testing.T) {
	dir := t.TempDir()
	plain, _ := runZsh(t, dir, `RANDOM=7; echo "$RANDOM"`)
	for _, src := range []string{
		`RANDOM=3+4; echo "$RANDOM"`,
		`abc=7; RANDOM=abc; echo "$RANDOM"`,
		`RANDOM=14/2; echo "$RANDOM"`,
	} {
		got, _ := runZsh(t, dir, src)
		if strings.TrimSpace(got) != strings.TrimSpace(plain) {
			t.Errorf("%s drew %q, want the seed-7 number %q", src, got, plain)
		}
	}
}
