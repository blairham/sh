// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `return` written in an EXIT trap's body, which two columns let name the
// shell's exit status and two do not — see
// Semantics.ReturnInTheExitTrapNamesTheStatus. The machinery for "the body
// can name the status" was already there for `exit`; only `return` did not
// reach it, so a caller branching on `$?` read success from a script that had
// said otherwise (#4559).
//
// **The script's own status is not nought**, and that is the whole of what
// makes this a test. With the script already at 0 every reading reports 0 and
// the probe decides nothing, which is how a one-line repro can make the
// question look unanimous.
func TestAReturnInTheExitTrapCanNameTheShellsStatus(t *testing.T) {
	// `false` leaves 1 behind, and the body names 5.
	const src = "trap 'echo X; return 5' EXIT\nfalse\n"
	for _, tc := range []struct {
		name    string
		names   Answer
		refused Answer
		want    int
		wantErr string
	}{
		// The status the body named, over the one the script was leaving
		// with.
		{"the return names it", Yes, No, 5, ""},
		// And the reading that lets the body end there without letting it
		// say anything about the status.
		{"the return ends the body only", No, No, 1, ""},
		// The control, and the reason the axis above carries an `unpinned`
		// verdict for the column that refuses: the two questions are asked
		// in an order, and where a `return` with nothing to return from is
		// refused outright the body never ends on one — so the line after
		// it runs and the status is the script's under either answer.
		{"a refused return decides nothing", Yes, Yes, 1, "can only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, status := trapRun(t, src, func(s *Semantics) {
				s.ReturnInTheExitTrapNamesTheStatus = tc.names
				s.ReturnOutsideAFunctionIsRefused = tc.refused
				// The refusal is a special builtin's complaint about how it
				// was called, and whether *that* ends the script is a third
				// axis. Answered here the way the one column that refuses
				// answers it outside POSIX mode, so that what this row
				// measures is the status the shell was already leaving with
				// rather than the refusal's own.
				s.BadOptionToSpecialBuiltinFatal = No
			}, Diagnostics{})
			if !strings.Contains(out, "X") {
				t.Errorf("stdout %q, want the body to have run", out)
			}
			if status != tc.want {
				t.Errorf("status %d, want %d", status, tc.want)
			}
			if tc.wantErr == "" && errs != "" {
				t.Errorf("stderr %q, want nothing", errs)
			}
			if tc.wantErr != "" && !strings.Contains(errs, tc.wantErr) {
				t.Errorf("stderr %q, want it to contain %q", errs, tc.wantErr)
			}
		})
	}
}

// The status the body named reaches a *subshell*'s own `$?` the same way,
// which is the same boundary and the same code path: an EXIT trap set inside
// a subshell fires as that subshell ends, and what it names is what the
// parent reads.
//
// Here as a row of its own because the two shapes are measured separately in
// the panel — `( trap 'return 5' EXIT; true ); echo $?` is 5 in the two
// columns that take the status and 0 in the two that do not — and a fix at
// the shell's own end would have left this one alone.
func TestAReturnInASubshellsExitTrapReachesItsStatus(t *testing.T) {
	const src = "( trap 'echo X; return 5' EXIT; true )\necho sub=$?\n"
	for _, tc := range []struct {
		names Answer
		want  string
	}{
		{Yes, "sub=5"},
		{No, "sub=0"},
	} {
		out, _, _ := trapRun(t, src, func(s *Semantics) {
			s.ReturnInTheExitTrapNamesTheStatus = tc.names
			s.ReturnOutsideAFunctionIsRefused = No
		}, Diagnostics{})
		if !strings.Contains(out, tc.want) {
			t.Errorf("%v: got %q, want it to contain %q", tc.names, out, tc.want)
		}
	}
}
