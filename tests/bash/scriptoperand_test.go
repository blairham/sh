// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// TestAScriptOperandThatWillNotOpen pins what bash says and reports when the
// script it was handed could not be read.
//
// Measured five ways — a missing path, a path whose parent is missing, a
// dangling symlink, a mode-000 file and a directory — which collapse into the
// two bash tells apart: nothing there is a missing command's 127, and there
// but unreadable is an unrunnable command's 126. It was a flat 2 for all of
// them, which is dash's answer given to everybody.
func TestAScriptOperandThatWillNotOpen(t *testing.T) {
	d := bash.Diagnostics()
	for _, tc := range []struct {
		name   string
		err    error
		want   string
		status int
	}{
		{
			"missing", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.ENOENT},
			"<shell>: s.sh: No such file or directory\n", 127,
		},
		{
			"unreadable", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.EACCES},
			"<shell>: s.sh: Permission denied\n", 126,
		},
		{
			// bash reaches this one after it has already taken the operand
			// for its own name, so the line it prints carries the script's
			// path where the shell's would go — `d: d: Is a directory`.
			// Which name stands there is the front end's to choose and
			// ScriptOperandNamesItself is what it asks; the wording is the
			// same one the rows above use, which is why this passes the
			// script's own path as the name.
			"a directory", &fs.PathError{Op: "read", Path: "s.sh", Err: syscall.EISDIR},
			"s.sh: s.sh: Is a directory\n", 126,
		},
	} {
		// The name in front of it is the operand's own once this shell has
		// the file open, and the shell's own where the open is what failed.
		// See Diagnostics.ScriptOperandNamedByItselfOnceOpened.
		name := "<shell>"
		if d.ScriptOperandNamesItself(tc.err) {
			name = "s.sh"
		}
		if got := d.ScriptDiagnostic(name, "s.sh", tc.err); got != tc.want {
			t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, tc.want)
		}
		if got := d.ScriptStatus(tc.err); got != tc.status {
			t.Errorf("%s: ScriptStatus = %d, want %d", tc.name, got, tc.status)
		}
	}
}

// And an operand read whole and then declined, which is the failure with no
// errno behind it: `bash lsbin`, a copy of `/bin/ls` on PATH, is `<path>:
// <path>: cannot execute binary file` at 126. Measured 2026-09-22 — #4159.
func TestAScriptOperandWhoseContentIsNotShellText(t *testing.T) {
	d := bash.Diagnostics()
	if got := d.ScriptDiagnostic("lsbin", "lsbin", interp.ErrBinaryScript); got !=
		"lsbin: lsbin: cannot execute binary file\n" {
		t.Errorf("ScriptDiagnostic = %q", got)
	}
	if got := d.ScriptStatus(interp.ErrBinaryScript); got != 126 {
		t.Errorf("ScriptStatus = %d, want 126", got)
	}
	// And it names the operand for the reason a directory does: the file was
	// open when the shell declined it.
	if !d.ScriptOperandNamesItself(interp.ErrBinaryScript) {
		t.Error("ScriptOperandNamesItself = false, want the operand's own name")
	}
}
