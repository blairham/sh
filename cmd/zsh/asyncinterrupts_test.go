// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestABackgroundProgramIgnoresTheKeyboardSignals pins the standard's rule
// for an asynchronous list in a shell without job control: SIGINT and SIGQUIT
// are ignored by what it runs. Measured 2026-10-02 against the reference,
// `sleep 1 & sleep 0.2; kill -SIG $!; sleep 0.2; kill -0 $!` under `-c`
// leaves the job alive for both (#5414). The control is TERM, which the same
// job dies of.
//
// Read from the status `wait` reports rather than from `kill -0` after a
// pause, because both halves of that raced the clock under load (#5570):
// the job could end on its own before the probe, and a job killed but not
// yet reaped still answers `kill -0`. An ignored signal is discarded when it
// is sent, so the SIGKILL after it decides the status, 137. A job the
// signal ends reports 128 plus that signal. Measured 2026-10-03 with
// `/bin/sleep 30` on zsh 5.9.2 and bash 5.3.20: INT and QUIT give 137, TERM
// gives 143.
func TestABackgroundProgramIgnoresTheKeyboardSignals(t *testing.T) {
	for sig, want := range map[string]string{"INT": "137\n", "QUIT": "137\n", "TERM": "143\n"} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		finish := "/bin/sleep 0.2; kill -KILL $!; "
		if sig == "TERM" {
			finish = ""
		}
		src := "/bin/sleep 30 & /bin/sleep 0.2; kill -" + sig + " $!; " + finish + "wait $!; echo $?"
		driver.MainArgs(sh, []string{"sh", "-c", src})
		if out.String() != want {
			t.Errorf("%s: got %q (stderr %q), want %q", sig, out.String(), errs.String(), want)
		}
	}
}
