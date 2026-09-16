// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import "testing"

// A process substitution's path handed to a function stays openable for the
// whole of the call, by every command the body runs — not only the first.
//
// Unanimous: every row is the substituted text in bash 5.3.20, zsh 5.9.2 and
// ksh93u+, measured 2026-09-16 from a script file under `env -i`. Here each
// was a bad file descriptor, in two ways. A command inside the body finishing
// took the call's pipes with it, so whatever ran after the first command could
// not open the path; and a subshell, a pipeline element or a command
// substitution started with none, so its commands were never given the
// number at all.
func TestAProcessSubstitutionOutlivesTheCommandsInsideTheCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src string }{
		{"after another command", `f() { p=$1; cat "$p"; }; f <(echo ok)`},
		{"after a test", `f() { [ -n "$1" ]; cat "$1"; }; f <(echo ok)`},
		{"in a pipeline", `f() { cat "$1" | cat; }; f <(echo ok)`},
		{"in a subshell", `f() { (cat "$1"); }; f <(echo ok)`},
		{"in a command substitution", `f() { echo "$(cat "$1")"; }; f <(echo ok)`},
		{"in a nested call, after a command", `g() { :; cat "$1"; }; f() { :; g "$1"; }; f <(echo ok)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := run(t, c.src, nil)
			if out != "ok\n" || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, "ok\n")
			}
		})
	}
}

// And the call's pipes are still closed when the call is done, which is what
// this change must not have moved: the enclosing list is handed on, never
// kept. `closed` in bash 5.3.20 and zsh 5.9.2. ksh93u+ answers `no` — the path
// still opens after the call there — which was this shell's answer in no
// dialect before this change either, and is not modeled here.
func TestTheCallStillClosesItsProcessSubstitution(t *testing.T) {
	t.Parallel()
	out, _ := run(t, `f() { v=$1; }; f <(echo no); cat "$v" 2>/dev/null || echo closed`, nil)
	if out != "closed\n" {
		t.Errorf("wrote %q, want %q", out, "closed\n")
	}
}

// `$LINENO` inside a substitution's body is the line of the script it was
// written on, as it is inside `$( … )`: 3 in bash 5.3.20, zsh 5.9.2 and
// ksh93u+ for a substitution on line 3, and it was 1 here.
func TestLinenoInsideAProcessSubstitutionIsTheScriptsLine(t *testing.T) {
	t.Parallel()
	out, _ := run(t, ":\n:\ncat <(echo \"$LINENO\")\n", nil)
	if out != "3\n" {
		t.Errorf("wrote %q, want %q", out, "3\n")
	}
}
