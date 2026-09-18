// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// TestAWaitSaysASignalEndedTheJobItReaped is #3538.
//
// The *foreground* shape of this was already right — a command a signal ended
// is reported here, through Semantics.ReportsACommandKilledBySignal and this
// machine's libc suffix — and a background job reaped by `wait` reached no
// sentence at all.
//
// Measured 2026-09-18, a script file under `env -i PATH=/usr/bin:/bin` with
// stdin on /dev/null, against Apple's dash-16 on macOS arm64:
//
//	sh -c 'kill -TERM $$' &; wait $!; echo "wait pid: $?"
//	    Terminated: 15 on stderr, then wait pid: 143
//	dash -c "sh -c 'kill -TERM \$\$'; echo st=\$?"
//	    Terminated: 15, then st=143
//
// It is the general sentence rather than a `wait`-flavored one, which is the
// difference from #3392: the other column that reports here names the builtin
// and the process id, and this one names neither. So the axis says whether
// the reap speaks and Diagnostics.WaitSignalNotice says with which words —
// empty here, which is how the reap reaches Runner.killedNotice.
func TestAWaitSaysASignalEndedTheJobItReaped(t *testing.T) {
	if got := dash.Semantics().WaitReportsTheSignalThatEndedTheJob; got != interp.Yes {
		t.Errorf("WaitReportsTheSignalThatEndedTheJob is %v, want Yes", got)
	}
	if own := dash.Diagnostics().WaitSignalNotice; own != "" {
		t.Errorf("WaitSignalNotice is %q, want none: this column writes the general sentence", own)
	}

	out, _ := answersRun(t, `sh -c 'kill -TERM $$' &
wait $!
echo "wait pid: $?"
`)
	// The words after `Terminated` are the host's, so the claim is the
	// sentence and the status rather than a machine's spelling.
	if !strings.Contains(out, "Terminated") {
		t.Errorf("out = %q, want the reap to say a signal ended the job", out)
	}
	if !strings.Contains(out, "wait pid: 143") {
		t.Errorf("out = %q, want the status unchanged at 143", out)
	}

	// A bare `wait` for the same job says nothing, in every column — which is
	// what keeps this a statement about a `wait` that named something.
	bare, _ := answersRun(t, `sh -c 'kill -TERM $$' &
wait
echo "bare wait: $?"
`)
	if strings.Contains(bare, "Terminated") {
		t.Errorf("out = %q, want a bare wait to say nothing", bare)
	}
}
