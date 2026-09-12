// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `fc`'s file letters, measured 2026-09-12 against zsh 5.9.2 with a scratch
// `HOME` and no startup files.
//
// The gate is asserted from outside, against the shipped binary, in
// `cmd/sh/sandboxfc_test.go`. What is here is what the letters do when
// nothing is watching — the half that decides whether the sandbox rows are
// measuring anything.
//
// Every expectation below was run side by side against the real binary under
// `zsh -f -i -c`, and the interactive half of that is not incidental: it is
// the condition the first test is about.

// fcRun runs src in a scratch directory with the shell marked **interactive**,
// which is the state zsh writes a history file in, and returns everything it
// wrote.
//
// runZshSplit's runner is not one, which is right for nearly every case in
// this package and is exactly wrong for these: see the pair in the first test.
func fcRun(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	return zshRunner(t, dir, src, true)
}

// fcScript is the same script in a shell nobody is watching — the other half of
// that pair, and the state `make sandbox` ran every route in until #2283.
func fcScript(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	return zshRunner(t, dir, src, false)
}

func zshRunner(t *testing.T, dir, src string, interactive bool) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:         dir,
		Vars:        map[string]string{"PATH": dir},
		Interactive: interactive,
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// zsh writes a history file **only when the shell is interactive**, and this
// is the pair that says so.
//
// It is the whole reason `builtin/fc-write` sat on `make sandbox`'s inert
// ledger through two other features landing: the sweep runs `-c`, so the row
// could not have gone green however faithfully the letter was implemented.
// And it is not about the list being empty — the same script seeds the list
// both times, and `fc -l` would print the entry in either.
//
// A shell that ignored interactivity would pass every other case in this file
// and fail only here, which is what makes the pair worth having rather than
// the interactive half alone.
func TestFcWritesAHistoryFileOnlyWhenTheShellIsInteractive(t *testing.T) {
	t.Parallel()
	const src = "HISTFILE=saved\nSAVEHIST=10\nprint -s 'echo x'\nfc -W\n"

	quiet := t.TempDir()
	if out, st := fcScript(t, quiet, src); out != "" || st != 0 {
		t.Errorf("script: out = %q status = %d, want silence at 0", out, st)
	}
	if _, err := os.Stat(filepath.Join(quiet, "saved")); err == nil {
		t.Error("a script wrote a history file, which zsh does not")
	}

	session := t.TempDir()
	if out, st := fcRun(t, session, src); out != "" || st != 0 {
		t.Errorf("session: out = %q status = %d, want silence at 0", out, st)
	}
	data, err := os.ReadFile(filepath.Join(session, "saved"))
	if err != nil {
		t.Fatalf("a session did not write a history file: %v", err)
	}
	if got, want := string(data), "echo x\n"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

// Three conditions write nothing, all at status 0 and all in silence.
//
// The silence is the part worth pinning. Every one of these is a script
// asking for something zsh declines to do, and zsh says nothing about any of
// them — including `HISTFILE` unset, which is the case bash's `history -w`
// answers by naming the variable and failing. Two shells, two answers, and a
// diagnostic here would appear in rc files that save history unconditionally.
func TestFcWritesNothingAndSaysNothingWhenItWouldNotSave(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src string }{
		{"an empty list", "HISTFILE=saved\nSAVEHIST=10\nfc -W\n"},
		{"SAVEHIST is zero", "HISTFILE=saved\nSAVEHIST=0\nprint -s x\nfc -W\n"},
		{"SAVEHIST is unset", "HISTFILE=saved\nunset SAVEHIST\nprint -s x\nfc -W\n"},
		{"HISTFILE is unset", "unset HISTFILE\nSAVEHIST=10\nprint -s x\nfc -W\n"},
	} {
		dir := t.TempDir()
		out, st := fcRun(t, dir, c.src)
		if out != "" || st != 0 {
			t.Errorf("%s: out = %q status = %d, want silence at 0", c.name, out, st)
		}
		if _, err := os.Stat(filepath.Join(dir, "saved")); err == nil {
			t.Errorf("%s: a file was written", c.name)
		}
	}
}

// `SAVEHIST` is a switch here and not a bound.
//
// It reads like a count and a shell that trimmed to it would look right, so
// this is measured rather than assumed: `SAVEHIST=1` with three entries
// writes all three. Whatever the variable bounds, it is not what `fc -W`
// puts down.
func TestSavehistDecidesWhetherFcWritesAndNotHowMuch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fcRun(t, dir, "HISTFILE=saved\nSAVEHIST=1\nprint -s a\nprint -s b\nprint -s c\nfc -W\n")
	data, err := os.ReadFile(filepath.Join(dir, "saved"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "a\nb\nc\n"; got != want {
		t.Errorf("contents = %q, want %q — SAVEHIST trimmed the write", got, want)
	}
}

// `-W` truncates and `-A` appends, and the pair is what tells them apart.
//
// `-A` twice over a one-entry list leaves that entry twice, because each
// append puts the whole list down again rather than only what arrived since.
// Measured, and it is the letter's own oddity rather than this shell's.
func TestFcWriteTruncatesWhereAppendAdds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"-W twice", "HISTFILE=saved\nSAVEHIST=10\nprint -s one\nfc -W\nfc -W\n", "one\n"},
		{"-A twice", "HISTFILE=saved\nSAVEHIST=10\nprint -s one\nfc -A\nfc -A\n", "one\none\n"},
	} {
		dir := t.TempDir()
		fcRun(t, dir, c.src)
		data, err := os.ReadFile(filepath.Join(dir, "saved"))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := string(data); got != c.want {
			t.Errorf("%s: contents = %q, want %q", c.name, got, c.want)
		}
	}
}

// The file is `0600`, which is narrower than the umask would give.
//
// A history file holds what somebody typed, so the shell that writes it picks
// the mode itself. The same answer bash's `history -w` gives, measured on
// both, and a test that accepted 0644 would be asserting the umask.
func TestFcWritesAHistoryFileReadableOnlyByItsOwner(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fcRun(t, dir, "HISTFILE=saved\nSAVEHIST=10\nprint -s x\nfc -W\n")
	info, err := os.Stat(filepath.Join(dir, "saved"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %#o, want 0600", got)
	}
}

// `-R` reads a file into the list, and `fc -l` prints it.
//
// The round trip is the case: what `-W` writes, `-R` reads back, and the
// listing is the `%5d  %s` shape numbered from one.
func TestFcReadFillsTheListAndTheListingPrintsIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seed"), []byte("echo r1\necho r2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := fcRun(t, dir, "fc -R seed\nfc -l\n")
	if want := "    1  echo r1\n    2  echo r2\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// `print -s` is what puts a line in the list, joined the way a command line
// is, and it still writes nothing to the output.
func TestPrintDashSRemembersWithoutWriting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := fcRun(t, dir, "print -s echo two args\nfc -l\n")
	if want := "    1  echo two args\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}

// Every other letter is still the core builtin's.
//
// This registration replaces `fc` rather than reimplementing it, so a letter
// it does not claim must reach the same answer it reached before — and `-l`
// on an *empty* list is the one that would be easiest to break, since that is
// the reading this file takes over when the list has something in it.
func TestFcLeavesEveryOtherLetterToTheCoreBuiltin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := fcRun(t, dir, "fc -l\n")
	if out == "" && st == 0 {
		return // the core's answer for a shell with nothing to list
	}
	// zsh's own answer is a refusal naming the event it cannot find, which
	// the core builtin already models through the semantics vector. Either
	// way it must not be this file's listing of an empty store.
	if out == "    1  \n" {
		t.Errorf("out = %q: the empty store was listed as an entry", out)
	}
}
