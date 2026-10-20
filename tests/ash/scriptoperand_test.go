// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/ash"
)

// TestAScriptOperandThatWillNotOpen: the `.` builtin's wording, the system's
// own reason, and 2 whatever went wrong. Measured 2026-10-03 in the pinned
// image, `ash nosuch.sh` and a mode-000 file read by an ordinary user. See
// interp.Diagnostics.ScriptReasonIsTheSystems.
func TestAScriptOperandThatWillNotOpen(t *testing.T) {
	d := ash.Diagnostics()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			"missing", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.ENOENT},
			"ash: can't open 's.sh': No such file or directory\n",
		},
		{
			"unreadable", &fs.PathError{Op: "open", Path: "s.sh", Err: syscall.EACCES},
			"ash: can't open 's.sh': Permission denied\n",
		},
	} {
		if got := d.ScriptDiagnostic("ash", "s.sh", tc.err); got != tc.want {
			t.Errorf("%s: ScriptDiagnostic = %q, want %q", tc.name, got, tc.want)
		}
		if got := d.ScriptStatus(tc.err); got != 2 {
			t.Errorf("%s: ScriptStatus = %d, want 2", tc.name, got)
		}
	}
}

// TestPS4TakesNoPromptEscapes: the trace prefix is expanded and nothing more,
// where PS1 reads this shell's escapes. Measured 2026-10-03 in the pinned
// image. See interp.PromptStyle.TraceTakesNoEscapes.
func TestPS4TakesNoPromptEscapes(t *testing.T) {
	out, _ := run(t, `PS4='<\u|\w|\h|\e|\101|\x41|\[|\$|\\|$((1+1))> '; set -x; :; set +x`)
	want := `<\u|\w|\h|\e|\101|\x41|\[|$|\|2> :` + "\n" + `<\u|\w|\h|\e|\101|\x41|\[|$|\|2> set +x` + "\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
