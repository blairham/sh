// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A **run-time** diagnostic at a prompt carries the prompt's location, not the
// general one — the other half of the seam #1892 fixed for a parse failure.
//
// Measured 2026-09-11 with `printf 'nosuchcmd_zz\n' | <shell> -i`: no shell in
// the panel writes a line number for a diagnostic about a typed line. Every
// line typed at a prompt is line 1, so the number said nothing whatever it
// was, and this shell wrote one for every complaint in a session (#2024).
//
// The route is what moves rather than the wording, so every complaint moves
// together: the rows below are a command name, a builtin's own complaint, an
// expansion failure and a redirection that will not open, which reach the
// location through four different call sites.
func TestARunTimeDiagnosticAtAPromptNamesNoLine(t *testing.T) {
	sh := shell()
	// A located dialect, so that a line would show if one were written. The
	// prompt answer is the same location the parse-failure route uses.
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	dg.BuiltinLocation = interp.LocationLineWord
	dg.PromptBuiltinLocation = interp.LocationNameOnly
	sh.Diagnostics = dg
	for _, tc := range []struct{ name, typed string }{
		{"a command name", "nosuchcmd_zz\n"},
		{"a builtin's complaint", "cd /nope_zz\n"},
		{"an expansion failure", "echo ${undefined_zz?boom}\n"},
		{"a redirection", "echo hi > /nope_zz/x\n"},
		// A command substitution runs inside the typed line and is located
		// by it too. A trap's own command is the same route and is pinned
		// in dialect/bash, where the axes a trap asks have answers.
		{"a command substitution", "x=$(cd /nope_zz)\n"},
		// `eval` is text handed over rather than a file, and is located the
		// way the text around it is.
		{"text handed to eval", "eval 'cd /nope_zz'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, _ := runPipedShell(t, sh, tc.typed, "testsh", "-i")
			if errs == "" {
				t.Fatalf("nothing was reported for %q", tc.typed)
			}
			if strings.Contains(errs, "line 1") {
				t.Errorf("%q said %q, want no line at a prompt", tc.typed, errs)
			}
			if !strings.Contains(errs, "testsh: ") {
				t.Errorf("%q said %q, want the shell named", tc.typed, errs)
			}
		})
	}
}

// And a script is untouched, which is what says the answer above belongs to
// the route rather than to the diagnostic: the same failures in a file still
// name the line they are on.
func TestARunTimeDiagnosticInAScriptStillNamesItsLine(t *testing.T) {
	sh := shell()
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	sh.Diagnostics = dg
	_, errs, _ := runArgs(t, sh, "testsh", "-c", "echo one\nnosuchcmd_zz")
	if !strings.Contains(errs, "line 2") {
		t.Errorf("a `-c` program said %q, want the line named", errs)
	}
}

// A **sourced file** keeps its own name and line inside a session, which is
// the one place the prompt's answer must not reach: a file is a file however
// it was reached.
//
// Measured 2026-09-11 on zsh 5.9.2, which is the panel member that names both:
// `. ./bad.sh` typed at a prompt reports `bad.sh:cd:1: no such file or
// directory`, where the same `cd` typed at that prompt reports no location at
// all. Without this the prompt's wording reached every line of every file a
// session sources, and the rc file is one of them.
func TestASourcedFileInASessionKeepsItsLine(t *testing.T) {
	sh := shell()
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	sh.Diagnostics = dg
	path := filepath.Join(t.TempDir(), "bad.sh")
	if err := os.WriteFile(path, []byte("echo one\nnosuchcmd_zz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs, _ := runPipedShell(t, sh, ". "+path+"\n", "testsh", "-i")
	if !strings.Contains(errs, "line 2") {
		t.Errorf("a file sourced at a prompt said %q, want its own line named", errs)
	}
}

// A builtin's complaint has a location of its own, and a dialect may answer it
// differently from the shell's own messages in the same session.
//
// Measured 2026-09-11 on zsh 5.9.2 under `-i`: `nosuchcmd_zz` is
// `zsh: command not found: nosuchcmd_zz` — the shell's name — and `cd /nope`
// is `cd: no such file or directory: /nope`, the *builtin's* name and no shell
// at all. Two answers in one session, which is what PromptBuiltinLocation is
// for and why applying a prompt location to Location alone would leave half a
// session wrong.
func TestABuiltinsComplaintAtAPromptHasItsOwnLocation(t *testing.T) {
	sh := shell()
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.BuiltinLocation = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	dg.PromptBuiltinLocation = interp.LocationBuiltinNameOnly
	sh.Diagnostics = dg
	_, errs, _ := runPipedShell(t, sh, "cd /nope_zz\n", "testsh", "-i")
	if !strings.Contains(errs, "cd: ") || strings.Contains(errs, "testsh: cd") ||
		strings.Contains(errs, "line 1") {
		t.Errorf("a builtin at a prompt said %q, want the builtin named alone", errs)
	}
	// And the shell's own complaint in the same session keeps the shell's
	// name, which is what makes the two fields two fields.
	_, errs, _ = runPipedShell(t, sh, "nosuchcmd_zz\n", "testsh", "-i")
	if !strings.Contains(errs, "testsh: ") || strings.Contains(errs, "line 1") {
		t.Errorf("the shell's own complaint said %q, want the shell named and no line", errs)
	}
}
