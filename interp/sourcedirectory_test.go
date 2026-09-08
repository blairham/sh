// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A directory where `.` wants a file is an axis, not an errno.
//
// The panel splits down the middle — two shells open the directory, read no
// commands out of it and report success in silence, and four call it an error
// — so the substrate asks rather than picks. These tests name the axis and
// never a shell; which answer each dialect gives is measured by the corpus
// rows under `dot/a-directory-operand-…`.
//
// We used to answer `no such file or directory` at 127 in every dialect,
// which is a fifth answer and a self-contradictory one: 127 is the status
// that says the path was never opened, for a path that plainly exists
// (#1577).

func directorySemantics(t *testing.T, isError Answer) Semantics {
	t.Helper()
	s := permissive()
	s.DotDirectoryOperandIsAnError = isError
	return s
}

// TestADirectoryOperandCanBeNoErrorAtAll pins the half that is not a
// diagnostic: the directory is opened, yields no commands, and that is an
// empty script rather than a failure.
func TestADirectoryOperandCanBeNoErrorAtAll(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := sourceRun(t, dir, "false\n. ./sub\necho \"st=$?\"\n",
		directorySemantics(t, No), Diagnostics{})
	if st != 0 {
		t.Errorf("shell status %d, want 0", st)
	}
	// The status is the empty script's own 0 and not the `false` before it,
	// which is what says the file was run rather than skipped. A fix that
	// returned early without touching the status would print st=1 here and
	// still say nothing, so the silence alone does not pin this.
	if got := out; got != "st=0\n" {
		t.Errorf("got %q, want a silent st=0", got)
	}
}

// TestADirectoryOperandCanBeAnError is the other answer, and it checks the
// status as well as the sentence: reporting the right words at 127 was half
// of what was wrong.
func TestADirectoryOperandCanBeAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := sourceRun(t, dir, ". ./sub\necho \"st=$?\"\n",
		directorySemantics(t, Yes),
		Diagnostics{DotCannotOpen: ".: %[1]s: %[2]s", DotCannotOpenStatus: 1})
	if !strings.Contains(out, ".: ./sub: Is a directory") {
		t.Errorf("got %q, want the reason to be the directory", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the dialect's cannot-open status", out)
	}
	if strings.Contains(out, "No such file or directory") || strings.Contains(out, "st=127") {
		t.Errorf("got %q, want no claim that the path is missing", out)
	}
}

// TestADirectoryOperandCanUseAWordingOfItsOwn pins the third verb.
//
// One dialect names the builtin here and names it nowhere else on this
// builtin, so the format takes the invoked name as well as the operand and
// the reason. A dialect that leaves the field empty keeps the one sentence it
// already had, which is what the test above measures.
func TestADirectoryOperandCanUseAWordingOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := sourceRun(t, dir, ". ./sub\n", directorySemantics(t, Yes),
		Diagnostics{
			DotCannotOpen:   "%[1]s: %[2]s",
			DotIsADirectory: "%[3]s: %[1]s: is a directory",
		})
	if want := ".: ./sub: is a directory"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestADirectoryTheShellCannotOpenIsNotADirectoryQuestion is the clause that
// took a measurement to find, and the one a fix reaches for last.
//
// Deciding on the stat alone is the obvious implementation and it is wrong:
// stat succeeds on a directory whose mode is 000, because the search
// permission that matters is the *parent's*. Every column in the panel calls
// that one a permission failure, including the two that call a readable
// directory success — so a shell that answered "it is a directory" here would
// report success for a directory it could not open at all.
func TestADirectoryTheShellCannotOpenIsNotADirectoryQuestion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a mode-000 directory, so there is no denial to measure")
	}
	dir := t.TempDir()
	closed := filepath.Join(dir, "shut")
	if err := os.Mkdir(closed, 0o000); err != nil {
		t.Fatal(err)
	}
	// Restored so the temporary directory can be removed.
	t.Cleanup(func() { _ = os.Chmod(closed, 0o755) })

	// The axis says a directory is *no* error, which is the answer that makes
	// the mistake visible: if the permission failure were taken for a
	// directory, this would run silently at 0.
	out, _ := sourceRun(t, dir, ". ./shut\necho \"st=$?\"\n",
		directorySemantics(t, No),
		Diagnostics{DotCannotOpen: ".: %[1]s: %[2]s", DotCannotOpenStatus: 1})
	if !strings.Contains(out, "Permission denied") {
		t.Errorf("got %q, want the kernel's own reason", out)
	}
	if strings.Contains(out, "st=0\n") {
		t.Errorf("got %q, want the open to have failed", out)
	}
}
