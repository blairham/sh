// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A writing substitution's body made before its command's redirections sees
// none of them, and one made part-way through sees those applied so far.
//
// zsh makes a list's substitutions first, so the body is handed the table as
// it was before the command. Measured 2026-10-15, zsh 5.9.2 against bash
// 5.3.20, with the body an external that lists /dev/fd/3 to 9:
//
//	: 3>/dev/null 4> >(CHK)                         zsh (empty)    bash 3
//	: 3> >(cat) 4> >(CHK)                           zsh (empty)    bash 3
//	exec 3>/dev/null; : 4> >(CHK)                   3 in both
//	{ : 4> >(CHK); } 3>/dev/null                    3 in both
//	{ echo hi; echo B >&4; } > >(sed s/^/A/) 4> >(sed s/^/b/)
//	                                                zsh Ahi bB     bash Ahi AbB
//
// The zsh dialect followed bash on every one of these, and on standard output
// that was a visible error: `: > >(cat) 4> >(cat)` printed `cat: stdout: Bad
// file descriptor`, where zsh prints nothing (#6525). The order is
// Runner's make-first, keyed on Semantics.OwnDescriptors and landed in #6531.
// These rows pin what that order means to the body rather than to the
// numbers.
//
// Builtin bodies, so nothing is needed on PATH: `true >&3` fails in a body that
// was not given 3. The control rows hold the descriptor in a table the body
// *is* given, an `exec` or an enclosing group, and must say `seen` under both
// answers. Otherwise "unseen" would only show the probe could not see
// anything.
func TestAMadeFirstBodyDoesNotSeeItsOwnCommandsRedirections(t *testing.T) {
	const look = `>({ true >&3; } 2>/dev/null && echo seen || echo unseen)`
	const first, inOrder = SavedStreamsAndPipeReadEndsTakeNumbers, OwnDescriptorsTakeNoNumber
	for _, c := range []struct {
		own       OwnDescriptorNumbers
		src, want string
	}{
		{first, `: 3>/dev/null 4> ` + look, "unseen\n"},
		{inOrder, `: 3>/dev/null 4> ` + look, "seen\n"},
		{first, `: 3> >(read -r x) 4> ` + look, "unseen\n"},
		{inOrder, `: 3> >(read -r x) 4> ` + look, "seen\n"},
		{first, `exec 3>/dev/null; : 4> ` + look, "seen\n"},
		{inOrder, `exec 3>/dev/null; : 4> ` + look, "seen\n"},
		{first, `{ : 4> ` + look + `; } 3>/dev/null`, "seen\n"},
		{inOrder, `{ : 4> ` + look + `; } 3>/dev/null`, "seen\n"},
		// Standard output: the second body writes into the shell's output
		// in zsh, and into the first body, which reads it and drops it, in
		// bash.
		{first, `: > >(read -r x) 4> >(echo wrote)`, "wrote\n"},
		{inOrder, `: > >(read -r x) 4> >(echo wrote)`, ""},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			out, errs := runOwnDescriptorsAllowingErrors(t, c.own, c.src)
			if out != c.want || errs != "" {
				t.Errorf("got %q (stderr %q), want %q", out, errs, c.want)
			}
		})
	}
}
