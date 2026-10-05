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

// TestARefusedCdThatClimbsIsNamedWhereItClimbs is #6091: in the dialect with
// Diagnostics.CdNamesALeadingDotDotFromTheDirectory, an operand that starts
// by climbing is named as the path it climbs to — the leading run taken off
// the directory, the rest as typed — and every other operand as written.
// With the switch off every row is the operand, which is the control.
func TestARefusedCdThatClimbsIsNamedWhereItClimbs(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	from := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(from, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ src, climbed string }{
		{"cd ../nosuch", root + "/nosuch"},
		{"cd ./../nosuch/..", root + "/nosuch/.."},
		{"cd ..//nosuch/..", root + "//nosuch/.."},
		{"cd ../nosuch/../x", root + "/nosuch/../x"},
		{"cd -P ../nosuch/..", root + "/nosuch/.."},
		{"cd deep/../../nosuch/..", "deep/../../nosuch/.."},
		{"cd nosuch", "nosuch"},
	} {
		for _, on := range []bool{true, false} {
			sem := PosixSemantics()
			sem.CdCancelsADotDot = CdDotDotLooksWithinTheOperand
			errs := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Dir: from, Stderr: errs,
				Diagnostics: &Diagnostics{
					CdCannotChange:                        "cd: %[1]s: [%[2]s]",
					CdNamesALeadingDotDotFromTheDirectory: on,
				},
			})
			runPart(t, r, c.src+"\n")
			want := strings.TrimPrefix(c.src, "cd ")
			want = strings.TrimPrefix(want, "-P ")
			if on {
				want = c.climbed
			}
			if got := errs.String(); got != "sh: cd: "+want+": [No such file or directory]\n" ||
				strings.Count(got, "\n") != 1 {
				t.Errorf("%s (switch %v): %q, want the line naming %s", c.src, on, got, want)
			}
		}
	}
}
