// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A reap leaves the finished jobs unmarked** —
// interp.Semantics.AReapLeavesFinishedJobsUnmarked. Measured 2026-10-03 on
// bash 5.3.20: `false & sleep 0.05 & wait %2; jobs` lists `[1]   Exit 1` with
// no `+`, where the same two jobs listed without a `wait` reaping one are
// `[1]-` and `[2]+`.
func TestAReapLeavesTheFinishedJobsUnmarked(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{
		Env: []string{"PATH=/usr/bin:/bin"}, Dir: t.TempDir(),
	}, "false & sleep 0.05 & wait %2; jobs\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "[1]   Exit 1") {
		t.Errorf("got %q, want job 1 listed with no marker", out)
	}
}
