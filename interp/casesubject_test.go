// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `case` whose subject or pattern could not be expanded runs no arm.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file, against bash 5.3.15 at
// /opt/homebrew/bin/bash, dash at /bin/dash, ksh93u+ at /bin/ksh and zsh
// 5.9.2 at /opt/homebrew/bin/zsh. Six shapes — `${p@@@x}` and an unset name
// under `set -u` and `$((1/0))`, each in the subject and in a pattern — and
// in none of the twenty-four columns does a branch run.
//
// The subject half is #1215's, and this is the rest of it: a **pattern** that
// could not be read was consumed as "did not match", so the shell walked on
// and ran a later arm at status 0 — the diagnostic printed and then a branch
// fired that no shell would have fired.
//
// Whether the *script* goes on afterwards is FailedExpansionAbandonsTheLine's
// question and not a question about `case`, which is why nothing here asserts
// it: every dialect matches its own reference on all six shapes once no arm
// runs.
func TestAFailedExpansionInACaseRunsNoArm(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a bad substitution in the subject", `p="a*"; case ${p@@@x} in a*) printf hit;; *) printf miss;; esac`},
		{"a bad substitution in a pattern", `p="a*"; case abc in ${p@@@x}) printf hit;; *) printf miss;; esac`},
		{"an unset name in the subject", `set -u; case ${NOPEV} in a*) printf hit;; *) printf miss;; esac`},
		{"an unset name in a pattern", `set -u; case abc in ${NOPEV}) printf hit;; *) printf miss;; esac`},
		{"a division by zero in the subject", `case $((1/0)) in a*) printf hit;; *) printf miss;; esac`},
		{"a division by zero in a pattern", `case abc in $((1/0))) printf hit;; *) printf miss;; esac`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if strings.Contains(out, "hit") || strings.Contains(out, "miss") {
				t.Errorf("out = %q, want no arm run", out)
			}
			if st == 0 {
				t.Errorf("status = 0, want a failure — a case nobody could read reported success")
			}
		})
	}
}

// The status a failed subject leaves is the failure's, not the zero a `case`
// that matches nothing reports.
//
// Its own test because the two are one line apart and the wrong order is
// invisible from the output: no arm runs either way, and only `$?` says
// whether the shell noticed. A subject whose failure was **fatal on its own**
// reports through r.ctl rather than through the flags failedHeading reads, so
// `set -u; case ${NOPEV} in *) …` fell past the heading check and had its
// status zeroed: 0 here against 1 in bash, ksh93 and zsh and 2 in dash. The
// same `${NOPEV}` in a simple command already reported it correctly, so this
// was the `case` losing a status rather than the shell mis-numbering one.
//
// Asserted against the status the *same failure in a simple command* leaves
// rather than against a number, because which number it is belongs to the
// dialect.
// A subject that expanded cleanly still reports 0 for matching nothing, and
// the arms still run. The guard above is a refusal, and a refusal that fires
// on the ordinary path would be the more expensive mistake.
func TestACaseWhoseSubjectExpandsStillChoosesItsArm(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the matching arm runs", `p=abc; case $p in a*) printf hit;; *) printf miss;; esac`, "hit"},
		{"a later arm runs", `p=zzz; case $p in a*) printf hit;; *) printf miss;; esac`, "miss"},
		{"an empty subject is a subject", `p=; case "$p" in "") printf empty;; *) printf other;; esac`, "empty"},
		{"nothing matches, and that is a zero", `p=zzz; case $p in a*) printf hit;; esac; printf done`, "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// `;;&` goes back to testing the later patterns, and one of those that cannot
// be read stops the statement rather than being read as "did not match".
//
// The second site is easy to miss: the re-matching loop calls the matcher
// again, and its result was consumed as a bare bool. Without this a
// `;;&` chain walked straight past a pattern the shell had just diagnosed.
func TestAFailedPatternStopsTheRematchingLoop(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// The arithmetic shape is the one that has to be here. A `set -u`
		// failure raises a fatal error of its own, so control flow stops the
		// later bodies whatever this loop does — the mutant that ignores the
		// judgement survives that shape and dies on this one, where the
		// expansion reports only that it failed and the statement is what
		// has to make something of it.
		{"a pattern that cannot be evaluated", `case abc in a*) printf one;;& $((1/0))) printf two;;& *) printf three;; esac`},
		{"an unset name under set -u", `set -u; case abc in a*) printf one;;& ${NOPEV}) printf two;;& *) printf three;; esac`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runExtended(t, tc.src)
			// Stdout and stderr are the same buffer here, so the diagnostic
			// is in out too — what matters is which bodies ran.
			if !strings.HasPrefix(out, "one") {
				t.Errorf("out = %q, want the first arm to have run", out)
			}
			if strings.Contains(out, "two") || strings.Contains(out, "three") {
				t.Errorf("out = %q, want the unreadable pattern to have ended the statement", out)
			}
			if st == 0 {
				t.Errorf("status = 0, want a failure")
			}
		})
	}
}

// A pattern in the `;;&` re-matching loop that asks an axis no dialect
// answered stops the statement too, and it is the shape that makes the
// judgement there load-bearing.
//
// Its own test because of what mutation says about the other shapes: a
// pattern that *failed* raises a fatal error on its way out, so control flow
// stops the later bodies whether or not the loop reads the answer, and the
// mutant that discards it survives them. An unanswered axis sets no control
// flow — it is a refusal, not an error — so this is the one shape where
// reading the answer is the only thing between a refusal and a body chosen
// by a guess.
//
// `[` as a pattern is the axis: it is the name of the test builtin and the
// core has no answer for an unterminated bracket.
func TestAnUnansweredPatternStopsTheRematchingLoop(t *testing.T) {
	out, st := runExtendedSem(t, `case x in x) printf one;;& [) printf two;;& *) printf three;; esac`, CoreSemantics())
	if !strings.HasPrefix(out, "one") {
		t.Errorf("out = %q, want the first arm to have run", out)
	}
	if strings.Contains(out, "two") || strings.Contains(out, "three") {
		t.Errorf("out = %q, want the refusal to have ended the statement", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — an unanswered axis is a refusal", st)
	}
}

// runExtended runs with the `;;&` terminator available, which the core
// grammar does not have.
func runExtended(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, caseTerminators, nil)
}

// runExtendedSem is runExtended with a semantics vector of its own.
func runExtendedSem(t *testing.T, src string, sem Semantics) (string, int) {
	t.Helper()
	return runGrammar(t, src, caseTerminators, withSem(sem))
}

func caseTerminators(d *syntax.Dialect) { d.CaseFallthrough, d.CaseContinue = true, true }

// A pattern the dialect rejects outright is reported once, not once per item.
//
// The other half of what a pattern's judgement is for, and a different half
// from a *failed* one: this failure reports through control flow rather than
// through the flags failedHeading reads, so without a check of its own the
// loop would test the second `[` as well and diagnose the same refusal twice.
//
// `[` is the shape: it is the name of the test builtin, and one answer to the
// unterminated-bracket axis calls it a bad pattern and abandons the script.
func TestARefusedPatternIsReportedOnce(t *testing.T) {
	out, _ := run(t, `case "[" in [) echo one;; [) echo two;; *) echo three;; esac`,
		withSem(bracketSem(BracketBadPattern)))
	if n := strings.Count(out, "bad pattern"); n != 1 {
		t.Errorf("out = %q, want the refusal once and got it %d times", out, n)
	}
	for _, arm := range []string{"one", "two", "three"} {
		if strings.Contains(out, arm) {
			t.Errorf("out = %q, want no arm run", out)
		}
	}
}
