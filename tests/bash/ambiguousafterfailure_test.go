// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// This shell is the one column that says `ambiguous redirect` at all, and the
// complaint is about **how many words** a target came to. A target whose
// expansion *failed* never got a count, so what the script is told is the
// failure's own sentence and nothing else.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash`
// 3.2.57 — `go version -m` says *not a Go executable* for both — `-c`, under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, counting **occurrences** of
// the sentence on standard error rather than asking whether it is in there.
// Both versions answer alike on every row below; this is a wording-and-count
// question and the two have split on one elsewhere in this campaign, so it was
// asked of each (#4688).
//
//	                              stderr lines   `ambiguous redirect`
//	: > $(( 1/0 ))                           1   0
//	: > ${q?bad}                             1   0
//	set -u; : > $NOPE                        1   0
//	e=;      : > $e                          1   1
//	e="a b"; : > $e                          1   1
//	e="a b"; : > $e$(( 1/0 ))                1   0
//
// The last three are the pair that names the noun. Holding the count fixed and
// moving only whether the expansion failed moves the answer, so it is the
// **failure** that decides and not the count — a rule keyed on "no words"
// would lose row four, and one keyed on "the wrong number of words" would lose
// row six.

// countAmbiguous is why this file exists: a Contains assertion reads one
// diagnostic and two identically, and two was the defect.
func countAmbiguous(out string) int {
	return strings.Count(out, "ambiguous redirect")
}

func TestAFailedRedirectionTargetIsOneDiagnosticHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"an arithmetic error", `: > $(( 1/0 ))`, 0},
		{"a parameter with a word of its own", `: > ${q?bad}`, 0},
		{"an unset name under nounset", `set -u; : > $NOPE`, 0},
		// The positive rows, so that the zeros above are falsifiable: a count
		// this shell really does refuse still says so, exactly once.
		{"no words, and nothing failed", `e=; : > $e`, 1},
		{"several words, and nothing failed", `e="a b"; : > $e`, 1},
		{"braces make several, and nothing failed", `: > {x,y}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), tc.src+"\n")
			if n := countAmbiguous(out); n != tc.want {
				t.Errorf("%s = %q: %d occurrences, want %d", tc.src, out, n, tc.want)
			}
		})
	}
}

// The pair that says which noun the rule is keyed on, held apart deliberately:
// each row below has a *wrong count* and a failure on the way to it, and this
// shell reports only the failure. A rule keyed on the count reports both.
func TestAFailedTargetExpansionOutranksItsWordCountHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"several words and a failure", `e="a b"; : > $e$(( 1/0 ))`},
		{"a failure and several words", `e="a b"; : > $(( 1/0 ))$e`},
		{"braces and a failure", `: > {x,y}$(( 1/0 ))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), tc.src+"\n")
			if n := countAmbiguous(out); n != 0 {
				t.Errorf("%s = %q: %d occurrences, want none", tc.src, out, n)
			}
			if lines := strings.Count(out, "\n"); lines != 1 {
				t.Errorf("%s = %q, want exactly one line — the failure's own", tc.src, out)
			}
		})
	}
}

// Every operator that takes a target, because the second line came from the
// target path and so reached all of them. A here-string's word is a *body*
// rather than a target and was right throughout, which is the control.
func TestEveryRedirectionOperatorsFailedTargetIsOneDiagnosticHere(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`: > $(( 1/0 ))`,
		`: >> $(( 1/0 ))`,
		`: < $(( 1/0 ))`,
		`: <> $(( 1/0 ))`,
		`: 2> $(( 1/0 ))`,
		`: &> $(( 1/0 ))`,
		`exec 3> $(( 1/0 ))`,
		`: <<< $(( 1/0 ))`,
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), src+"\n")
			if n := countAmbiguous(out); n != 0 {
				t.Errorf("%s = %q: %d occurrences, want none", src, out, n)
			}
			if lines := strings.Count(out, "\n"); lines != 1 {
				t.Errorf("%s = %q, want exactly one line", src, out)
			}
		})
	}
}

// And on every command kind, with the reach and the status asserted beside the
// count: the command does not run, the rest of the line goes with the failure,
// and the next line reads 1. Those were right before and are what a change to
// the diagnostic must not move.
func TestAFailedRedirectionTargetKeepsItsReachAndStatusHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a special builtin", `: > $(( 1/0 ))`},
		{"a regular builtin", `echo RAN > $(( 1/0 ))`},
		{"a function", `f() { echo RAN; }; f > $(( 1/0 ))`},
		{"a group", `{ echo RAN; } > $(( 1/0 ))`},
		{"a loop", `for i in 1; do echo RAN; done > $(( 1/0 ))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st := runBash(t, t.TempDir(), tc.src+"\necho \"after st=$?\"\n")
			if strings.Contains(out, "RAN") {
				t.Errorf("%s = %q, want the command left unrun", tc.src, out)
			}
			if !strings.Contains(out, "after st=1") || st != 0 {
				t.Errorf("%s = %q (status %d), want the next line at 1", tc.src, out, st)
			}
			if n := countAmbiguous(out); n != 0 {
				t.Errorf("%s = %q: %d occurrences, want none", tc.src, out, n)
			}
		})
	}
}
