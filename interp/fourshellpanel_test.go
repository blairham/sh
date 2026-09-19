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
// That sentence is the first one the file's own pass corrected: dash and
// BusyBox ash both say 2, and dialect/ash already held 2 throughout. Nine of
// its lines were measured across all seven columns on 2026-09-19 and
// rewritten, which is what takes the budget from 54 to 45 — every dialect
// value checked was right, so what the pass produced is a corrected record
// rather than a defect. Two findings were not about ash at all: the same bash
// binary called `sh` is silent where bash announces an empty hash table, and
// drops the `alias ` prefix from a listing, so two fields documented as
// "bash" are bash-as-bash.
//
// A second pass the same day measured eight more across all seven columns and
// takes the budget from 45 to 37. Every dialect value it checked was right
// again, so it too is a corrected record; what it changed is what a reader is
// told. Three of the eight were wrong about more than a count:
//
//   - ReturnOutsideAFunction said "the other three obey it and end the
//     script". Four columns obey it, and bash — the shell the sentence was
//     about — does not: it complains and runs the next command at status 0,
//     so it was in neither group while appearing to be the subject of both.
//   - TrapBadSignal said status 1 was "the only part of this any two of them
//     agree on". BusyBox ash writes bash's wording and ksh93 writes dash's,
//     so the wordings pair up across the family lines and the sentence was
//     false in exactly the direction the missing column supplies.
//   - UnboundPositional said "true of three of the four", and the count was
//     right by accident: the three are dash, zsh and ash, and ksh93 does not
//     refuse an unset positional under `set -u` at all. A column with nothing
//     to word had been filed as a column that words it the same way.
//
// LowercaseReason is the one that had to be measured twice, and it is the
// reminder that a probe has to discriminate: ash's other reasons read
// lowercase and look exactly like that flag, and only a reason it does not
// substitute — Permission denied — tells the two apart. It capitalizes, so
// zsh really is alone.
//
// # The panel is seven columns and one of them is an invocation
//
// Two of this pass's eight had to be corrected *after* they were written,
// because the first probe set `argv[0]` on `env` rather than on the shell it
// was about to exec — so what it called bash-as-`sh` was an ordinary bash and
// the column read as a duplicate of the one beside it. Running the same binary
// through a link named `sh` puts it in POSIX mode, and two of the eight split:
//
//   - ReturnOutsideAFunction: bash and bash 3.2 complain and run the next
//     command at 0; the same binary called `sh` complains and ends the script
//     at 2. A third behavior neither group holds.
//   - TrapPrintsSignalPrefix: bash and bash 3.2 write `SIGINT`; the same
//     binary called `sh` writes `INT`, with everybody else.
//
// That is the same class as the previous pass's `HashEmptyTable` and
// `AliasListPrefix`, and it is worth stating in its own right: **the column a
// four-shell sentence leaves out is usually BusyBox ash and is not always**.
// #3228's own framing says "almost always", and a reader who takes that as
// "always" will check ash, find it agrees, and file the sentence as correct
// while a bash column two rows over disagrees. A probe that cannot tell
// bash-as-`sh` from bash cannot see any of these, and it looks exactly like a
// probe that can.
//
// Per file rather than one total, because a single number lets a file that
// gets worse hide behind a file that gets better — and these three are worked
// on separately, so that trade would be made by accident rather than chosen.
var fourShellPhraseBudget = map[string]int{
	"semantics.go":   41,
	"diagnostics.go": 37,
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
