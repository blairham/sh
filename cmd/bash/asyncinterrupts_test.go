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
func TestABackgroundProgramIgnoresTheKeyboardSignals(t *testing.T) {
	for sig, want := range map[string]string{"INT": "alive\n", "QUIT": "alive\n", "TERM": "dead\n"} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		src := "/bin/sleep 2 & /bin/sleep 0.2; kill -" + sig + " $!; /bin/sleep 0.2; " +
			"if kill -0 $! 2>/dev/null; then echo alive; kill $!; else echo dead; fi"
		driver.MainArgs(sh, []string{"sh", "-c", src})
		if out.String() != want {
			t.Errorf("%s: got %q (stderr %q), want %q", sig, out.String(), errs.String(), want)
		}
	}
}
