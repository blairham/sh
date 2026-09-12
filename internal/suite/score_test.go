// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestOneExtraLineNearTheTopDoesNotZeroAFile is the property the whole
// measure exists for.
//
// Lining two outputs up by index is the obvious comparison and it is wrong in
// exactly one way that matters: a file that emits one extra line early agrees
// with the oracle on everything after it, and index comparison scores that
// zero and calls it a total failure. A longest common subsequence scores it
// at what it is, which is nearly right.
func TestOneExtraLineNearTheTopDoesNotZeroAFile(t *testing.T) {
	var want []string
	for i := range 100 {
		want = append(want, "line "+strconv.Itoa(i))
	}
	got := append([]string{"line 0", "an extra line nobody asked for"}, want[1:]...)

	common, longest, _ := agreement(got, want)
	rate := float64(common) / float64(longest)
	if rate < 0.98 {
		t.Fatalf("one extra line scored %.3f; an LCS should see the other 100 lines agree", rate)
	}
	// The positional reading, spelled out, so the test says what it is
	// guarding against rather than only that a number is high.
	positional := 0
	for i := range min(len(got), len(want)) {
		if got[i] == want[i] {
			positional++
		}
	}
	if float64(positional)/float64(len(want)) > 0.1 {
		t.Fatalf("the positional reading scored %d/%d, so this case no longer "+
			"distinguishes the two measures", positional, len(want))
	}
}

func TestAgreement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		a, b       string
		wantCommon int
		wantLong   int
	}{
		{"identical", "a\nb\nc\n", "a\nb\nc\n", 3, 3},
		{"nothing in common", "x\ny\n", "a\nb\nc\n", 0, 3},
		{"a line removed from the middle", "a\nc\n", "a\nb\nc\n", 2, 3},
		{"reordered", "b\na\n", "a\nb\n", 1, 2},
		{"both empty", "", "", 0, 0},
		{"one empty", "", "a\n", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common, longest, _ := agreement(lines(tc.a), lines(tc.b))
			if common != tc.wantCommon || longest != tc.wantLong {
				t.Errorf("got %d/%d, want %d/%d", common, longest, tc.wantCommon, tc.wantLong)
			}
		})
	}
}

func TestAgreementCapsAVeryLongOutputAndSaysSo(t *testing.T) {
	long := make([]string, maxLines+50)
	for i := range long {
		long[i] = strconv.Itoa(i)
	}
	_, _, capped := agreement(long, long)
	if !capped {
		t.Error("an output past the bound was compared whole and nothing said so")
	}
	_, _, capped = agreement(long[:10], long[:10])
	if capped {
		t.Error("a short output was reported as capped")
	}
}

// TestNormalizeReplacesAPathAndNotAWord is the trap `make wild` fell into
// first: replacing a shell's base name as well as its path turned unrelated
// words into <shell> and reported a difference between two identical outputs.
func TestNormalizeReplacesAPathAndNotAWord(t *testing.T) {
	const shell = "/opt/homebrew/bin/bash"
	out := normalize(shell+": line 3: oops\nGNU bashbug reads bash scripts\n", shell, "")
	if strings.Contains(out, shell) {
		t.Error("the shell's own path survived normalization")
	}
	if !strings.Contains(out, "GNU bashbug reads bash scripts") {
		t.Errorf("a word was rewritten as if it were a path: %q", out)
	}
}

func TestNormalizeRemovesTheDirectoryEachRunWasGiven(t *testing.T) {
	out := normalize("wrote /tmp/suite123/t/f and /var/folders/ab/cd/T/x\n", "/bin/sh", "/tmp/suite123/t")
	if strings.Contains(out, "suite123") || strings.Contains(out, "var/folders") {
		t.Errorf("a per-run directory survived: %q", out)
	}
}

func TestLinesDropsOnlyTheTrailingNewline(t *testing.T) {
	if got := len(lines("a\nb\n")); got != 2 {
		t.Errorf("a\\nb\\n is %d lines, want 2", got)
	}
	if got := len(lines("a\nb\n\n")); got != 3 {
		t.Errorf("a blank line the shell actually wrote was eaten: %d lines, want 3", got)
	}
}

func TestRatesAreZeroRatherThanNaNWithNothingScored(t *testing.T) {
	var rep Report
	for name, got := range map[string]float64{
		"strict": rep.StrictRate(), "parse": rep.ParseRate(), "line": rep.LineRate(),
	} {
		if math.IsNaN(got) || got != 0 {
			t.Errorf("%s rate over an empty run is %v, want 0", name, got)
		}
	}
}
