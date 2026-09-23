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

// Sixteen here-documents to a command, and the seventeenth is refused.
//
// Measured 2026-09-23 on bash 5.3.20 and on the 5.3.15 in the image this shell's
// own suite is graded in: sixteen runs, seventeen is `maximum here-document count
// exceeded` at the command's line, the input ends and the shell exits 2. dash,
// ksh93 and zsh have no bound at all, which is why the number is this dialect's.
//
// It is CVE-2014-7186's bound — an unbounded redirection stack — and bash's own
// regression test for that CVE is where it was found: `exportfunc1.sub` opens
// eighteen, and this shell ran past it (#4143).
func TestAHeredocCountBeyondTheBoundIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		n       int
		refused bool
	}{
		{"one below the bound", 15, false},
		{"the bound itself", 16, false},
		{"one past it", 17, true},
		{"well past it", 30, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runHeredocScript(t, dir, heredocCommand(c.n)+"echo after\n")
			if !c.refused {
				if !strings.Contains(out, "after") || st != 0 {
					t.Errorf("output %q at %d, want the command to run", out, st)
				}
				return
			}
			if !strings.Contains(out, "line 1: maximum here-document count exceeded") {
				t.Errorf("output %q, want the refusal at the command's line", out)
			}
			if st != 2 {
				t.Errorf("status %d, want 2", st)
			}
			// The input ends, which is what makes this a parse failure rather
			// than a command that went wrong.
			for _, line := range strings.Split(out, "\n") {
				if line == "after" {
					t.Errorf("output %q, want nothing after the refusal", out)
				}
			}
		})
	}
}

// The bound is on one **command** and not on the input, which is the measurement
// that says what is being counted: the list of operators still waiting for a
// body, drained at every newline.
func TestTheHeredocBoundIsPerCommand(t *testing.T) {
	dir := t.TempDir()
	out, st := runHeredocScript(t, dir, heredocCommand(16)+heredocCommand(16)+"echo after\n")
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("output %q at %d, want both commands to run", out, st)
	}
	if strings.Contains(out, "exceeded") {
		t.Errorf("output %q, want no refusal", out)
	}
}

// heredocCommand is a `cat` with n here-documents on it and their n bodies.
func heredocCommand(n int) string {
	var b strings.Builder
	b.WriteString("cat")
	for range n {
		b.WriteString(" <<EOF")
	}
	b.WriteString("\n")
	for range n {
		b.WriteString("EOF\n")
	}
	return b.String()
}

// runHeredocScript runs src from a file through the front end, which is what a
// parse failure needs: the dialect test helper fails the test on one, and a
// refused parse is exactly what these rows are about.
func runHeredocScript(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	path := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", path})
	return out.String() + errs.String(), code
}
