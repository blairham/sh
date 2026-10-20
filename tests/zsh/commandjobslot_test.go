// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The command the shell is running holds a job number while it runs**, and
// a marker can land on that number (#5301). Every row measured 2026-10-01 on
// zsh 5.9.2 under `-f -c`, byte for byte; the rows marked control agree with
// a shell that holds nothing. See interp.Semantics.ACommandHoldsAJobSlot.
//
// No row races a sleep against the shell (#5556). The rows used to race sleep
// lengths of 0.1s to 0.5s against the shell, and under load one failed. Each
// state is now something the script did, not something the clock allowed:
//   - A job that has to still be running sleeps thirty seconds and is killed
//     before the row ends.
//   - A job that has to end during a command, without being reaped by `wait`,
//     is a program, and the command waits for it with poll. poll is a
//     `/bin/sh` that returns once the job's pid is gone. It is bounded at
//     3000 checks 10ms apart, so a failure cannot leave it spinning. It is a
//     simple command, so like the `/bin/sleep` it replaces it holds no
//     slot.
//   - `(exit 3)` became `/bin/sh -c 'exit 3'`, so the job has a pid to watch.
//
// Every row was measured again in this shape, 2026-10-03, on zsh 5.9.2, and
// prints what it printed before, with 30 in place of each sleep length.
func TestACommandHoldsAJobSlot(t *testing.T) {
	// poll returns once the job whose pid is in $p is gone. It is a simple
	// command, so it holds no slot of its own.
	const poll = `/bin/sh -c 'i=0; while kill -0 $1 2>/dev/null && [ $i -lt 3000 ]; do /bin/sleep 0.01; i=$((i+1)); done' poll $p`
	const exit3 = `/bin/sh -c 'exit 3'`
	for _, c := range []struct{ name, src, want string }{
		{
			"a brace group holds 1",
			`{ /bin/sleep 30 & jobs; kill $!; }; wait`,
			"[2]  + running    /bin/sleep 30\n",
		},
		{
			"a function call holds 1",
			`f() { /bin/sleep 30 & jobs; kill $!; }; f; wait`,
			"[2]  + running    /bin/sleep 30\n",
		},
		{
			"eval holds 1",
			`eval '/bin/sleep 30 & jobs; kill $!'; wait`,
			"[2]  + running    /bin/sleep 30\n",
		},
		{
			"control: a job at the top",
			`/bin/sleep 30 & jobs; kill $!; wait`,
			"[1]  + running    /bin/sleep 30\n",
		},
		{
			"control: the slot goes with the command",
			`{ :; }; /bin/sleep 30 & jobs; kill $!; wait`,
			"[1]  + running    /bin/sleep 30\n",
		},
		{
			"the + falls to the slot",
			`f() { ` + exit3 + ` & p=$!; ` + poll + `; wait %%; wait %-; }; f`,
			"f:wait: %%: no such job\nf:wait: no previous job\n",
		},
		{
			"control: no slot, no current job",
			exit3 + ` & p=$!; ` + poll + `; wait %%`,
			"zsh:wait:1: no current job\n",
		},
		{
			"kill reaches the slot and sends nothing",
			`f() { ` + exit3 + ` & p=$!; ` + poll + `; kill -0 %%; echo k=$?; kill -0 %1; echo k=$?; jobs %1; }; f`,
			"k=0\nk=0\nf:jobs: %1: no such job\n",
		},
		{
			"the - stays on a number the command let go of",
			`/bin/sleep 30 & { /bin/sleep 30 & }; jobs; jobs %-; kill %1 %3 %4 2>/dev/null; wait`,
			"[1]    running    /bin/sleep 30\n[3]  + running    /bin/sleep 30\nzsh:jobs:1: %-: no such job\n",
		},
		{
			"a job that ended during the command left the - on it",
			`/bin/sleep 0 & p=$!; { ` + poll + `; }; jobs %%; jobs %-`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: %-: no such job\n",
		},
		{
			"the + on the slot goes with it",
			`f() { /bin/sleep 0 & p=$!; ` + poll + `; }; f; jobs %%; jobs %-`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: no previous job\n",
		},
		{
			"a function with a simple body holds one too",
			`f() ` + poll + `; /bin/sleep 0 & p=$!; f; jobs %%; jobs %-`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: %-: no such job\n",
		},
		{
			"a leaving - is chosen again",
			`{ /bin/sleep 30 & ` + exit3 + ` & p=$!; /bin/sleep 30 & ` + poll + `; jobs %-; kill %2 %4 %5 2>/dev/null; }; wait`,
			"[2]  - running    /bin/sleep 30\n",
		},
		{
			"control: a job that ended before the command leaves no slot behind",
			`/bin/sleep 0 & p=$!; ` + poll + `; { jobs %%; jobs %-; }`,
			"zsh:jobs:1: no current job\nzsh:jobs:1: no previous job\n",
		},
		{
			"a bare disown with the + on the slot",
			`f() { ` + exit3 + ` & p=$!; ` + poll + `; disown; echo $?; }; f`,
			"f:disown: no current job\n1\n",
		},
		{
			"a bare wait hands the + to the slot",
			`{ /bin/sleep 30 & kill %%; wait; jobs %%; jobs %-; }`,
			"zsh:jobs:1: %%: no such job\nzsh:jobs:1: no previous job\n",
		},
		{
			"a command inside a subshell holds nothing more",
			`( /bin/sleep 30 & a=$!; { /bin/sleep 30 & print ${(k)jobstates}; kill $a $!; } ); wait`,
			"2 3\n",
		},
		{
			"the - is the highest number, the slot counting",
			`{ ` + exit3 + ` & p=$!; /bin/sleep 30 & ` + poll + `; jobs %%; jobs %-; kill %3 %4 2>/dev/null; }; wait`,
			"[3]  + running    /bin/sleep 30\nzsh:jobs:1: %-: no such job\n",
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
//
// No row races a sleep against the shell (#5554). It used `/bin/sleep 0.4`
// jobs that had to be still running when `jobs` listed them, and at load 16
// they were not. A job that must still be running now sleeps for thirty
// seconds and is killed at the end, and a job that must have finished is
// killed and waited for, so each state is something the script did rather
// than something the clock allowed — and every job is killed by number at
// the end, one more number than there should be, so a shell that numbered
// them wrongly fails at once rather than waiting thirty seconds. Both rows
// were measured again in this shape, 2026-10-03, on zsh 5.9.2, with the same
// listing.
func TestAFinishedJobsNumberIsTheNextJobs(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"refilled", `/bin/sleep 30 & kill %1; wait %1; /bin/sleep 30 & jobs; kill %1 %2 2>/dev/null; wait`,
			"[1]  + running    /bin/sleep 30\n",
		},
		{
			"listed by number",
			`/bin/sleep 30 & /bin/sleep 30 & /bin/sleep 30 & kill %1; wait %1; /bin/sleep 30 & jobs; kill %1 %2 %3 %4 2>/dev/null; wait`,
			"[1]  + running    /bin/sleep 30\n[2]    running    /bin/sleep 30\n[3]  - running    /bin/sleep 30\n",
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
