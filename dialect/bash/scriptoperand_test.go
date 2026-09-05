// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/bash"
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
			// The status only. bash reaches this one after it has already
			// taken the operand for its own name, so the line it prints
			// carries the script's path where the shell's would go —
			// `d: d: Is a directory` — which is a quirk of the order it does
			// things in rather than a wording any dialect vector could hold.
			"a directory", &fs.PathError{Op: "read", Path: "s.sh", Err: syscall.EISDIR},
			"", 126,
		},
	} {
		if tc.want != "" {
			if got := d.ScriptDiagnostic("<shell>", "s.sh", tc.err); got != tc.want {
				t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, tc.want)
			}
		}
		if got := d.ScriptStatus(tc.err); got != tc.status {
			t.Errorf("%s: ScriptStatus = %d, want %d", tc.name, got, tc.status)
		}
	}
}
