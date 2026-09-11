// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// What the shell was born with is looked up by scanning `NAME=VALUE` entries
// without splitting them, which is #2036. These pin the three things that
// scan has to get right and that a split walk got for free.
//
// They name no shell: every rule here is unanimous across the panel.

// envRun runs src against an explicit Env, and nothing else.
func envRun(t *testing.T, env []string, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh", Env: env,
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// A name is the whole of what precedes the `=`. Testing a prefix instead
// would let `PATHX` answer for `PATH`, which is the way a scan that does not
// split can be wrong and a scan that splits cannot.
func TestALongerNameDoesNotAnswerForAShorterOne(t *testing.T) {
	env := []string{"PATHX=wrong", "PATH=right", "XPATH=alsowrong"}
	if out, st := envRun(t, env, `echo "$PATH"`); st != 0 || out != "right\n" {
		t.Errorf("$PATH = %q, status %d, want %q", out, st, "right\n")
	}
	// And the other direction: a name longer than every entry matches none of
	// them rather than running off the end of one.
	if out, st := envRun(t, env, `echo "[${PATHXY-unset}]"`); st != 0 || out != "[unset]\n" {
		t.Errorf("$PATHXY = %q, status %d, want %q", out, st, "[unset]\n")
	}
}

// An entry with no `=` is not a variable, and must not be read as a name
// holding nothing. It cannot match, whatever the name is.
func TestAnEntryWithNoEqualsIsNotAVariable(t *testing.T) {
	env := []string{"JUSTANAME", "REAL=here"}
	if out, st := envRun(t, env, `echo "[${JUSTANAME-unset}][$REAL]"`); st != 0 || out != "[unset][here]\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "[unset][here]\n")
	}
}

// The first entry wins where a name appears twice, which is the answer the
// split walk gave and is what a shell handed a duplicated name reads.
func TestTheFirstOfTwoEntriesForOneNameWins(t *testing.T) {
	if out, st := envRun(t, []string{"DUP=first", "DUP=second"}, `echo "$DUP"`); st != 0 || out != "first\n" {
		t.Errorf("$DUP = %q, status %d, want %q", out, st, "first\n")
	}
}

// `unset` takes the name away, and the question is asked about the name once
// rather than about every entry. A value it is holding must not come back.
func TestUnsetHidesAnInheritedNameAndItsExport(t *testing.T) {
	env := []string{"GONE=value"}
	if out, st := envRun(t, env, `unset GONE; echo "[${GONE-unset}]"`); st != 0 || out != "[unset]\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "[unset]\n")
	}
	// An empty value is a value: the name is there and holds nothing, which
	// is a different answer from having been taken away.
	if out, st := envRun(t, []string{"EMPTY="}, `echo "[${EMPTY-unset}]"`); st != 0 || out != "[]\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "[]\n")
	}
}
