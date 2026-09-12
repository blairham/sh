// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `kill %1` reaches every process a backgrounded pipeline is made of.
//
// A job is not one process. `sleep 45 | cat &` is two, and `%1` names the job
// rather than whichever half happens to answer `$!` — measured against bash
// 5.3.15, `sleep 45 | cat & kill %1; wait` returns at once there, with both
// halves gone. Here only the element that settled the job's pid was ever
// recorded, so the signal reached the `cat`, the `sleep` ran on, and the
// `wait` that followed sat out its full forty-five seconds.
//
// That is #2295, and it is why bash's own `jobs` file stopped finishing inside
// the suite's per-file bound: the file is a long series of jobs told to stop,
// and each one this shell failed to reach was added to the run in full.
//
// The kill is the very next command after the `&` on purpose. Both halves of
// the defect are here: the shell has to *know* both processes by then, which
// means `&` cannot return until every element of the pipeline has started, and
// it has to signal both. With only the second half fixed this passed or failed
// by which goroutine won.
func TestKillingABackgroundedPipelineReachesEveryProcessInIt(t *testing.T) {
	for _, w := range []struct{ name, src string }{
		{"two elements", `%[1]s 45 | /bin/cat & kill %%1; wait; printf DONE`},
		{"three elements", `%[1]s 45 | /bin/cat | /bin/cat & kill %%1; wait; printf DONE`},
		{"the sleep last", `/bin/echo hi | %[1]s 45 & kill %%1; wait; printf DONE`},
	} {
		t.Run(w.name, func(t *testing.T) {
			src := fmt.Sprintf(w.src, lookCmd("sleep"))
			var got string
			deadline(t, "killing a backgrounded pipeline", func() {
				// The one axis these cases reach that the core leaves open:
				// whether a signal that ended a *middle* element of a
				// pipeline is remarked on. It decides nothing here — the
				// subject is which processes the signal reached — so the
				// case answers it rather than reading an unanswered axis's
				// complaint as output.
				got, _ = runLeavingJobsRunning(t, src, func(r *Runner) {
					sem := *r.Semantics
					sem.ReportsAnyKilledPipelineElement = No
					r.Semantics = &sem
				})
			})
			if got != "DONE" {
				t.Errorf("output = %q, want DONE — a process of the job outlived `kill %%1`", got)
			}
		})
	}
}
