// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// CDPATH: a relative operand that does not begin with a dot segment is
// looked for under each entry, and where the winning entry is not `.` the
// shell may say where it went — three of the panel do, one moves in silence.
func TestCdpathSearchesAndMayAnnounce(t *testing.T) {
	for _, c := range []struct {
		name      string
		announces Answer
		cdpath    string
		operand   string
		announced bool
		found     bool
	}{
		{"found and announced", Yes, "./pool", "sub", true, true},
		{"found in silence", No, "./pool", "sub", false, true},
		{"a dot entry moves quietly", Yes, ".:./pool", "here", false, true},
		{"a dot-led operand skips the search", Yes, "./pool", "./sub", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, d := range []string{
				filepath.Join(dir, "pool", "sub"),
				filepath.Join(dir, "here"),
			} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			sem := PosixSemantics()
			sem.CdpathAnnouncesTheDirectory = c.announces
			out := &strings.Builder{}
			errOut := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errOut,
				Vars: map[string]string{"CDPATH": c.cdpath},
			})
			f, err := syntax.Parse("cd "+c.operand+"\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if c.found != (r.Dir != dir) {
				t.Errorf("cd %s from %q left Dir %q; CDPATH=%q", c.operand, dir, r.Dir, c.cdpath)
			}
			if got := strings.TrimSpace(out.String()); (got != "") != c.announced {
				t.Errorf("stdout %q; announced = %v, want %v", got, got != "", c.announced)
			} else if c.announced && got != r.Dir {
				t.Errorf("announced %q, want the destination %q", got, r.Dir)
			}
		})
	}
}
