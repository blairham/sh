// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `ambiguous redirect` is a complaint about **how many words** a target came
// to, so a target whose expansion *failed* has nothing to be ambiguous about:
// it never produced a count. One failure, one diagnostic.
//
// The reading that refuses a target for its count is the only one that can
// reach this, and it wrote the failure's own sentence and then a second line
// about the word nobody could compute.
//
// Measured 2026-09-26, `-c`, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// counting *occurrences* on standard error rather than asking whether the
// text is in there — against `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash`
// 3.2.57, `/opt/homebrew/bin/zsh -f` 5.9.2, `/bin/ksh` 93u+, `/bin/dash`
// 0.5.12 and BusyBox 1.37.0 in the pinned alpine image. `go version -m` says
// *not a Go executable* for each of the five paths.
//
//	: > $(( 1/0 ))       lines   `ambiguous redirect`
//	bash 5.3.20              1   0
//	bash 3.2.57              1   0
//	zsh 5.9.2                1   0
//	ksh93u+                  1   0
//	dash 0.5.12              1   0
//	BusyBox ash 1.37.0       1   0
//
// Nobody writes it, and the column that writes it at all is the one that
// writes the first line too (#4688).

// ambiguousTargets is the reading that refuses a target for its count, with
// the failure axes answered the way the column holding that reading answers
// them: a failed expansion gives up the line rather than the shell.
func ambiguousTargets(dir string) func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		sem.SplitParamExpansion = Yes
		sem.SplitCommandSubstitution = Yes
		sem.GlobExpansionResults = Yes
		sem.GlobNoMatchIsError = No
		sem.RedirectTargetIsAnOrdinaryWord = Yes
		sem.RedirectTargetTakesPathnameExpansion = Yes
		sem.BraceExpansion = Yes
		sem.FailedExpansionAbandonsTheLine = Yes
		sem.FatalErrorStatusIsOne = Yes
		dg := Diagnostics{AmbiguousRedirect: "%[1]s: ambiguous redirect"}
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
	}
}

// countAmbiguous is the whole point of this file: a `Contains` assertion
// cannot tell one diagnostic from two, and two is what the bug was.
func countAmbiguous(out string) int {
	return strings.Count(out, "ambiguous redirect")
}

// The noun: **the failure** decides, not the count.
//
// Both halves of this pair leave a target of no words, and only one of them
// failed on the way. The reference writes `ambiguous redirect` for the first
// and not for the second, so a rule keyed on the count would get one of them
// wrong — which is the shape this seam keeps producing.
func TestAFailedTargetExpansionIsNotAnAmbiguousRedirect(t *testing.T) {
	t.Run("no words, and nothing failed: ambiguous", func(t *testing.T) {
		out, st := run(t, `e=; : > $e`+"\n"+`echo "after st=$?"`, ambiguousTargets(t.TempDir()))
		if n := countAmbiguous(out); n != 1 {
			t.Errorf("out = %q: %d ambiguous lines, want exactly 1", out, n)
		}
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("out = %q (status %d), want the next line at 1", out, st)
		}
	})
	t.Run("no words, because the expansion failed: not ambiguous", func(t *testing.T) {
		out, st := run(t, `: > $(( 1/0 ))`+"\n"+`echo "after st=$?"`, ambiguousTargets(t.TempDir()))
		if n := countAmbiguous(out); n != 0 {
			t.Errorf("out = %q: %d ambiguous lines, want none — the failure is the diagnostic", out, n)
		}
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("out = %q (status %d), want the next line at 1", out, st)
		}
	})
}

// The other half of the same pair, and the one that says the count is not the
// noun even when the count is genuinely wrong: `$e` is two words and is
// ambiguous, and `$e$(( 1/0 ))` is two words that never got made because the
// expansion failed first. The reference writes one line for the second.
func TestAFailedTargetExpansionWinsOverAWrongWordCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"several words, nothing failed", `e="a b"; : > $e`, 1},
		{"several words, and the expansion failed", `e="a b"; : > $e$(( 1/0 ))`, 0},
		{"braces make several, nothing failed", `: > {x,y}`, 1},
		{"braces make several, and the expansion failed", `: > {x,y}$(( 1/0 ))`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\n"+`echo "after st=$?"`, ambiguousTargets(t.TempDir()))
			if n := countAmbiguous(out); n != tc.want {
				t.Errorf("out = %q: %d ambiguous lines, want %d", out, n, tc.want)
			}
		})
	}
}

// Every failure kind the target's expansion can arrive at, because the rule is
// about the failure and not about which one it was.
func TestEveryFailedTargetExpansionKindIsOneDiagnostic(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an arithmetic error", `: > $(( 1/0 ))`},
		{"a parameter with a word of its own", `: > ${q?bad}`},
		{"an unset name under nounset", `set -u; : > $NOPE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, ambiguousTargets(t.TempDir()))
			if n := countAmbiguous(out); n != 0 {
				t.Errorf("out = %q: %d ambiguous lines, want none", out, n)
			}
			if strings.TrimSpace(out) == "" {
				t.Errorf("out = %q, want the failure's own sentence — the probe cannot fire", out)
			}
			if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != 1 {
				t.Errorf("out = %q, want exactly one line of diagnostic, got %d", out, lines)
			}
		})
	}
}

// And every operator that takes a target, because the doubling was in the
// target path and so reached all of them. A here-string is deliberately absent
// — its word is a *body*, which goes through another door entirely and was
// right all along.
func TestEveryRedirectionOperatorsFailedTargetIsOneDiagnostic(t *testing.T) {
	for _, op := range []string{`>`, `>>`, `<`, `<>`, `2>`, `&>`} {
		t.Run(op, func(t *testing.T) {
			out, _ := run(t, `: `+op+` $(( 1/0 ))`, ambiguousTargets(t.TempDir()))
			if n := countAmbiguous(out); n != 0 {
				t.Errorf("out = %q: %d ambiguous lines, want none", out, n)
			}
		})
	}
	t.Run("exec", func(t *testing.T) {
		out, _ := run(t, `exec 3> $(( 1/0 ))`, ambiguousTargets(t.TempDir()))
		if n := countAmbiguous(out); n != 0 {
			t.Errorf("out = %q: %d ambiguous lines, want none", out, n)
		}
	})
}

// And the reach and the status are not what moved, which is asserted rather
// than assumed: the command does not run, the redirection is still refused,
// and the next line still reads 1.
func TestAFailedTargetExpansionKeepsItsReachAndStatus(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a special builtin", `: > $(( 1/0 ))`},
		{"a regular builtin", `echo RAN > $(( 1/0 ))`},
		{"a function", `f() { echo RAN; }; f > $(( 1/0 ))`},
		{"a group", `{ echo RAN; } > $(( 1/0 ))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src+"\n"+`echo "after st=$?"`, ambiguousTargets(t.TempDir()))
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the command left unrun", out)
			}
			if !strings.Contains(out, "after st=1") || st != 0 {
				t.Errorf("out = %q (status %d), want the next line at 1", out, st)
			}
		})
	}
}
