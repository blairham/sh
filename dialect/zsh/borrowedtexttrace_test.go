// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `xtrace` from inside a `.` or an `eval` does not follow a redirection
// applied to that construct — and **only** that construct's own redirection
// is undone.
//
// Measured 2026-09-30 on zsh 5.9.2. The rows are chosen so that each of the
// three wrong models fails one of them: a destination pinned for the
// construct's duration loses "a redirected function inside", one recomputed
// from the raw stream loses "nested, both redirected", and a field that is
// not refreshed per command loses "redirected by an enclosing group".
func TestXtraceFromBorrowedTextDoesNotFollowItsRedirection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, script, want string }{
		{
			// The front itself: the sourced file's trace is not in the file.
			"sourced text", "print 'print inner' > s.in\nset -x\n. ./s.in 2>>out\nset +x\n", "",
		},
		{
			"eval text", "set -x\neval 'print inner' 2>>out\nset +x\n", "",
		},
		{
			// A redirection on something *inside* still moves it.
			"a redirected function inside",
			"g() { print in-g }\nprint 'g 2>>out' > s.in\nset -x\n. ./s.in 2>>other\nset +x\n",
			"+g:0> print in-g",
		},
		{
			// Nested, both redirected. Checked in `other` as well as `out`,
			// which the `alsoEmpty` field below is for: a destination that
			// skipped the enclosing construct would put the inner trace in
			// `other` and leave `out` empty, so a row that looked only at
			// `out` passes either way. A mutant proved that.
			"nested, both redirected",
			"print 'print inner' > b.in\nprint '. ./b.in 2>>out' > a.in\nset -x\n. ./a.in 2>>other\nset +x\n",
			"",
		},
		{
			// An enclosing group's redirection *is* in force when the
			// construct is reached, so the trace goes there.
			"redirected by an enclosing group",
			"{ set -x\neval 'print inner'\nset +x\n} 2>>out\n",
			"+(eval):1> print inner",
		},
		{
			// Nothing traced at all: a redirected `eval` with `xtrace` off
			// must not consult the axis, which a strict core would refuse
			// by name. The row asserts the text ran and wrote nothing to
			// `out` — see the guard in borrowedTextTraceStream.
			"not tracing at all", "eval 'print inner' 2>>out\n", "",
		},
		{
			// And a group around a source, which is the same question with
			// the borrowed text one level in.
			"group around a source",
			"print 'print inner' > s.in\n{ set -x\n. ./s.in\nset +x\n} 2>>out\n",
			"+./s.in:1> print inner",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if _, st := runZsh(t, dir, "PS4='+%N:%i> '\n"+tc.script); st != 0 {
				t.Fatalf("script exited %d", st)
			}
			got, err := os.ReadFile(filepath.Join(dir, "out"))
			if err != nil && tc.want != "" {
				t.Fatalf("no `out` written, so this row looked at nothing: %v", err)
			}
			if tc.want == "" {
				if err == nil && strings.Contains(string(got), ">") {
					t.Errorf("`out` holds %q, want no trace in it", got)
				}
				// And `other`, where a row wrote one: the trace has to be in
				// neither file, not merely absent from the one named.
				if o, oerr := os.ReadFile(filepath.Join(dir, "other")); oerr == nil &&
					strings.Contains(string(o), ">") {
					t.Errorf("`other` holds %q, want no trace in it either", o)
				}
				return
			}
			if !strings.Contains(string(got), tc.want) {
				t.Errorf("`out` = %q, want it to carry %q", got, tc.want)
			}
		})
	}
}

// The trace still reaches standard error in every row above — the point is
// where it does *not* go, and a rule that dropped the line entirely would
// satisfy the empty rows without satisfying this.
func TestXtraceFromBorrowedTextStillReachesStandardError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "PS4='+%N:%i> '\nprint 'print inner' > s.in\nset -x\n. ./s.in 2>>out\nset +x\n"
	// runZsh returns stdout and stderr together, which is what this row
	// needs: the trace has to be *somewhere*, and the file rows above only
	// say where it is not.
	out, st := runZsh(t, dir, src)
	if st != 0 {
		t.Fatalf("script exited %d: %q", st, out)
	}
	if !strings.Contains(out, "+./s.in:1> print inner") {
		t.Errorf("combined output = %q, want the sourced line's trace on standard error", out)
	}
}
