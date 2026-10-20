// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// TestAScriptOperandThatWillNotOpen pins zsh's answer, which is the one that
// makes the pair a pair rather than a rule.
//
// zsh knows the file would not open and declines to say which way: a missing
// path, a mode-000 file and a directory all get the same sentence and the same
// 127, where bash and ksh93 split the wording and the status and dash splits
// neither but answers 2. Three different shapes over one failure, which is why
// this is Diagnostics data and not a branch.
func TestAScriptOperandThatWillNotOpen(t *testing.T) {
	d := zsh.Diagnostics()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"missing", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.ENOENT}},
		{"unreadable", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.EACCES}},
		{"a directory", &fs.PathError{Op: "read", Path: "s.sh", Err: syscall.EISDIR}},
	} {
		const want = "<shell>: can't open input file: s.sh\n"
		if got := d.ScriptDiagnostic("<shell>", "s.sh", tc.err); got != want {
			t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, want)
		}
		if got := d.ScriptStatus(tc.err); got != 127 {
			t.Errorf("%s: ScriptStatus = %d, want 127", tc.name, got)
		}
	}
}
