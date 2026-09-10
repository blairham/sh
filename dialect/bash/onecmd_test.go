// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `set -o onecmd` and its letter `set -t`, measured against bash 5.3.15 and
// bash 3.2.57 on 2026-09-10 and recorded in the corpus as the `opt/set-t-…`
// cases.
//
// The shell listed the name and then refused to set it, which is the pairing
// #1709 is about: an agent harness snapshots a shell with `shopt -p` and with
// `set -o | grep on | awk '{print "set -o " $1}'`, sources the result ahead of
// every command it runs, and `grep on` matches the *name* `onecmd` as readily
// as the status column — so the refusal landed on every command.

// runScript writes src to a file and runs it the way a shell runs a script,
// which is the route the option is about: it stops the shell that is reading.
func runScript(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), append([]string{"bash"}, args...))
	return out.String(), errs.String(), code
}

// scriptFile writes src into a scratch file and answers its path.
func scriptFile(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestOnecmdStopsTheShellThatIsReading is the option's whole effect: the line
// that turned it on finishes and nothing more is read.
func TestOnecmdStopsTheShellThatIsReading(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// The next line is never read, under either spelling.
		{"the letter", "echo A\nset -t\necho B\n", "A\n"},
		{"the name", "echo A\nset -o onecmd\necho B\n", "A\n"},
		// The line that sets it runs to its end first, so B is written.
		{"its own line finishes", "set -t; echo B\necho C\n", "B\n"},
		// And because it is read after the line rather than when it is set,
		// that same line can take it back.
		{"taken back on the same line", "set -t; set +t\necho C\n", "C\n"},
		{"taken back by name", "set -o onecmd; set +o onecmd\necho C\n", "C\n"},
		// A compound command is one line: the rest of the branch runs.
		{"a compound is one unit", "if true; then\n set -t\n echo IN\nfi\necho AFTER\n", "IN\n"},
		// The status is the last command's, not the stopping's.
		{"the status is the line's", "set -t; (exit 7)\necho NO\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runScript(t, scriptFile(t, c.src))
			if out != c.want || errs != "" {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
	// The status, separately, because the table above asserts output.
	if _, _, code := runScript(t, scriptFile(t, "set -t; (exit 7)\necho NO\n")); code != 7 {
		t.Errorf("status %d, want the last command's 7", code)
	}
}

// TestOnecmdDoesNotStopAFileItSourced draws the line the implementation turns
// on: the option stops the shell that is *reading*, and a sourced file is not
// that shell. Measured — the file runs to its own end and the shell that was
// reading when `.` returned reads no further.
func TestOnecmdDoesNotStopAFileItSourced(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner.sh")
	if err := os.WriteFile(inner, []byte("echo S1\nset -t\necho S2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(dir, "outer.sh")
	if err := os.WriteFile(outer, []byte(". "+inner+"\necho AFTER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runScript(t, outer)
	if out != "S1\nS2\n" || errs != "" || code != 0 {
		t.Errorf("out %q errs %q status %d, want S1 and S2 and no AFTER", out, errs, code)
	}
}

// TestOnecmdAtTheInvocation is the shape a script cannot set up for itself:
// the option is already on when the first line is read, so one line runs.
func TestOnecmdAtTheInvocation(t *testing.T) {
	for _, args := range [][]string{
		{"-t"},
		{"-o", "onecmd"},
	} {
		path := scriptFile(t, "echo A\necho B\n")
		out, errs, code := runScript(t, append(append([]string{}, args...), path)...)
		if out != "A\n" || errs != "" || code != 0 {
			t.Errorf("bash %v: out %q errs %q status %d, want A alone", args, out, errs, code)
		}
	}
}

// TestOnecmdReadsOnThroughACommandString is the route bash does *not* stop,
// and it is the one that matters for #1709: a harness sources its snapshot
// ahead of a `-c` command, so a shell that stopped here would run nothing.
//
// Measured: the option is on throughout — `$-` says so — and the string is
// read to its end anyway. ksh93 stops, which is why this is an axis.
func TestOnecmdReadsOnThroughACommandString(t *testing.T) {
	out, errs, code := runScript(t, "-c", "set -t\necho B\ncase $- in *t*) echo has-t;; *) echo no-t;; esac\n")
	if out != "B\nhas-t\n" || errs != "" || code != 0 {
		t.Errorf("out %q errs %q status %d, want B and has-t", out, errs, code)
	}
	// The invocation spelling of the same route.
	out, errs, code = runScript(t, "-t", "-c", "echo A\necho B\n")
	if out != "A\nB\n" || errs != "" || code != 0 {
		t.Errorf("bash -t -c: out %q errs %q status %d, want both lines", out, errs, code)
	}
}

// TestOnecmdIsListedAndSettable is the pairing itself: a name in the listing
// that is refused when it is set cannot be captured and restored. Both
// spellings, the listing, `$-` and SHELLOPTS, since a harness reads all of
// them back.
func TestOnecmdIsListedAndSettable(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -o onecmd; echo "st=$?"`, "st=0\n"},
		{`set -t; echo "st=$?"`, "st=0\n"},
		{`set +o onecmd; echo "st=$?"`, "st=0\n"},
		{`set -o | grep onecmd`, "onecmd         \toff\n"},
		{`set -o onecmd; set -o | grep onecmd`, "onecmd         \ton\n"},
		{`set -o onecmd; set +o | grep onecmd`, "set -o onecmd\n"},
		{`set -t; set +o | grep onecmd`, "set -o onecmd\n"},
		{`set -o onecmd; case $- in *t*) echo has-t;; *) echo no-t;; esac`, "has-t\n"},
		{`set -t; case $SHELLOPTS in *onecmd*) echo in;; *) echo out;; esac`, "in\n"},
		// And off again, both ways round, so the letter and the name are one
		// state rather than two that happen to agree at first.
		{`set -t; set +o onecmd; case $- in *t*) echo has-t;; *) echo no-t;; esac`, "no-t\n"},
		{`set -o onecmd; set +t; case $- in *t*) echo has-t;; *) echo no-t;; esac`, "no-t\n"},
	} {
		var out, errs bytes.Buffer
		code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", c.src})
		if out.String() != c.want || errs.String() != "" || code != 0 {
			t.Errorf("%s = %q / %q status %d, want %q and nothing said",
				c.src, out.String(), errs.String(), code, c.want)
		}
	}
}

// TestASetOListingSourcesBack is the round trip #1709 reported, written the
// way the harness writes it. `grep on` matches the option *name*, so the file
// holds `set -o onecmd` whatever the status column said — which is the reason
// "we only emit what is on" was never a defense.
func TestASetOListingSourcesBack(t *testing.T) {
	dir := t.TempDir()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c", `set -o | grep "on" | awk '{print "set -o " $1}'`,
	})
	if code != 0 || errs.String() != "" {
		t.Fatalf("listing: %q / %q status %d", out.String(), errs.String(), code)
	}
	snap := out.String()
	if !strings.Contains(snap, "set -o onecmd\n") {
		t.Fatalf("the snapshot %q does not hold the line this is about", snap)
	}
	path := filepath.Join(dir, "snap.sh")
	if err := os.WriteFile(path, []byte(snap), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errs.Reset()
	code = driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", ". " + path + "\necho COMMAND-RAN\n"})
	if out.String() != "COMMAND-RAN\n" || errs.String() != "" || code != 0 {
		t.Errorf("sourcing the snapshot: %q / %q status %d, want the command to run with nothing said",
			out.String(), errs.String(), code)
	}
}
