// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// holeProbe makes a hole in the jobs table and then asks where the next job
// went.
//
// The hole is made by killing the middle job and reaping it **by name**,
// which is the one spelling every shell in the panel acts on: a job left to
// finish on its own is kept by some of them and dropped by others, and #969
// is the standing record of how little that question holds still. Nothing
// here depends on it — the case is about allocation, and the two readings of
// allocation differ whether or not the reaping is what freed the slot.
//
// Five-second jobs killed at the end, for the reason the corpus family gives:
// a job with seconds left is certainly still running, so no answer below is
// racing the scheduler, and the case still costs no wall time because the
// jobs are killed rather than waited for.
//
// `two` is the slot the hole was in and `four` is the slot past the highest.
// Both are asked, because either alone is satisfied by a shell that started
// no new job at all.
const holeProbe = `sleep 5 & sleep 5 & sleep 5 &
kill %2 >/dev/null 2>&1
wait %2 >/dev/null 2>&1
sleep 5 &
jobs %2 >/dev/null 2>&1; echo "two=$?"
jobs %4 >/dev/null 2>&1; echo "four=$?"
kill %1 >/dev/null 2>&1; kill %2 >/dev/null 2>&1
kill %3 >/dev/null 2>&1; kill %4 >/dev/null 2>&1
:
`

func runJobHole(t *testing.T, refills Answer) (out, errOut string) {
	t.Helper()
	f, err := syntax.Parse(holeProbe, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := testSemantics()
	sem.NextJobNumberRefillsAHole = refills
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh", Env: testPATH(),
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	return o.String(), e.String()
}

// The refilling rule, which is dash's, ksh93's, zsh's and ash's: the new job
// goes back into the slot the hole was in and there is no fourth slot.
//
// Measured 2026-09-12 on a script, three runs each and identical: `jobs %2`
// answers 0 in all four of them afterwards and `jobs %4` answers each shell's
// own no-such-job status.
func TestTheNextJobNumberMayRefillAHole(t *testing.T) {
	out, errOut := runJobHole(t, Yes)
	if !strings.Contains(out, "two=0") {
		t.Errorf("out = %q, want the hole refilled (two=0); stderr %q", out, errOut)
	}
	if strings.Contains(out, "four=0") {
		t.Errorf("out = %q, want no fourth slot; stderr %q", out, errOut)
	}
}

// And bash's rule: the hole stays a hole and the numbering carries on past
// the highest occupied slot.
//
// Measured the same day, bash 5.3.15 and bash 3.2.57 alike: `jobs %2` is
// still no-such-job and `jobs %4` is 0.
func TestTheNextJobNumberMayCarryOnPastTheHighest(t *testing.T) {
	out, errOut := runJobHole(t, No)
	if strings.Contains(out, "two=0") {
		t.Errorf("out = %q, want the hole left alone; stderr %q", out, errOut)
	}
	if !strings.Contains(out, "four=0") {
		t.Errorf("out = %q, want the new job past the highest (four=0); stderr %q", out, errOut)
	}
}

// The axis is asked **at the disagreement and nowhere else**, which is what
// keeps an unanswered preset able to run ordinary scripts.
//
// A table with no hole in it gets the same number from both rules, so a shell
// that has not chosen must not be stopped by a background job — and nearly
// every script that starts one never leaves a hole. Two jobs, no kill, and
// the third is `%3` whatever the field says.
func TestATableWithNoHoleDoesNotAskTheQuestion(t *testing.T) {
	src := `sleep 5 & sleep 5 &
sleep 5 &
jobs %3 >/dev/null 2>&1; echo "three=$?"
jobs %4 >/dev/null 2>&1; echo "four=$?"
kill %1 >/dev/null 2>&1; kill %2 >/dev/null 2>&1; kill %3 >/dev/null 2>&1
:
`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := testSemantics()
	sem.NextJobNumberRefillsAHole = Unspecified
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh", Env: testPATH(),
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if e.Len() != 0 {
		t.Errorf("wrote %q to the error stream, want nothing — no hole, so nothing to disagree about", e.String())
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if out := o.String(); !strings.Contains(out, "three=0") || strings.Contains(out, "four=0") {
		t.Errorf("out = %q, want the third job at %%3 and no %%4", out)
	}
}

// And where they do disagree, an unanswered field says so rather than picking
// one. The refusal is the whole reason this is `ask`ed and not read: the two
// rules give different job numbers, and a script that then says `kill %4` is
// signaling a different process depending on which was guessed.
func TestAnUnansweredHoleQuestionIsRefused(t *testing.T) {
	_, errOut := runJobHole(t, Unspecified)
	if !strings.Contains(errOut, "the shells disagree here") {
		t.Errorf("stderr = %q, want the unanswered-axis refusal", errOut)
	}
}
