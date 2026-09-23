// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// The `CHLD` trap fires once per child the shell reaps, and this shell has no
// child for most of what a real one forks.
//
// Every row was run in bash 5.3.20 and here before it was written down —
// 2026-09-23, `env -i PATH=/usr/bin:/bin`, one script per row so that a late
// arrival cannot be counted against the next case. The counter is read after a
// `wait` and a settling command, because the condition is raised between
// commands and a read on the line after the fork is a race rather than a
// measurement.
//
// The shapes that already worked are here as well as the ones that did not.
// Three of them worked *by accident* — a background subshell running an
// external fired for the external rather than for the subshell — so a test
// that only covered the broken rows would leave the accident in place.
func TestTheChildTrapFiresOncePerFork(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      int
	}{
		// Nothing forks, so nothing is counted. The control for every row
		// below it.
		{"a builtin in the foreground forks nothing", `:`, 0},
		// A goroutine here and a fork in bash: the rows this change is for.
		{"a builtin in the background is a fork", `{ :; } &`, 1},
		{"and so is a function", `f() { :; }; f &`, 1},
		{"and so is a subshell", `( : ) &`, 1},
		{"five of them are five", `for i in 1 2 3 4 5; do ( : ) & done`, 5},
		// A real process, which worked before and must keep working.
		{"an external in the foreground", `/bin/echo x >/dev/null`, 1},
		{"a pipeline forks per element", `/bin/echo a >/dev/null | /bin/cat >/dev/null`, 2},
		{"a command substitution is one", `x=$(/bin/echo y)`, 1},
		// The accident: two processes inside one fork. The shell reaps the
		// subshell, and the subshell reaps the two — so the shell counts one,
		// and counting the kernel's arrivals counted two.
		{
			"a subshell is one however many commands are in it",
			`( /bin/echo a >/dev/null; /bin/echo b >/dev/null )`, 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "n=0\ntrap 'n=$((n+1))' CHLD\n" + c.src +
				"\nwait\n/bin/sleep 0.15\n:\n:\nprintf '%s' \"$n\"\n"
			out, status := run(t, src, nil)
			// The settling command is an external and forks too, so the
			// figure a row names is the one written here plus that one.
			want := c.want + 1
			if got := strings.TrimSpace(out); got != itoa(want) || status != 0 {
				t.Errorf("counted %q at status %d, want %d", got, status, want)
			}
		})
	}
}
