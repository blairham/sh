// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// onecmdShell is a shell that has the `onecmd` name, which is a dialect's
// declaration rather than the substrate's: dash does not have it at all, so it
// lives in extraSetOptions and a bare core shell would answer `invalid option
// name`. Declared here rather than borrowing a dialect so that what these
// tests exercise is the front end's reading and the substrate's option, which
// is all the option is.
func onecmdShell() driver.Shell {
	sh := shell()
	sh.Register = func(r *interp.Runner) { r.AddSetOptions("onecmd") }
	return sh
}

// TestOneCommandStopsAfterTheLineThatSetIt pins `set -o onecmd`, whose whole
// effect is on *reading*: the line that turns it on finishes, and then nothing
// more is read.
//
// The line rather than the statement is the unit, which is the part a change to
// this loop could quietly lose, and it is measured rather than chosen. bash
// 5.3.15 from a script file:
//
//	set -o onecmd; echo B          -> B
//	set -o onecmd; echo B; echo C  -> B and C
//	{ set -o onecmd; echo B; }     -> B
//
// All three run everything on the line and then stop; none stop at the
// statement that asked. It is also not one-way the way `set -n` is: the state
// is read once the line is over, so a `set +o onecmd` later on that same line
// cancels the stop outright.
//
// The letter is a different question and deliberately not asserted here. bash
// and ksh93 take `set -t`, zsh has it and refuses to move it — `can't change
// option: -t`, fatally — and dash does not have the letter at all.
func TestOneCommandStopsAfterTheLineThatSetIt(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			name: "the next line is not read",
			src:  "echo A\nset -o onecmd\necho B\necho C\n",
			want: "A\n",
			why:  "the option's own line is the last one read, so neither line after it runs",
		},
		{
			name: "the rest of its own line still runs",
			src:  "set -o onecmd; echo B\necho C\n",
			want: "B\n",
			why:  "the unit is the line: B is on the line that asked and runs, C is on the next and does not",
		},
		{
			name: "every statement on its own line runs",
			src:  "set -o onecmd; echo B; echo C\necho D\n",
			want: "B\nC\n",
			why:  "stopping at the statement that asked would drop C, which bash runs",
		},
		{
			name: "inside a compound on one line",
			src:  "echo A\n{ set -o onecmd; echo B; }\necho C\n",
			want: "A\nB\n",
			why:  "the brace group is one line, so it finishes before the shell stops reading",
		},
		{
			name: "inside a function call",
			src:  "f() { set -o onecmd; echo in; }\nf\necho after\n",
			want: "in\n",
			why:  "the call's line finishes and the line after it is never read",
		},
		{
			name: "cancelled on the same line",
			src:  "set -o onecmd; set +o onecmd; echo B\necho C\n",
			want: "B\nC\n",
			why:  "the state is read when the line is over, so turning it back off first leaves nothing to stop — unlike `set -n`, which never comes back",
		},
		{
			name: "off is a no-op",
			src:  "set +o onecmd\necho A\n",
			want: "A\n",
			why:  "asking for the state the shell is already in moves nothing",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runArgs(t, onecmdShell(), "testsh", writeScript(t, c.src))
			if errs != "" {
				t.Fatalf("stderr %q, want none", errs)
			}
			if code != 0 {
				t.Fatalf("status %d, want 0 — a shell that stops early still ends at the status of what it ran", code)
			}
			if out != c.want {
				t.Errorf("got %q, want %q — %s", out, c.want, c.why)
			}
		})
	}
}

// TestOneCommandDoesNotTruncateACommandString: a `-c` program is one command
// however many lines it spans, so the option never withholds any of it.
//
// Measured on bash 5.3.15 — the same four lines that stop after the second in a
// *file* all run when handed over as one `-c` string. Worth its own test
// because the obvious implementation, stop once the line is over, is right for
// a file and wrong here, and nothing else in the suite would have caught it:
// reading a `-c` string a line at a time is this front end's business and not
// something a script can observe.
func TestOneCommandDoesNotTruncateACommandString(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "several lines",
			src:  "echo A\nset -o onecmd\necho B\necho C",
			want: "A\nB\nC\n",
		},
		{
			name: "one line",
			src:  "set -o onecmd; echo A; echo B",
			want: "A\nB\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runArgs(t, onecmdShell(), "testsh", "-c", c.src)
			if errs != "" {
				t.Fatalf("stderr %q, want none", errs)
			}
			if code != 0 {
				t.Fatalf("status %d, want 0", code)
			}
			if out != c.want {
				t.Errorf("got %q, want %q — the whole string is the one command, so all of it runs", out, c.want)
			}
		})
	}
}

// TestOneCommandIsWrittenAndReadBackThroughTheListing is the round trip that
// made this worth fixing rather than a missing nicety.
//
// A shell snapshot generator dumps `set -o` and sources the result ahead of
// every command it runs, and it picks the rows with `grep "on"` — which matches
// the *spelling* `onecmd` whatever its state. So every snapshot taken from a
// bash shell contains `set -o onecmd`, and we answered each one with a
// diagnostic on stderr where bash takes it at 0.
func TestOneCommandIsWrittenAndReadBackThroughTheListing(t *testing.T) {
	out, errs, code := runArgs(t, onecmdShell(), "testsh", "-c",
		"set -o onecmd; set +o | grep -c 'set -o onecmd'; set +o onecmd")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "1\n"; out != want {
		t.Errorf("got %q, want %q — written through `set -o` and read back through `set +o`", out, want)
	}
}
