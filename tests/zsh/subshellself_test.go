// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **A spec naming the `( … )` subshell itself is passed over by a plain
// `jobs` and by `wait`, and missed by every other verb** (#5320). Measured
// 2026-10-01 on zsh 5.9.2 under `-f -c`, byte for byte, with the subshell
// never the last command — the last command of a `-c` string is not forked
// there, which is another route. See interp.Runner.subshellSelfLookup.
func TestASubshellsOwnSpecIsPassedOver(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"%1", `(jobs %1; echo u=$?; wait %1; echo w=$?; kill -0 %1; echo k=$?); :`,
			"u=0\nw=0\nzsh:kill:1: %1: no such job\nk=1\n",
		},
		{
			"the inherited +", `/bin/sleep 0.3 & ( jobs %%; echo s=$?; jobs %-; echo t=$? ); wait`,
			"s=0\nzsh:jobs:1: no previous job\nt=127\n",
		},
		{"control: a parent with no job", `( jobs %%; echo s=$?); :`, "zsh:jobs:1: no current job\ns=127\n"},
		{"in a substitution", `x=$(jobs %1; echo u=$?; wait %1; echo w=$?); echo "$x"`, "u=0\nw=0\n"},
		{
			"a + on a number nothing holds, a - on 2", `/bin/sleep 0.3 & /bin/sleep 0.3 & /bin/sleep 0.3 & ( jobs %%; echo s=$?; jobs %-; echo t=$? ); wait`,
			"s=0\nzsh:jobs:1: no previous job\nt=127\n",
		},
		{
			"a - on 1", `/bin/sleep 0.4 & /bin/sleep 0.4 & ( /bin/sleep 0.2 & jobs %%; echo s=$?; jobs %-; echo t=$? ); wait`,
			"[2]  + running    /bin/sleep 0.2\ns=0\nt=0\n",
		},
		{
			"the verbs that miss it", `( wait %1; echo w=$?; jobs -l %1; echo l=$?; disown %1; echo d=$? ); :`,
			"w=0\nzsh:jobs:1: %1: no such job\nl=127\nzsh:disown:1: %1: no such job\nd=127\n",
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
