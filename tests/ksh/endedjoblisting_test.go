// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// TestAnEndedJobIsListedUnderTheMonitorAndNotWithoutIt is #3537.
//
// This shell reaps a `&` job only while it is watching its children, so a
// `jobs` listing calls an ended job `Running` with no monitor and reports it
// properly with one. Ours said `Running` either way, because the difference
// was written down as a *word* — the preset spelled its `Done` as ` Running`
// — which answered the listing without the monitor and answered it wrongly
// with one.
//
// Measured 2026-09-18, a script file under `env -i PATH=/usr/bin:/bin` with
// stdin from /dev/null, ksh93u+ 2012-08-01:
//
//	set -m; ( exit 0 ) &; ( exit 7 ) &; ( sleep .5 ) &; sleep .2; jobs
//	    [3] +  Running / [2] -  Done(7) / [1]    Done
//	the same script without `set -m`
//	    [3] +  Running / [2] -  Running / [1]    Running
//	and still `Running` after a `wait` that reaped them
//	set -m; sh -c 'kill -TERM $$' &; sleep .4; jobs
//	    [1] + Terminated
//
// So the words are `Done` and `Done(N)`, and the blindness is
// Semantics.EndedJobIsListedAsRunningWithoutTheMonitor. The signal row above
// is a third word this listing has and no field holds; it is left where it
// was rather than guessed at, since `Exit N` is what every other column
// writes there too.
func TestAnEndedJobIsListedUnderTheMonitorAndNotWithoutIt(t *testing.T) {
	if got := ksh.Semantics().EndedJobIsListedAsRunningWithoutTheMonitor; got != interp.Yes {
		t.Errorf("EndedJobIsListedAsRunningWithoutTheMonitor is %v, want Yes", got)
	}

	const jobs = `( sleep 0.5 ) &
( exit 7 ) &
sleep 0.2
jobs
`
	monitored, _ := answersRun(t, "set -m\n"+jobs)
	if !strings.Contains(monitored, "Done(7)") {
		t.Errorf("under the monitor the listing is %q, want a Done(7) row", monitored)
	}
	if !strings.Contains(monitored, "Running") {
		t.Errorf("under the monitor the listing is %q, want the sleeping job still running", monitored)
	}

	blind, _ := answersRun(t, jobs)
	if strings.Contains(blind, "Done") {
		t.Errorf("with no monitor the listing is %q, want nothing noticed", blind)
	}
	if n := strings.Count(blind, "Running"); n != 2 {
		t.Errorf("with no monitor the listing is %q, want both rows running", blind)
	}
}
