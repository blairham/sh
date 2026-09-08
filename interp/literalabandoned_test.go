// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An array literal's element list is a *heading*: it is expanded before the
// assignment decides what to store, so a failure in it costs the store rather
// than one element of it.
//
// We reported the failure and then made the assignment anyway, from whatever
// words had survived — the unexpanded pattern where a pattern had missed, and
// the words on either side of a division by zero. Which shells call which of
// these a failure is a dialect question and is measured by the corpus rows
// under `array/…-abandons-the-assignment`; what these tests pin is that the
// substrate abandons the store whenever the expansion said it failed (#1568).

// runLiteralIn runs src in a directory of the test's own, with a pattern that
// matches nothing being the error one dialect calls it. The directory matters:
// a pattern resolved against the developer's own directory could match.
func runLiteralIn(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobNoMatchIsError = Yes
		// Every case here is inside `eval`, because a failure in an element
		// list is fatal and a shell that has stopped cannot be asked what its
		// variable is holding. This is the axis that lets the `eval` catch it
		// and hand the aftermath back; it is asked for its own sake in
		// fileabandon_test.go and is only scaffolding here.
		sem.FatalErrorEndsBorrowedTextOnly = Yes
		sem.ParamErrorIsAnExitRequest = No
		r.Semantics = &sem
	})
}

func emptyDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// TestAnUnmatchedPatternLeavesTheNameAlone is the issue as it was filed.
func TestAnUnmatchedPatternLeavesTheNameAlone(t *testing.T) {
	out, _ := runLiteralIn(t, emptyDir(t),
		`reply=(keep); eval "reply=(zzznomatch*)"; echo "[${reply[*]}] n=${#reply[@]}"`)
	if want := "[keep] n=1"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q — the assignment abandoned", out, want)
	}
	if strings.Contains(out, "zzznomatch*]") {
		t.Errorf("got %q, want the pattern not to have been stored", out)
	}
	if !strings.Contains(out, "no matches found") {
		t.Errorf("got %q, want the miss still reported", out)
	}
}

// TestAnUnmatchedPatternCreatesNoName is the same rule from the other
// starting state, and the half a fix reaches by keeping the old value: a name
// that had none must stay unset rather than be created empty.
func TestAnUnmatchedPatternCreatesNoName(t *testing.T) {
	out, _ := runLiteralIn(t, emptyDir(t),
		`eval "reply=(zzznomatch*)"; echo "n=${#reply[@]} set=${reply+yes}"`)
	if want := "n=0 set="; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q — no name created", out, want)
	}
}

// TestOneUnmatchedElementAbandonsThemAll is the distinguishing question: the
// whole assignment, or only the failing element?
//
// A printed value alone cannot answer it — an implementation that dropped
// just the pattern would leave two good elements standing and look entirely
// reasonable. The elements on either side of the miss are what make the two
// readings differ.
func TestOneUnmatchedElementAbandonsThemAll(t *testing.T) {
	dir := emptyDir(t)
	if err := os.WriteFile(filepath.Join(dir, "m1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runLiteralIn(t, dir,
		`reply=(keep); eval "reply=(m1 zzznomatch* m1)"; echo "[${reply[*]}] n=${#reply[@]}"`)
	if want := "[keep] n=1"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q — the whole list abandoned, not the one element", out, want)
	}
}

// TestAFailedExpansionAbandonsTheLiteral is the same store reached without a
// pattern, which is what says the rule belongs to the assignment rather than
// to the matcher. The status is half the assertion: we stored the surviving
// words *and reported success*, for a value the shell had just said it could
// not compute.
func TestAFailedExpansionAbandonsTheLiteral(t *testing.T) {
	out, _ := runLiteralIn(t, emptyDir(t),
		`reply=(keep); eval 'reply=(a $((1/0)) b)'; echo "st=$? [${reply[*]}] n=${#reply[@]}"`)
	if want := "[keep] n=1"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q — the assignment abandoned", out, want)
	}
	if strings.Contains(out, "st=0 ") {
		t.Errorf("got %q, want the failure's own status rather than success", out)
	}
}

// TestAnUnsetNameUnderNounsetAbandonsTheLiteral is the third route into the
// same store. One of the three not abandoning would say the check sits at the
// wrong level rather than being missing once.
func TestAnUnsetNameUnderNounsetAbandonsTheLiteral(t *testing.T) {
	out, _ := runLiteralIn(t, emptyDir(t),
		`set -u; reply=(keep); eval 'reply=(a $NOPEVAR b)'; echo "[${reply[*]}] n=${#reply[@]}"`)
	if want := "[keep] n=1"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q — the assignment abandoned", out, want)
	}
}

// TestAMatchingPatternStillAssigns is the row that keeps the guard from being
// satisfied by refusing everything: an element list that expanded is stored,
// and a pattern that matches is not a failure.
func TestAMatchingPatternStillAssigns(t *testing.T) {
	dir := emptyDir(t)
	for _, n := range []string{"m1", "m2"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, st := runLiteralIn(t, dir, `reply=(keep); reply=(m*); echo "[${reply[*]}] n=${#reply[@]}"`)
	if want := "[m1 m2] n=2"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
