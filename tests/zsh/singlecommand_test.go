// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// `singlecommand` is the one name in this shell's table whose answer depends
// on where the request came from: refused to a running script, taken on the
// command line that started the shell.
//
// Measured on zsh 5.9.2, 2026-09-10, against a three-line script — see
// Semantics.ImmovableOptionsSetAtInvocation and setopt.go's
// singleCommandOption for the whole of the measurement (#1730).

// zshScriptFileAt writes src to a scratch file and answers its path.
func zshScriptFileAt(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestTheInvocationTakesSingleCommandUnderEverySpelling: the letter, the
// option's own name and the spelling borrowed from another shell are three
// doors into one state, so all three have to be behind the alias resolution
// rather than in front of it.
func TestTheInvocationTakesSingleCommandUnderEverySpelling(t *testing.T) {
	script := zshScriptFileAt(t, "echo P1\necho P2\necho P3\n")
	for _, argv := range [][]string{
		{"zsh", "-t", script},
		{"zsh", "-o", "singlecommand", script},
		{"zsh", "-o", "onecmd", script},
	} {
		var out, errs bytes.Buffer
		code := driver.MainArgs(zshWriting(&out, &errs), argv)
		if out.String() != "P1\n" || errs.String() != "" || code != 0 {
			t.Errorf("%v ran %q / said %q status %d, want P1 and nothing said",
				argv[1:], out.String(), errs.String(), code)
		}
	}
}

// TestTheOptionIsVisibleToTheWholeNamespaceAfterTheInvocation: one state, and
// the three doors that read it — the listing, the condition and `$-` — all
// have to see it. Measured: `zsh -t -c 'echo "$-"'` is `569Xt`, and a `-t`
// script's first line reporting `setopt` writes `singlecommand`.
func TestTheOptionIsVisibleToTheWholeNamespaceAfterTheInvocation(t *testing.T) {
	script := zshScriptFileAt(t,
		"setopt | grep single\n[[ -o singlecommand ]]; echo \"cond=$?\"\n")
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-t", script})
	// Only the first line runs, which is the option working; what matters
	// here is that the line it did run saw the option.
	if out.String() != "singlecommand\n" || code != 0 {
		t.Errorf("listed %q status %d, want the option in the listing", out.String(), code)
	}
	out.Reset()
	errs.Reset()
	code = driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-t", "-c", `case $- in *t*) echo has-t ;; *) echo no-t ;; esac`})
	if out.String() != "has-t\n" || code != 0 {
		t.Errorf("$- = %q status %d, want the letter", out.String(), code)
	}
}

// TestACommandStringIsReadToItsEnd: the route the option does not stop.
// Measured, `zsh -t -c $'echo A\necho B'` writes both lines, which is
// Semantics.OneCommandStopsACommandString answered `No`.
func TestACommandStringIsReadToItsEnd(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-t", "-c", "echo A\necho B"})
	if out.String() != "A\nB\n" || code != 0 {
		t.Errorf("ran %q status %d, want both lines", out.String(), code)
	}
}

// TestAScriptStillMayNotMoveItInEitherDirection: the refusal that makes the
// invocation a route split rather than a loosening. Measured — `unsetopt
// singlecommand` after `zsh -t` is `can't change option: singlecommand`, so
// the grant really is the invocation's and not the state's.
func TestAScriptStillMayNotMoveItInEitherDirection(t *testing.T) {
	script := zshScriptFileAt(t, "unsetopt singlecommand\necho AFTER\n")
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-t", script})
	if want := script + ":unsetopt:1: can't change option: singlecommand\n"; errs.String() != want {
		t.Errorf("said %q, want %q", errs.String(), want)
	}
	if out.String() != "" || code == 0 {
		t.Errorf("ran %q at %d, want nothing after the refused line", out.String(), code)
	}
}
