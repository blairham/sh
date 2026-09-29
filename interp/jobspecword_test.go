// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The unanswered path of [Semantics.JobSpecCommandWord], which no dialect
// reaches — every preset answers the axis — and which is therefore only
// visible from here.
//
// Three things have to hold together and a mutation that broke any one of
// them survived a suite graded on the dialects alone:
//
//   - a `%` command word with no answer **refuses by name** and does not run;
//   - a command word that is not one runs as it always did, because the axis
//     is asked only where the question arises;
//   - the refusal is **this question's own** and not the command's shared
//     one. That flag carries whatever the command has already refused over,
//     and a site that read it here abandoned commands for somebody else's
//     refusal — which is exactly what the first draft did.
func TestAPercentWordWithNoAnswerRefusesByNameAndNothingElseDoes(t *testing.T) {
	run := func(src string) string {
		t.Helper()
		var out bytes.Buffer
		dir := t.TempDir()
		sem := PosixSemantics()
		r := newTestRunner(t, &Runner{
			Stdout: &out, Stderr: &out, Semantics: &sem,
			Dir: dir, Name: "sh", Vars: map[string]string{"PATH": dir},
		})
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out.String()
	}
	got := run("%prep\n")
	if !strings.Contains(got, "job specification") || !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("a %% command word said %q, want the axis refused by name", got)
	}
	// And the refusal costs the command: a word the shell decided it could
	// not read is not then looked up as a name.
	if strings.Contains(got, "not found") {
		t.Errorf("said %q, want the refusal instead of a lookup", got)
	}
	// The gate. A vector with no answer here still runs everything that is
	// not spelled with a leading `%` — without it, a command word being the
	// commonest thing a shell has, nothing would run at all.
	if got := run("echo RAN\n"); !strings.Contains(got, "RAN") {
		t.Errorf("an ordinary command said %q, want it to run", got)
	}
	if got := run("echo %prep\n"); !strings.Contains(got, "%prep") {
		t.Errorf("a %% argument said %q, want it printed", got)
	}
}
