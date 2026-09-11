// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A redirection's target is expanded where the command it belongs to runs.
// The measured tables are at the top of interp/redirtarget.go.

// TestATargetThatCouldNotBeExpandedIsNotOpened is the core half, and it is
// unanimous: one mistake, one diagnostic. The empty string was opened
// afterwards, so a script got the unset name and then a complaint about a
// file nobody wrote.
func TestATargetThatCouldNotBeExpandedIsNotOpened(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a write", `set -u; cat /dev/null > "$NOPE_R"`},
		{"a read", `set -u; cat < "$NOPE_R"`},
		{"an append", `set -u; echo hi >> "$NOPE_R"`},
		{"on a builtin", `set -u; : > "$NOPE_R"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if !strings.Contains(out, "NOPE_R") {
				t.Errorf("out = %q, want the unset name reported", out)
			}
			if strings.Contains(out, "No such file") || strings.Contains(out, "no such file") {
				t.Errorf("out = %q, want no second complaint about the empty name", out)
			}
			if n := strings.Count(out, "\n"); n != 1 {
				t.Errorf("out = %q, want exactly one diagnostic", out)
			}
		})
	}
}

// TestATargetThatCouldNotBeExpandedLeavesTheStreamAlone. The command does not
// run, which is the half a status cannot show: without it the command ran
// with the stream it was redirecting *away from*, so `cat < "$NOPE"` sat
// reading the shell's own input.
func TestATargetThatCouldNotBeExpandedLeavesTheStreamAlone(t *testing.T) {
	out, _ := run(t, `set -u; echo CAME-HERE > "$NOPE_R"`, nil)
	if strings.Contains(out, "CAME-HERE") {
		t.Errorf("out = %q, want the command never to have run", out)
	}
}

// TestRedirectTargetExpandsInTheCommandsProcessConfinesTheWrite is the axis
// on a command the shell runs as a process of its own: Yes puts the write
// back, No leaves it.
func TestRedirectTargetExpandsInTheCommandsProcessConfinesTheWrite(t *testing.T) {
	for _, tc := range []struct {
		answer Answer
		want   string
	}{
		{Yes, "u=UNSET"},
		{No, "u=made"},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetExpandsInTheCommandsProcess = tc.answer
			out, _ := run(t, "unset u\ncat /dev/null > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\"", withSem(sem))
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestRedirectTargetExpandsInTheCommandsProcessDecidesWhatAFailureCosts is
// the same answer's other consequence, and they are one fact: an expansion
// that happened in the command's process takes the command with it, and one
// that happened here takes the shell.
func TestRedirectTargetExpandsInTheCommandsProcessDecidesWhatAFailureCosts(t *testing.T) {
	for _, tc := range []struct {
		answer    Answer
		wantAlive bool
	}{
		{Yes, true},
		{No, false},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetExpandsInTheCommandsProcess = tc.answer
			out, _ := run(t, "set -u\ncat /dev/null > \"$NOPE_R\"\nprintf 'st=%s alive' \"$?\"", withSem(sem))
			if alive := strings.Contains(out, "alive"); alive != tc.wantAlive {
				t.Errorf("out = %q, want the script alive afterwards = %v", out, tc.wantAlive)
			}
		})
	}
}

// TestATargetForThisShellsOwnCommandKeepsItsWrite is the other side of the
// line the axis is asked at, and it is unanimous in the panel: a builtin, a
// function or a group is run by this shell, so there is no other process for
// the write to land in and the axis is not asked.
func TestATargetForThisShellsOwnCommandKeepsItsWrite(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a builtin", "unset u\n: > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\""},
		{"a function", "f() { :; }\nunset u\nf > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\""},
		{"a group", "unset u\n{ :; } > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetExpandsInTheCommandsProcess = Unspecified
			out, _ := run(t, tc.src, withSem(sem))
			if out != "u=made" {
				t.Errorf("out = %q, want the write kept and nothing asked", out)
			}
		})
	}
}

// TestAQuietTargetAsksNothing. A target that expands to a name is every other
// redirection, and an unanswered dialect must still be able to open a file.
func TestAQuietTargetAsksNothing(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetExpandsInTheCommandsProcess = Unspecified
	out, st := run(t, `v=f; echo hi > "$v"; cat f`, withSem(sem))
	if out != "hi\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the file written and read back", out, st)
	}
}

// And one that writes refuses rather than guessing, which is what keeps the
// core from having an opinion it never measured.
func TestAnUnansweredTargetThatWritesIsRefused(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetExpandsInTheCommandsProcess = Unspecified
	out, st := run(t, "unset u\ncat /dev/null > \"${u:=made}\" && printf 'CAME-HERE'", withSem(sem))
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want the refusal named", out)
	}
	if strings.Contains(out, "CAME-HERE") {
		t.Errorf("out = %q, want the command never to have run", out)
	}
	if st == 0 {
		t.Error("status 0 for a refusal")
	}
}
