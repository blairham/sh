// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The panel has seven columns and the dialect set has five, and for a long
// time the axis doc comments were written as though both were four.
//
// It is not a style complaint. A comment that says what four shells do is also
// a claim about what our fifth dialect should answer, and a reader who
// consults one gets a group with a column missing from it. That is how #3226
// happened: EchoExpandsHexEscapes read "bash and zsh do, dash and ksh93 print
// it as written", BusyBox ash — which expands it — was in neither list, and
// the value was left at a default saying it does not, while the corpus row
// beside it already held the right answer. Reading the same comments against
// the shell turned up four more live defects: #3237, #3238, #3239, #3245.
//
// The drift is mechanical, so the guard is a ratchet rather than a rule: these
// phrases may only become rarer. A new one cannot be added, and the count goes
// down as the remaining comments are measured and rewritten. #3228 is the
// campaign; this is what keeps the next column addition from starting it
// again.
//
// Deliberately a count and not a ban. A number of the matches are honest —
// "all four combinations", "a type of its own with four answers", "the four
// columns without the construct" — and a rule that could not tell those from a
// miscount would be satisfied by rewording rather than by measuring. A ceiling
// that only falls needs no such judgement, and the budgets below are the whole
// audit trail.
//
// # One budget per file, and the third file
//
// interp/diagnostics.go was outside this guard until #3228 was re-read
// against it, and it held **54** of these lines — more than the two guarded
// files together. Its Answer axes are the same kind of claim in the same
// words: "CdStatus is what that reports. dash says 2 and the other three say
// 1." A ratchet that covers half a population is not a ratchet; new prose
// simply lands in the half it does not read.
//
// Per file rather than one total, because a single number lets a file that
// gets worse hide behind a file that gets better — and these three are worked
// on separately, so that trade would be made by accident rather than chosen.
var fourShellPhraseBudget = map[string]int{
	"semantics.go":   41,
	"diagnostics.go": 54,
	filepath.Join("..", "syntax", "dialect.go"): 11,
}

// fourShellPanel is the phrase set from the audit that filed #3228. Each one
// names a panel of four where the panel is seven.
var fourShellPanel = regexp.MustCompile(
	`\bthe other three\b|\bthree of the four\b|\ball four\b|\bfour answers\b|` +
		`\bthe four (?:columns|shells|of them|dialects)\b`)

func TestNoNewFourShellPanelInAnAxisDoc(t *testing.T) {
	for _, path := range slices.Sorted(maps.Keys(fourShellPhraseBudget)) {
		budget := fourShellPhraseBudget[path]
		n := fourShellPanelLines(t, path)
		t.Logf("%s: %d of %d", path, n, budget)
		switch {
		case n > budget:
			t.Errorf("%s: %d doc lines describe a four-shell panel, where the budget is %d.\n"+
				"The panel is seven columns and the dialect set is five. Name the "+
				"columns, or measure the one the sentence leaves out — it is "+
				"almost always BusyBox ash, and it is the column this project has "+
				"had wrong five separate times (#3228).", path, n, budget)
		case n < budget:
			t.Errorf("%s: %d doc lines describe a four-shell panel, where the budget is %d.\n"+
				"Lower this file's entry in fourShellPhraseBudget to %d: the ratchet "+
				"only holds while the number it holds is the real one.", path, n, budget, n)
		}
	}
}

// fourShellPanelLines counts the comment lines in one file that describe a
// panel of four.
//
// A file that cannot be read is a failure rather than a zero. A ratchet whose
// count silently becomes nought on a rename is a green check for a rule that
// stopped applying, which is the same shape as the population it was not
// reading.
func fourShellPanelLines(t *testing.T, path string) int {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var n int
	for _, line := range strings.Split(string(src), "\n") {
		// Comment lines only. The phrases are prose, and a string literal
		// holding one would be a wording rather than a claim about the panel.
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if fourShellPanel.MatchString(line) {
			n++
		}
	}
	return n
}
