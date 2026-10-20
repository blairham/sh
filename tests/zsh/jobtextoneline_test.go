// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestAJobsCommandIsOneLineOfTheParse pins the command a `jobs` row writes:
// the statement reprinted in the body arrangement and run onto one line.
// Measured 2026-10-01 and 2026-10-02 on zsh 5.9.2 under `-f` (#5342).
func TestAJobsCommandIsOneLineOfTheParse(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"x=1   /bin/sleep  1", "x=1 /bin/sleep 1"},
		{"(/bin/sleep 1;:)", "( /bin/sleep 1; :; )"},
		{"if :; then /bin/sleep 1; else :; fi", "if :; then; /bin/sleep 1; else; :; fi"},
		{"for i in a; do /bin/sleep 1; done", "for i in a; do; /bin/sleep 1; done"},
		{"case x in x) /bin/sleep 1;; y|z) :; :;; esac", "case x in (x) /bin/sleep 1 ;; (y | z) :; : ;; esac"},
		{"/bin/sleep 1 >/dev/null 2>&1", "/bin/sleep 1 > /dev/null 2>&1"},
		{"{ /bin/sleep 1; :; }", "{ /bin/sleep 1; :; }"},
		{"{ /bin/sleep 1 } always { : }", "{; /bin/sleep 1; } always {; :; }"},
		{"! /bin/sleep 1", "/bin/sleep 1"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.cmd+" &\njobs; kill %1")
		if !strings.HasSuffix(strings.TrimRight(got, "\n"), "running    "+tc.want) {
			t.Errorf("%s\n got %q\nwant the row ending %q", tc.cmd, got, tc.want)
		}
	}
}
