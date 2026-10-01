// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The command the shell is running holds a job number while it runs**, and
// a marker can land on that number (#5301). Every row measured 2026-10-01 on
// zsh 5.9.2 under `-f -c`, byte for byte; the rows marked control agree with
// a shell that holds nothing. See interp.Semantics.ACommandHoldsAJobSlot.
func TestACommandHoldsAJobSlot(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a brace group holds 1", `{ /bin/sleep 0.2 & jobs; }; wait`,
			"[2]  + running    /bin/sleep 0.2\n",
		},
		{
			"a function call holds 1", `f() { /bin/sleep 0.2 & jobs; }; f; wait`,
			"[2]  + running    /bin/sleep 0.2\n",
		},
		{
			"eval holds 1", `eval '/bin/sleep 0.2 & jobs'; wait`,
			"[2]  + running    /bin/sleep 0.2\n",
		},
		{
			"control: a job at the top", `/bin/sleep 0.2 & jobs; wait`,
			"[1]  + running    /bin/sleep 0.2\n",
		},
		{
			"control: the slot goes with the command", `{ :; }; /bin/sleep 0.2 & jobs; wait`,
			"[1]  + running    /bin/sleep 0.2\n",
		},
		{
			"the + falls to the slot", `f() { (exit 3) & /bin/sleep 0.2; wait %%; wait %-; }; f`,
			"f:wait: %%: no such job\nf:wait: no previous job\n",
		},
		{
			"control: no slot, no current job", `(exit 3) & /bin/sleep 0.2; wait %%`,
			"zsh:wait:1: no current job\n",
		},
		{
			"kill reaches the slot and sends nothing",
			`f() { (exit 3) & /bin/sleep 0.2; kill -0 %%; echo k=$?; kill -0 %1; echo k=$?; jobs %1; }; f`,
			"k=0\nk=0\nf:jobs: %1: no such job\n",
		},
		{
			"the - stays on a number the command let go of",
			`/bin/sleep 0.3 & { /bin/sleep 0.3 & }; jobs; jobs %-; wait`,
			"[1]    running    /bin/sleep 0.3\n[3]  + running    /bin/sleep 0.3\nzsh:jobs:1: %-: no such job\n",
		},
		{
			"a job that ended during the command left the - on it",
			`/bin/sleep 0.1 & { /bin/sleep 0.3; }; jobs %%; jobs %-`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: %-: no such job\n",
		},
		{
			"the + on the slot goes with it",
			`f() { /bin/sleep 0.1 & /bin/sleep 0.3; }; f; jobs %%; jobs %-`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: no previous job\n",
		},
		{
			"the - is the highest number, the slot counting",
			`{ (exit 3) & /bin/sleep 0.4 & /bin/sleep 0.2; jobs %%; jobs %-; }; wait`,
			"[3]  + running    /bin/sleep 0.4\nzsh:jobs:1: %-: no such job\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// **A finished job's number is free for the next one**, in the dialect where
// the job leaves the table, and an oldest-first listing is by number (#5301).
// Measured 2026-10-01 on zsh 5.9.2 under `-f -c`.
func TestAFinishedJobsNumberIsTheNextJobs(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"refilled", `/bin/sleep 0.1 & /bin/sleep 0.3; /bin/sleep 0.2 & jobs; wait`,
			"[1]  + running    /bin/sleep 0.2\n",
		},
		{
			"listed by number",
			`/bin/sleep 0.4 & /bin/sleep 0.4 & /bin/sleep 0.4 & kill %1; wait %1; /bin/sleep 0.2 & jobs; wait`,
			"[1]  + running    /bin/sleep 0.2\n[2]    running    /bin/sleep 0.4\n[3]  - running    /bin/sleep 0.4\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
