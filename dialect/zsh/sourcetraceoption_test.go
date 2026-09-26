// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `setopt sourcetrace` writes one line to standard error as the shell enters
// each file it sources — the ordinary trace prefix with the literal word
// `<sourcetrace>` where a command's words would go.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26, from
// a script file. The option *off* is the control on every row and already
// agreed before this was wired, so a row that passes in both states says the
// option is what moved (#4550).
func TestSourceTraceWritesOneLinePerSourcedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.zsh", "print a\n")
	out, st, errs := runZshSplit(t, dir, "setopt sourcetrace\n. ./lib.zsh\nprint back\n")
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "+./lib.zsh:1> <sourcetrace>\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "a\nback\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// The control: with the option off the same script writes nothing extra, so
// the row above cannot be a shell that traces whatever it is asked.
func TestSourceTraceOffWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.zsh", "print a\n")
	out, _, errs := runZshSplit(t, dir, "unsetopt sourcetrace\n. ./lib.zsh\nprint back\n")
	if errs != "" {
		t.Errorf("stderr = %q, want nothing", errs)
	}
	if want := "a\nback\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// `PS4` decides the prefix here exactly as it decides an `xtrace` line's, in
// this dialect's own prompt language.
func TestSourceTraceUsesPS4(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.zsh", "print a\n")
	_, _, errs := runZshSplit(t, dir, "PS4='@%N:%i> '\nsetopt sourcetrace\n. ./lib.zsh\n")
	if want := "@./lib.zsh:1> <sourcetrace>\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

// It is a second event on the same writer rather than a mode of `xtrace`:
// each option traces with the other off, and with both on the entry line
// lands between the `.` the caller traced and the file's first command.
func TestSourceTraceAndXtraceAreSeparateEvents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.zsh", "print a\n")
	_, _, errs := runZshSplit(t, dir, "setopt sourcetrace xtrace\n. ./lib.zsh\n")
	lines := strings.Split(strings.TrimSuffix(errs, "\n"), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "> . ./lib.zsh") ||
		lines[1] != "+./lib.zsh:1> <sourcetrace>" ||
		lines[2] != "+./lib.zsh:1> print a" {
		t.Errorf("stderr = %q, want the entry between the `.` and the file's first command", errs)
	}
}

// Nesting is the same event again, and a file that could not be opened is not
// an entry — which is what says the line marks the file rather than the call.
func TestSourceTraceMarksEachEntryAndNoFailedOpen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "inner.zsh", "print inner\n")
	writeFile(t, dir, "outer.zsh", ". ./inner.zsh\n")
	_, _, errs := runZshSplit(t, dir, "setopt sourcetrace\n. ./outer.zsh\n. ./nope.zsh\n")
	want := "+./outer.zsh:1> <sourcetrace>\n+./inner.zsh:1> <sourcetrace>\n"
	if !strings.HasPrefix(errs, want) {
		t.Errorf("stderr = %q, want it to start %q", errs, want)
	}
	if strings.Contains(errs, "nope.zsh:1> <sourcetrace>") {
		t.Errorf("stderr = %q, want no entry for a file that could not be opened", errs)
	}
}
