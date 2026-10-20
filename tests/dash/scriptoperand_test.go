// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// TestAScriptOperandThatWillNotOpen pins dash's answer, which is the one the
// whole panel used to get: 2 whatever went wrong.
//
// It is also the only dialect that writes a line it has not reached — the
// `0:` in `dash: 0: cannot open …` — and the only one whose reason for a
// missing file is its own text rather than the operating system's.
//
// A *directory* is deliberately not here. Real dash opens one, reads nothing
// from it and exits 0 in silence, which is a shell doing nothing where the
// other three refuse; this dialect reports it as unreadable instead, and that
// divergence is recorded rather than reproduced.
func TestAScriptOperandThatWillNotOpen(t *testing.T) {
	d := dash.Diagnostics()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			"missing", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.ENOENT},
			"<shell>: 0: cannot open s.sh: No such file\n",
		},
		{
			"unreadable", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.EACCES},
			"<shell>: 0: cannot open s.sh: Permission denied\n",
		},
	} {
		if got := d.ScriptDiagnostic("<shell>", "s.sh", tc.err); got != tc.want {
			t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, tc.want)
		}
		if got := d.ScriptStatus(tc.err); got != 2 {
			t.Errorf("%s: ScriptStatus = %d, want 2", tc.name, got)
		}
	}
}
