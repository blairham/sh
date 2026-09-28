// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The words behind `exec` are an **ordinary command word list** here: this
// shell's functions and builtins are reached, a precommand modifier standing
// in front of either is read as a modifier, and the shell ends with that
// command's status. The other four columns answer `exec: …: not found`.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME and
// stdin at /dev/null (#5047).
//
// **The rows that discriminate are the builtins with no external of the same
// name.** `exec echo hi` writes `hi` under either reading, because `/bin/echo`
// exists and the replacement road finds it; `exec true` and `exec false` are
// the same trap. A grid made of those agrees for a reason that has nothing to
// do with the question.
func TestExecReachesTheShellsOwnCommands(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A builtin, and the shell ends — `after` is never written.
		{`exec :; print after`, ""},
		{`exec print -n hi; print after`, "hi"},
		{`exec typeset v=1; print after`, ""},
		{`exec unset v; print after`, ""},
		{`exec eval 'print E'; print after`, "E"},
		{`exec . /dev/null; print after`, ""},
		// A function, which is reached before the builtin table.
		{`f(){ print F; }; exec f; print after`, "F"},
		// A modifier in front of either, which is the shape this was filed
		// for: the chain is read rather than looked up on PATH.
		{`exec builtin echo hi; print after`, "hi"},
		{`exec builtin builtin echo hi; print after`, "hi"},
		{`exec exec echo hi; print after`, "hi"},
		{`exec exec exec echo hi; print after`, "hi"},
		{`exec builtin exec echo hi; print after`, "hi"},
		{`exec - echo hi; print after`, "hi"},
		{`exec noglob echo a[b]c; print after`, "a[b]c"},
		{`exec builtin noglob echo a[b]c; print after`, "a[b]c"},
		// The shell ends whatever the command did — a bad option, a refused
		// `shift`, a `break` outside a loop. This is measured, not assumed.
		{`exec unset -q; print after`, "zsh:unset:1: bad option: -q"},
		{`exec shift; print after`, "zsh:shift:1: shift count must be <= $#"},
		{`exec break; print after`, "zsh:break:1: not in while, until, select, or repeat loop"},
		// And the builtin that ran names *itself* in the location, not the
		// `exec` that reached it.
		{`exec false; print after`, ""},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell to have ended", tc.src, out)
		}
	}
}

// TestExecThatReachedNoCommandLeavesTheShellStanding: the one row on that road
// that does not end the script, and the noun is **whether a command ran** —
// not whether one succeeded.
//
// Every other failure above ends it, including a name PATH did not have. What
// is different here is that a prefix builtin reached nothing at all, so the
// `exec` is the redirection form and the shell is still the shell.
func TestExecThatReachedNoCommandLeavesTheShellStanding(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`exec builtin; print "A st=$?"`, "A st=0"},
		{`exec command; print "A st=$?"`, "A st=0"},
		{`exec exec; print "A st=$?"`, "A st=0"},
		{`exec builtin nosuchb; print "A st=$?"`, "zsh:1: no such builtin: nosuchb\nA st=1"},
		{`f(){ print F; }; exec builtin f; print "A st=$?"`, "zsh:1: no such builtin: f\nA st=1"},
		// The controls: a name PATH did not have ends the script, and so
		// does `command` handed one — so it is not "a lookup failed".
		{`exec nosuchcmd; print "A st=$?"`, "zsh:1: command not found: nosuchcmd"},
		{`exec command nosuchc; print "A st=$?"`, "zsh:1: command not found: nosuchc"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
	// And such an `exec` is the redirection form, so what it was written
	// with is kept: everything the script writes afterwards lands in the
	// file, exactly as a bare `exec >f` does.
	out, _ := runZsh(t, t.TempDir(), `exec builtin >f; print after; exec >&2; read -r x <f; print -r -- $x`)
	if strings.TrimSpace(out) != "after" {
		t.Errorf("redirection form = %q, want the redirection kept", out)
	}
}
