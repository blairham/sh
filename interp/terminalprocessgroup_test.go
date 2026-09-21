// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The number an interactive shell with no terminal names as the process group
// it could not hand the terminal to, and it is two answers rather than one.
//
// Measured 2026-09-21 against bash 5.3.20 at /opt/homebrew/bin/bash, with no
// controlling terminal anywhere and nothing but a pipe or a file on any
// stream. One harness started the same binary with the same command string
// twice in a row and changed exactly one thing between them — whether the
// child was put into a process group of its own:
//
//	own group (pid == pgid)     cannot set terminal process group (-1)
//	the caller's group          cannot set terminal process group (78796)
//
// Three alternating repeats gave the same pair every time, and a run with a
// wrapper process between the group leader and the shell — leader 84507,
// wrapper 84516, shell in 84507 — wrote 84507, so the second answer is the
// group rather than the parent.
//
// Both branches are asserted here rather than through a shell, because a
// test binary cannot move itself into a group of its own without changing
// the process the rest of the suite is running in.
func TestTheProcessGroupNamedWithNoTerminal(t *testing.T) {
	for _, c := range []struct {
		name      string
		pid, pgid int
		want      int
	}{
		{"a shell in the group its caller was in names that group", 78803, 78796, 78796},
		{"and one below a wrapper still names the group, not the parent", 84521, 84507, 84507},
		{"a shell that already leads its own group has none to name", 78799, 78799, -1},
		{"which is the same answer for a session leader", 1, 1, -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := terminalProcessGroup(c.pid, c.pgid); got != c.want {
				t.Errorf("pid %d in group %d named %d, want %d",
					c.pid, c.pgid, got, c.want)
			}
		})
	}
}

// And the two branches are genuinely different, which is the assertion a
// table of four rows does not make on its own: a shell that returned its own
// process group whatever its standing — which is what this shell did until
// #4012 — passes two of the rows above and is wrong on the other two.
func TestTheTwoAnswersAreNotOne(t *testing.T) {
	const pgid = 4242
	leads := terminalProcessGroup(pgid, pgid)
	follows := terminalProcessGroup(pgid+1, pgid)
	if leads == follows {
		t.Fatalf("both standings named %d; the answer does not depend on the standing", leads)
	}
	if follows != pgid {
		t.Errorf("a shell in group %d named %d, want the group", pgid, follows)
	}
	if leads != noProcessGroupToName {
		t.Errorf("a shell leading group %d named %d, want %d",
			pgid, leads, noProcessGroupToName)
	}
}
