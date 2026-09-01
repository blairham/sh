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

func deathRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = a
	dg := Diagnostics{}
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// A killed process has no exit status of its own. Go reports -1, which is not
// a status a shell can report at all — it reached `$?` as -1 and the process's
// own exit code as 255.
//
// PIPE and TERM rather than KILL: both are ordinary here, and using two says
// the base is a base rather than something done for one signal.
func TestAKilledCommandEncodesTheSignal(t *testing.T) {
	for _, c := range []struct {
		sig            string
		oneTwentyEight int
		twoFiftySix    int
	}{
		{"PIPE", 141, 269},
		{"TERM", 143, 271},
		{"HUP", 129, 257},
	} {
		src := "sh -c 'kill -" + c.sig + " $$'"
		if _, st := deathRun(t, No, src); st != c.oneTwentyEight {
			t.Errorf("%s with base 128: status %d, want %d", c.sig, st, c.oneTwentyEight)
		}
		if _, st := deathRun(t, Yes, src); st != c.twoFiftySix {
			t.Errorf("%s with base 256: status %d, want %d", c.sig, st, c.twoFiftySix)
		}
	}
}

// A command that exits by itself is untouched by any of this, whichever way
// the axis is answered — which is what makes the encoding about being killed
// rather than about failing.
func TestAnOrdinaryFailureIsNotEncoded(t *testing.T) {
	for _, a := range []Answer{No, Yes, Unspecified} {
		if out, st := deathRun(t, a, `sh -c 'exit 3'`); st != 3 {
			t.Errorf("answer %v: status %d (%q), want 3", a, st, out)
		}
	}
	// Nor is success, which would otherwise be the easiest thing to break by
	// encoding signal 0.
	for _, a := range []Answer{No, Yes} {
		if _, st := deathRun(t, a, `sh -c 'exit 0'`); st != 0 {
			t.Errorf("answer %v: status %d, want 0", a, st)
		}
	}
}

// Unanswered is refused, like any other axis: the two bases give different
// answers to `$?` and the core does not pick one.
func TestAnUnansweredBaseIsRefused(t *testing.T) {
	out, _ := deathRun(t, Unspecified, `sh -c 'kill -PIPE $$'`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want a refusal", out)
	}
}
