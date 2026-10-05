// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A bare assignment costs the same with forty hidden names in the shell as
// with none. One dialect registers about that many of its own parameters with
// the hide attribute before a script runs, and every assignment used to take
// the kind of all of them before and after, to see whether the line had
// retyped one: about four microseconds an assignment, and roughly 70ms of an
// interactive start on the maintainer's real configuration (2026-10-05). It
// looks at the names it writes now — see Runner.watchHiddenKinds.
//
// Allocations rather than time, so the row is deterministic: the old snapshot
// built a map per assignment, and a watch that has nothing to record builds
// none. The control is the hidden name the line does write, which has to come
// out of the line without the attribute when its kind changes.
func TestABareAssignmentCostsNothingPerHiddenName(t *testing.T) {
	run := func(r *Runner, f *syntax.File) {
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	f, err := syntax.Parse("x=1", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	allocs := func(hidden int) float64 {
		var out, errs strings.Builder
		r := seamRunner(t, &out, &errs)
		for i := 0; i < hidden; i++ {
			r.MarkHideInScope(fmt.Sprintf("hidden%d", i))
		}
		run(r, f)
		return testing.AllocsPerRun(200, func() { run(r, f) })
	}
	if none, forty := allocs(0), allocs(40); forty > none {
		t.Errorf("an assignment allocates %v times with forty hidden names and %v with none", forty, none)
	}
}
