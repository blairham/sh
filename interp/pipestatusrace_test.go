// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Two subshells in one pipeline each record a pipeline status of their own,
// concurrently, into a runner they were cloned from. Only under `-race`, and
// only when something ran first: the record has to already hold a slice for
// the two clones to have anything to share.
//
// The dialect is needed because the record is only kept where a dialect has
// named it, so the substrate alone cannot reach this at all.
func TestTwoSubshellsInAPipelineDoNotShareTheRecord(t *testing.T) {
	src := "true\n(exit 3) | (exit 4) | true\necho \"st=$? [${PIPESTATUS[@]}]\"\n"
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		var buf bytes.Buffer
		sem := bash.Semantics()
		dg := bash.Diagnostics()
		r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg}
		bash.Apply(r)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		// The record is also correct, not merely unraced: whichever clone
		// wrote last, the pipeline's own three statuses are what survives.
		if got := strings.TrimSpace(buf.String()); got != "st=0 [3 4 0]" {
			t.Fatalf("said %q, want st=0 [3 4 0]", got)
		}
	}
}
