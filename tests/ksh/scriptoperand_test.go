// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// TestAScriptOperandThatWillNotOpen pins ksh93's answer.
//
// It splits the same two ways bash does — 127 for a path that names nothing,
// 126 for one that is there and will not open — and is the only dialect that
// words them apart as well: a missing script is simply "not found", the way a
// missing command is, and one that will not open gets the bracketed reason
// ksh93 puts around every errno it quotes.
func TestAScriptOperandThatWillNotOpen(t *testing.T) {
	d := ksh.Diagnostics()
	for _, tc := range []struct {
		name   string
		err    error
		want   string
		status int
	}{
		{
			"missing", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.ENOENT},
			"<shell>: s.sh: not found\n", 127,
		},
		{
			"unreadable", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.EACCES},
			"<shell>: s.sh: cannot open [Permission denied]\n", 126,
		},
		{
			"a directory", &fs.PathError{Op: "read", Path: "s.sh", Err: syscall.EISDIR},
			"<shell>: s.sh: cannot open [Is a directory]\n", 126,
		},
	} {
		if got := d.ScriptDiagnostic("<shell>", "s.sh", tc.err); got != tc.want {
			t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, tc.want)
		}
		if got := d.ScriptStatus(tc.err); got != tc.status {
			t.Errorf("%s: ScriptStatus = %d, want %d", tc.name, got, tc.status)
		}
	}
}
