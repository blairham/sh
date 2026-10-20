// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// **A job's command is listed as bash prints its parse, not as it was typed**
// (#5311). Measured 2026-10-01 on bash 5.3.20, `CMD &` and then `jobs`, byte
// for byte; the pipeline is the control, which prints as it was typed. See
// interp.Semantics.JobCommandIsReprinted.
func TestAJobsCommandIsReprinted(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`(/bin/sleep 1;:) &`, "( /bin/sleep 1; : ) &"},
		{`x=1   /bin/sleep  1 &`, "x=1 /bin/sleep 1 &"},
		{`if :; then /bin/sleep 1; fi &`, "if :; then\n    /bin/sleep 1;\nfi &"},
		{`{ /bin/sleep 1; :; } &`, "{ /bin/sleep 1; :; } &"},
		{`while :; do /bin/sleep 1; break; done &`, "while :; do\n    /bin/sleep 1; break;\ndone &"},
		{`/bin/sleep 1 >/dev/null 2>&1 &`, "/bin/sleep 1 > /dev/null 2>&1 &"},
		{`! /bin/sleep 1 &`, "/bin/sleep 1 &"},
		{`/bin/sleep 1 | /bin/cat &`, "/bin/sleep 1 | /bin/cat &"},
	} {
		t.Run(c.src, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src+"\njobs\nkill %% 2>/dev/null")
			if want := "[1]+  Running                    " + c.want + "\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}
