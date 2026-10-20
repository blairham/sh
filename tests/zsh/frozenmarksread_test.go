// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **Reading the table moves a forked body's inherited `+` off a number no
// job holds** (#5429). Measured 2026-10-02 on zsh 5.9.2 under `-f -c`, pids
// masked. See interp.Runner.settleFrozenMarks.
func TestReadingTheTableMovesAFrozenPlus(t *testing.T) {
	const two = "/bin/sleep 1 & "
	for _, c := range []struct{ name, src, want string }{
		{
			"parentheses, the + on the - job",
			two + two + two + `( /bin/sleep 0.3 & print -n "${(kv)jobstates} / "; jobs >/dev/null; print ${(kv)jobstates} ); :`,
			"2 running:-:P=running / 2 running:+:P=running\n",
		},
		{
			"parentheses, the - on the parentheses",
			two + two + two + two + `( /bin/sleep 0.3 & jobs %- >/dev/null; echo s=$? ); :`,
			"s=0\n",
		},
		{
			"parentheses, two jobs",
			two + two + two + two + two + `( /bin/sleep 0.3 & /bin/sleep 0.3 & jobs %% >/dev/null; print ${(kv)jobstates} ); :`,
			"2 running:-:P=running 3 running:+:P=running\n",
		},
		{
			"a brace element, the - is nothing",
			two + two + `{ /bin/sleep 0.3 & jobs %- 2>&1; print ${(kv)jobstates} } | cat; :`,
			"zsh:jobs:1: no previous job\n1 running:+:P=running\n",
		},
		{
			"kill finds the job",
			two + two + two + `( /bin/sleep 0.3 & kill -0 %%; echo s=$? ); :`,
			"s=0\n",
		},
		{
			"starting a job sees an element's end without reaping it",
			`echo | { /bin/sleep 0.3 & print ${(kv)jobstates} }`,
			"1 running:-:P=running 2 running:+:P=running\n",
		},
		{
			"control: a : reads nothing",
			two + two + two + `( /bin/sleep 0.3 & :; print ${(kv)jobstates} ); :`,
			"2 running:-:P=running\n",
		},
		{
			"control: no + to move",
			`( /bin/sleep 0.3 & jobs ); :`,
			"[2]    running    /bin/sleep 0.3\n",
		},
		{
			"control: the + on the parentheses",
			two + `( /bin/sleep 0.3 & jobs ) | cat; :`,
			"[2]    running    /bin/sleep 0.3\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := statePIDs.ReplaceAllString(runZshC(c.src), ":P="); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
