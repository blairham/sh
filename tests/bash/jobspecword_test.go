// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A command word beginning with `%` is a job specification here too — and it
// is asked of **the name the lookup is about to use**, not of the word the
// script wrote. That is the other value of the form, and it is why the axis
// is a form rather than a flag: a plain `%prep` row agrees with zsh, and only
// the rows below can tell the two readings apart.
//
// Measured 2026-09-29 on bash 5.3.20 (`/opt/homebrew/bin/bash`), script
// files. Every one of these is `fg: no job control` at 1 here and `command
// not found: %prep` at 127 in zsh.
//
// **Two rows of this reading are measured and not yet answered**, and they
// are named here rather than smoothed over, because a test file that lists
// only what passes is the wrong record of what a change did:
//
//	command %prep        bash: fg: no job control, 1   |  here: not found, 127
//	%prep | cat          bash: %prep: command not found |  here: fg: no job control
//
// The first is the reading reaching inside `command`, which strips itself
// before the lookup this shell makes. The second is the reading **not**
// reaching a pipeline element — where zsh's does, so it is a difference
// between the two columns and not a second question. Both go to #4436.
func TestThePercentWordIsReadAfterExpansion(t *testing.T) {
	const said = "bash: line 1: fg: no job control\n"
	for _, tc := range []struct{ name, src, want string }{
		{"written plainly", "%prep\n", said},
		{"quoted", "\"%prep\"\n", said},
		{"escaped", "\\%prep\n", said},
		{"out of a parameter", "x=%prep\n$x\n", "bash: line 2: fg: no job control\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, tc.src+"echo \"st=$?\"\n")
			if out != "st=1\n" || errs != tc.want {
				t.Errorf("out %q err %q, want %q / %q", out, errs, "st=1\n", tc.want)
			}
		})
	}
}

// And a `%` anywhere but the command word is an ordinary character, which is
// the control that keeps the rule about the command word rather than about
// the byte.
func TestAPercentArgumentIsJustAWord(t *testing.T) {
	out, errs := runBashSplit(t, "echo %prep\n")
	if out != "%prep\n" || errs != "" {
		t.Errorf("out %q err %q, want it printed", out, errs)
	}
}
