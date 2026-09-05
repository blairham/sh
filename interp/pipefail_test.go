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

func pipefailRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.PipefailOption = a
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// A pipeline reports its last element. With pipefail it reports its last
// *failing* element — which is a different thing from its first failure, and
// the difference only shows when two elements fail with different statuses.
func TestPipefailReportsTheLastFailure(t *testing.T) {
	for _, c := range []struct {
		src    string
		with   int
		wthout int
	}{
		{`(exit 3) | (exit 4) | true`, 4, 0},
		{`(exit 4) | (exit 3) | true`, 3, 0},
		{`(exit 3) | true | true`, 3, 0},
		{`true | true | (exit 5)`, 5, 5},
		{`true | true | true`, 0, 0},
		// `exit` in an element is that element's status like any other, and
		// pipefail reports it — measured at 6 in bash, ksh93 and zsh.
		{`exit 6 | true`, 6, 0},
	} {
		if _, st := pipefailRun(t, Yes, "set -o pipefail\n"+c.src); st != c.with {
			t.Errorf("with pipefail, %s = %d, want %d", c.src, st, c.with)
		}
		if _, st := pipefailRun(t, Yes, c.src); st != c.wthout {
			t.Errorf("option available but unset, %s = %d, want %d", c.src, st, c.wthout)
		}
	}
}

// `set +o` puts it back.
func TestPipefailCanBeTurnedOff(t *testing.T) {
	if _, st := pipefailRun(t, Yes, "set -o pipefail\nset +o pipefail\n(exit 3) | true"); st != 0 {
		t.Errorf("after set +o, status %d, want 0", st)
	}
}

// Where the dialect has no such option the name is not an option, and is
// refused exactly as any other name this shell does not have — the pipeline
// then goes on reporting its last element, which is the answer the option
// exists to avoid.
func TestWithoutTheOptionTheNameIsRefused(t *testing.T) {
	out, st := pipefailRun(t, No, "set -o pipefail\necho st=$?\n(exit 3) | true\necho p=$?")
	if !strings.Contains(out, "invalid option name") {
		t.Errorf("said %q, want it refused as a name", out)
	}
	if !strings.Contains(out, "st=2") || !strings.Contains(out, "p=0") {
		t.Errorf("said %q, want the refusal to leave the pipeline alone", out)
	}
	_ = st
}

// Unanswered is refused once, not twice: the axis speaks, and the bad-name
// complaint that would follow is a consequence of the refusal rather than
// anything the script wrote.
func TestAnUnansweredPipefailIsRefusedOnce(t *testing.T) {
	out, _ := pipefailRun(t, Unspecified, "set -o pipefail")
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("said %q (%d lines), want one", out, n)
	}
	if strings.Contains(out, "invalid option name") {
		t.Errorf("said %q, want no second complaint about the same word", out)
	}
}
