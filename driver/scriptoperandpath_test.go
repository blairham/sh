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

// writeAt puts a file where a test needs one.
func writeAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A script operand written without a slash is looked for along `$PATH`, in the
// dialects that look — #4159.
//
// Measured 2026-09-22 with a script on a PATH directory and nowhere else: bash
// and ksh93 run it and zsh and dash report a file that is not there. The
// operand that *has* a slash is never searched for in any of them.
//
// It is what `bash ls` does, and what it then says: the search finds `/bin/ls`,
// the shell reads it, and it declines to run what came out.
func TestAScriptOperandIsSearchedForOnPath(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "onpath"), "echo RAN\n")

	sh := shell()
	sh.Semantics.ScriptOperandSearchedOnPath = true
	sh.Env = []string{"PATH=" + dir}
	out, errs, code := runArgs(t, sh, "testsh", "onpath")
	if code != 0 || out != "RAN\n" {
		t.Errorf("got %q status %d stderr %q, want the file on PATH to have run", out, code, errs)
	}

	// And the dialect that does not look reports a file that is not there,
	// from the same environment and the same operand.
	sh = shell()
	sh.Env = []string{"PATH=" + dir}
	out, _, code = runArgs(t, sh, "testsh", "onpath")
	if code == 0 || out != "" {
		t.Errorf("got %q status %d, want no search at all", out, code)
	}
}

// A slash says the operand is a path, so nothing is searched for. Without this
// the search would answer for `./name` too, and a file that is not in the
// current directory would quietly run one of the same name from somewhere on
// PATH — which no shell in the panel does.
func TestASlashInTheOperandStopsTheSearch(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "onpath"), "echo MUST NOT RUN\n")

	sh := shell()
	sh.Semantics.ScriptOperandSearchedOnPath = true
	sh.Env = []string{"PATH=" + dir}
	out, _, code := runArgs(t, sh, "testsh", "./onpath")
	if code == 0 || strings.Contains(out, "MUST NOT RUN") {
		t.Errorf("got %q status %d, want the operand taken as a path", out, code)
	}
}

// A candidate this shell cannot read is passed over and the search goes on,
// ending at "no such file" rather than at a permission refusal — measured, a
// mode-000 file on PATH leaves `bash unread2` at 127 with the shell's own name
// in front of it.
func TestAnUnreadableCandidateIsPassedOver(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeAt(t, filepath.Join(first, "twice"), "echo MUST NOT RUN\n")
	if err := os.Chmod(filepath.Join(first, "twice"), 0o000); err != nil {
		t.Fatal(err)
	}
	writeAt(t, filepath.Join(second, "twice"), "echo RAN\n")

	sh := shell()
	sh.Semantics.ScriptOperandSearchedOnPath = true
	sh.Env = []string{"PATH=" + first + string(os.PathListSeparator) + second}
	out, errs, code := runArgs(t, sh, "testsh", "twice")
	if code != 0 || out != "RAN\n" {
		t.Errorf("got %q status %d stderr %q, want the second candidate", out, code, errs)
	}
}

// Two names come out of a search that found something, and they are not the
// same name: `$0` is the word that was typed and a diagnostic names what the
// search resolved. Measured — `bash zeroprobe` answers `$0` of `zeroprobe` and
// writes `/…/pdir/zeroprobe: line 1: …` about a failure inside it.
func TestTheSearchKeepsTheTypedWordAsDollarZero(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "named"), "echo \"zero=[$0]\"\nnosuchcommand\n")

	sh := shell()
	sh.Semantics.ScriptOperandSearchedOnPath = true
	sh.Diagnostics.Location = interp.LocationLineWord
	sh.Env = []string{"PATH=" + dir}
	out, errs, _ := runArgs(t, sh, "testsh", "named")
	if out != "zero=[named]\n" {
		t.Errorf("stdout = %q, want $0 to be the word that was typed", out)
	}
	if want := filepath.Join(dir, "named") + ": line 2:"; !strings.Contains(errs, want) {
		t.Errorf("stderr = %q, want %q in it", errs, want)
	}
}

// A file that is read and then declined, which is the other half of what
// `bash ls` does: `<path>: <path>: cannot execute binary file` at 126, with no
// errno behind it — nothing failed to open.
func TestAScriptOperandWhoseContentIsNotShellText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image")
	writeAt(t, path, "\x7fELF\x00\x01\x02\n")

	sh := shell()
	sh.Semantics.BinaryContentIsNotRunAsAScript = interp.Yes
	sh.Diagnostics.ScriptBinaryContent = "%[1]s: cannot execute binary file"
	sh.Diagnostics.ScriptOperandNamedByItselfOnceOpened = true
	sh.Diagnostics.ScriptNotFound = "%[1]s: %[2]s"
	sh.Diagnostics.ScriptNotReadableStatus = 126
	out, errs, code := runArgs(t, sh, "testsh", path)
	if code != 126 || out != "" {
		t.Errorf("got %q status %d, want 126 and nothing run", out, code)
	}
	if want := path + ": " + path + ": cannot execute binary file\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}

	// A dialect with no wording for it reads whatever it was handed, which is
	// what this front end did before there was a field.
	sh = shell()
	sh.Semantics.BinaryContentIsNotRunAsAScript = interp.Yes
	_, _, code = runArgs(t, sh, "testsh", path)
	if code == 126 {
		t.Errorf("status %d, want the file read rather than declined", code)
	}
}
